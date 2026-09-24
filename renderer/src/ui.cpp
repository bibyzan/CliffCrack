#include "ui.h"

#include <imgui.h>
#include <imgui_impl_vulkan.h>

#include <cfloat>
#include <cmath>
#include <cstdio>

namespace {

bool     g_ready = false;
VkFormat g_color_format = VK_FORMAT_UNDEFINED; // the backend keeps a pointer to these
VkFormat g_depth_format = VK_FORMAT_UNDEFINED;

float srgb_to_linear(float c) {
    return c <= 0.04045f ? c / 12.92f : std::pow((c + 0.055f) / 1.055f, 2.4f);
}

// ImGui's palette is authored in sRGB, but we draw into an sRGB swapchain that
// encodes on write; linearise the style so colours come out as designed.
void linearize_style() {
    ImGuiStyle& style = ImGui::GetStyle();
    for (ImVec4& c : style.Colors) {
        c.x = srgb_to_linear(c.x);
        c.y = srgb_to_linear(c.y);
        c.z = srgb_to_linear(c.z);
    }
}

ImGui_ImplVulkan_PipelineInfo pipeline_info() {
    ImGui_ImplVulkan_PipelineInfo info{};
    info.MSAASamples = VK_SAMPLE_COUNT_1_BIT;
    info.PipelineRenderingCreateInfo = {VK_STRUCTURE_TYPE_PIPELINE_RENDERING_CREATE_INFO};
    info.PipelineRenderingCreateInfo.colorAttachmentCount = 1;
    info.PipelineRenderingCreateInfo.pColorAttachmentFormats = &g_color_format;
    info.PipelineRenderingCreateInfo.depthAttachmentFormat = g_depth_format;
    return info;
}

void check_vk(VkResult result) {
    if (result < 0) std::fprintf(stderr, "[renderer] imgui vulkan error: VkResult %d\n", static_cast<int>(result));
}

} // namespace

bool ui_init(const UiInitInfo& info, std::string* error) {
    IMGUI_CHECKVERSION();
    ImGui::CreateContext();
    ImGuiIO& io = ImGui::GetIO();
    io.IniFilename = nullptr; // don't litter imgui.ini next to the exe
    ImGui::StyleColorsDark();
    linearize_style();

    // The backend has no prototypes: hand it the loader volk already opened.
    const bool loaded = ImGui_ImplVulkan_LoadFunctions(
        VK_API_VERSION_1_3,
        [](const char* name, void* instance) -> PFN_vkVoidFunction {
            return vkGetInstanceProcAddr(static_cast<VkInstance>(instance), name);
        },
        info.instance);
    if (!loaded) {
        ImGui::DestroyContext();
        *error = "ImGui_ImplVulkan_LoadFunctions failed";
        return false;
    }

    g_color_format = info.color_format;
    g_depth_format = info.depth_format;

    ImGui_ImplVulkan_InitInfo init{};
    init.ApiVersion = VK_API_VERSION_1_3;
    init.Instance = info.instance;
    init.PhysicalDevice = info.physical_device;
    init.Device = info.device;
    init.QueueFamily = info.queue_family;
    init.Queue = info.queue;
    init.DescriptorPoolSize = IMGUI_IMPL_VULKAN_MINIMUM_SAMPLED_IMAGE_POOL_SIZE; // backend-owned pool
    init.MinImageCount = 2;
    init.ImageCount = info.image_count < 2 ? 2 : info.image_count;
    init.UseDynamicRendering = true;
    init.PipelineInfoMain = pipeline_info();
    init.CheckVkResultFn = check_vk;
    if (!ImGui_ImplVulkan_Init(&init)) {
        ImGui::DestroyContext();
        *error = "ImGui_ImplVulkan_Init failed";
        return false;
    }
    g_ready = true;
    return true;
}

void ui_set_formats(VkFormat color_format, VkFormat depth_format) {
    if (!g_ready || (color_format == g_color_format && depth_format == g_depth_format)) return;
    g_color_format = color_format;
    g_depth_format = depth_format;
    ImGui_ImplVulkan_PipelineInfo info = pipeline_info();
    ImGui_ImplVulkan_CreateMainPipeline(&info);
}

void ui_frame(VkCommandBuffer cmd, VkExtent2D extent, const RUIInput& input,
              RUICmd* cmds, uint32_t count, const char* text, uint32_t text_length, RUIOutput* out) {
    if (!g_ready) return;

    ImGuiIO& io = ImGui::GetIO();
    io.DisplaySize = ImVec2(static_cast<float>(extent.width), static_cast<float>(extent.height));
    io.DeltaTime = input.delta_time > 0.0f ? input.delta_time : 1.0f / 60.0f;
    if (input.mouse_x < 0.0f || input.mouse_y < 0.0f) {
        io.AddMousePosEvent(-FLT_MAX, -FLT_MAX); // no mouse (e.g. captured for camera look)
    } else {
        io.AddMousePosEvent(input.mouse_x, input.mouse_y);
    }
    for (int b = 0; b < 3; ++b) io.AddMouseButtonEvent(b, ((input.mouse_buttons >> b) & 1u) != 0);
    if (input.wheel != 0.0f) io.AddMouseWheelEvent(0.0f, input.wheel);

    ImGui_ImplVulkan_NewFrame();
    ImGui::NewFrame();

    bool        window_open = false;
    bool        collapsed = false; // current window's contents are hidden
    std::string label;
    for (uint32_t i = 0; i < count; ++i) {
        RUICmd& c = cmds[i];
        c.result = 0;
        if (c.label_offset <= text_length && c.label_length <= text_length - c.label_offset) {
            label.assign(text + c.label_offset, c.label_length);
        } else {
            label.clear();
        }

        if (c.kind == R_UI_WINDOW) {
            if (window_open) ImGui::End(); // tolerate a missing R_UI_END
            if (c.x != 0.0f || c.y != 0.0f) ImGui::SetNextWindowPos(ImVec2(c.x, c.y), ImGuiCond_FirstUseEver);
            collapsed = !ImGui::Begin(label.empty() ? "##window" : label.c_str(), nullptr,
                                      ImGuiWindowFlags_AlwaysAutoResize);
            window_open = true;
            continue;
        }
        if (c.kind == R_UI_END) {
            if (window_open) ImGui::End();
            window_open = collapsed = false;
            continue;
        }
        if (collapsed) continue;

        switch (c.kind) {
        case R_UI_TEXT:
            ImGui::TextUnformatted(label.data(), label.data() + label.size());
            break;
        case R_UI_SLIDER: {
            float v = c.value;
            if (ImGui::SliderFloat(label.c_str(), &v, c.min, c.max)) {
                c.value = v;
                c.result = 1;
            }
            break;
        }
        case R_UI_CHECKBOX: {
            bool v = c.value != 0.0f;
            if (ImGui::Checkbox(label.c_str(), &v)) {
                c.value = v ? 1.0f : 0.0f;
                c.result = 1;
            }
            break;
        }
        case R_UI_BUTTON:
            if (ImGui::Button(label.c_str())) c.result = 1;
            break;
        case R_UI_SEPARATOR:
            ImGui::Separator();
            break;
        default:
            break;
        }
    }
    if (window_open) ImGui::End();

    ImGui::Render();
    ImGui_ImplVulkan_RenderDrawData(ImGui::GetDrawData(), cmd);

    if (out) {
        out->want_mouse = io.WantCaptureMouse ? 1 : 0;
        out->want_keyboard = io.WantCaptureKeyboard ? 1 : 0;
    }
}

void ui_shutdown() {
    if (!g_ready) return;
    ImGui_ImplVulkan_Shutdown();
    ImGui::DestroyContext();
    g_ready = false;
}
