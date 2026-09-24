#include "ui.h"

#include <imgui.h>
#include <imgui_impl_vulkan.h>

#include <algorithm>
#include <cfloat>
#include <cmath>
#include <cstdio>

namespace {

bool     g_ready = false;
float    g_scale = 1.0f; // UI size multiplier
VkFormat g_color_format = VK_FORMAT_UNDEFINED; // the backend keeps a pointer to these
VkFormat g_depth_format = VK_FORMAT_UNDEFINED;

float srgb_to_linear(float c) {
    return c <= 0.04045f ? c / 12.92f : std::pow((c + 0.055f) / 1.055f, 2.4f);
}

// The game's look: deep navy translucent panels, rounded corners, warm white
// text and the ball's orange as the accent. Colours are written in sRGB here
// and linearised afterwards.
void apply_style() {
    ImGuiStyle& style = ImGui::GetStyle();
    style.WindowRounding = 12.0f;
    style.ChildRounding = 8.0f;
    style.FrameRounding = 8.0f;
    style.PopupRounding = 8.0f;
    style.GrabRounding = 8.0f;
    style.ScrollbarRounding = 8.0f;
    style.WindowBorderSize = 0.0f;
    style.FrameBorderSize = 0.0f;
    style.WindowPadding = ImVec2(18.0f, 14.0f);
    style.FramePadding = ImVec2(12.0f, 6.0f);
    style.ItemSpacing = ImVec2(10.0f, 8.0f);
    style.WindowTitleAlign = ImVec2(0.5f, 0.5f);

    const ImVec4 navy(0.07f, 0.09f, 0.16f, 0.84f);
    const ImVec4 slate(0.17f, 0.21f, 0.33f, 0.95f);
    const ImVec4 slate_hi(0.23f, 0.28f, 0.43f, 1.0f);
    const ImVec4 orange(0.88f, 0.42f, 0.11f, 1.0f);
    const ImVec4 orange_hi(0.97f, 0.55f, 0.20f, 1.0f);
    const ImVec4 text(0.97f, 0.96f, 0.93f, 1.0f);

    ImVec4* c = style.Colors;
    c[ImGuiCol_Text] = text;
    c[ImGuiCol_TextDisabled] = ImVec4(0.62f, 0.64f, 0.72f, 1.0f);
    c[ImGuiCol_WindowBg] = navy;
    c[ImGuiCol_PopupBg] = navy;
    c[ImGuiCol_Border] = ImVec4(1.0f, 1.0f, 1.0f, 0.08f);
    c[ImGuiCol_FrameBg] = slate;
    c[ImGuiCol_FrameBgHovered] = slate_hi;
    c[ImGuiCol_FrameBgActive] = slate_hi;
    c[ImGuiCol_TitleBg] = ImVec4(0.10f, 0.13f, 0.22f, 0.95f);
    c[ImGuiCol_TitleBgActive] = ImVec4(0.13f, 0.17f, 0.28f, 1.0f);
    c[ImGuiCol_TitleBgCollapsed] = navy;
    c[ImGuiCol_Button] = slate;
    c[ImGuiCol_ButtonHovered] = orange;
    c[ImGuiCol_ButtonActive] = orange_hi;
    c[ImGuiCol_Header] = slate;
    c[ImGuiCol_HeaderHovered] = orange;
    c[ImGuiCol_HeaderActive] = orange_hi;
    c[ImGuiCol_SliderGrab] = orange;
    c[ImGuiCol_SliderGrabActive] = orange_hi;
    c[ImGuiCol_CheckMark] = orange_hi;
    c[ImGuiCol_Separator] = ImVec4(1.0f, 1.0f, 1.0f, 0.14f);
    c[ImGuiCol_PlotHistogram] = orange;
    c[ImGuiCol_PlotHistogramHovered] = orange_hi;
    c[ImGuiCol_ResizeGrip] = ImVec4(0, 0, 0, 0);
}

// Uses Windows' Bahnschrift (a sporty DIN face) or Segoe UI if present,
// otherwise ImGui's built-in font. ImGui 1.92 rasterises glyphs at whatever
// size is asked for, so scaled text stays sharp.
void load_font() {
    ImGuiIO& io = ImGui::GetIO();
    for (const char* path : {"C:/Windows/Fonts/bahnschrift.ttf", "C:/Windows/Fonts/segoeui.ttf",
                             "/system/fonts/Roboto-Regular.ttf"}) {
        if (FILE* f = std::fopen(path, "rb")) {
            std::fclose(f);
            if (io.Fonts->AddFontFromFileTTF(path, 18.0f)) return;
        }
    }
    io.Fonts->AddFontDefault();
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
    apply_style();
    linearize_style();
    load_font();
    g_scale = info.scale > 0.0f ? info.scale : 1.0f;
    ImGui::GetStyle().ScaleAllSizes(g_scale);
    ImGui::GetStyle().FontScaleMain = g_scale;

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

// Rotates finished draw data from the upright display into the swapchain's
// native orientation (Android pre-rotation). Vertices and clip rectangles are
// remapped; a 90-degree turn keeps rectangles axis-aligned.
void rotate_draw_data(ImDrawData* data, VkSurfaceTransformFlagBitsKHR transform) {
    const float w = data->DisplaySize.x, h = data->DisplaySize.y;
    auto map = [&](ImVec2 p) -> ImVec2 {
        switch (transform) {
        case VK_SURFACE_TRANSFORM_ROTATE_90_BIT_KHR:  return ImVec2(h - p.y, p.x);
        case VK_SURFACE_TRANSFORM_ROTATE_180_BIT_KHR: return ImVec2(w - p.x, h - p.y);
        case VK_SURFACE_TRANSFORM_ROTATE_270_BIT_KHR: return ImVec2(p.y, w - p.x);
        default:                                      return p;
        }
    };
    for (ImDrawList* list : data->CmdLists) {
        for (ImDrawVert& v : list->VtxBuffer) v.pos = map(v.pos);
        for (ImDrawCmd& c : list->CmdBuffer) {
            const ImVec2 a = map(ImVec2(c.ClipRect.x, c.ClipRect.y));
            const ImVec2 b = map(ImVec2(c.ClipRect.z, c.ClipRect.w));
            c.ClipRect = ImVec4(std::min(a.x, b.x), std::min(a.y, b.y), std::max(a.x, b.x), std::max(a.y, b.y));
        }
    }
    if (transform == VK_SURFACE_TRANSFORM_ROTATE_90_BIT_KHR || transform == VK_SURFACE_TRANSFORM_ROTATE_270_BIT_KHR) {
        data->DisplaySize = ImVec2(h, w);
    }
}

void ui_frame(VkCommandBuffer cmd, VkExtent2D display, VkSurfaceTransformFlagBitsKHR transform,
              const RUIInput& input, RUICmd* cmds, uint32_t count, const char* text, uint32_t text_length,
              RUIOutput* out) {
    if (!g_ready) return;

    ImGuiIO& io = ImGui::GetIO();
    io.DisplaySize = ImVec2(static_cast<float>(display.width), static_cast<float>(display.height));
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
    bool        font_pushed = false;
    bool        shadowed = false; // text gets a drop shadow (transparent window)
    bool        centered = false; // centre text and buttons
    auto        end_window = [&] {
        if (font_pushed) ImGui::PopFont();
        ImGui::End();
        window_open = collapsed = font_pushed = shadowed = centered = false;
    };
    // Moves the cursor so an item `width` wide is centred in the window.
    auto centre = [&](float width) {
        if (!centered) return;
        const float avail = ImGui::GetContentRegionAvail().x;
        if (avail > width) ImGui::SetCursorPosX(ImGui::GetCursorPosX() + (avail - width) * 0.5f);
    };
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
            if (window_open) end_window(); // tolerate a missing R_UI_END
            const uint32_t   options = static_cast<uint32_t>(c.value);
            ImGuiWindowFlags flags = ImGuiWindowFlags_AlwaysAutoResize;
            if (options & R_UI_WINDOW_OVERLAY) {
                flags |= ImGuiWindowFlags_NoTitleBar | ImGuiWindowFlags_NoMove | ImGuiWindowFlags_NoResize |
                         ImGuiWindowFlags_NoCollapse | ImGuiWindowFlags_NoSavedSettings;
            }
            if (options & R_UI_WINDOW_NO_BACKGROUND) flags |= ImGuiWindowFlags_NoBackground;
            if (options & R_UI_WINDOW_ANCHORED) {
                ImGui::SetNextWindowPos(ImVec2(c.x * io.DisplaySize.x, c.y * io.DisplaySize.y), ImGuiCond_Always,
                                        ImVec2(c.x, c.y));
            } else if (c.x != 0.0f || c.y != 0.0f) {
                ImGui::SetNextWindowPos(ImVec2(c.x, c.y), ImGuiCond_FirstUseEver);
            }
            collapsed = !ImGui::Begin(label.empty() ? "##window" : label.c_str(), nullptr, flags);
            window_open = true;
            shadowed = (options & R_UI_WINDOW_NO_BACKGROUND) != 0;
            centered = (options & R_UI_WINDOW_CENTERED) != 0;
            if (c.max > 0.0f && c.max != 1.0f) {
                // FontSizeBase is unscaled; the style's FontScaleMain (UI scale) applies on top.
                ImGui::PushFont(nullptr, ImGui::GetStyle().FontSizeBase * c.max); // rasterised at that size
                font_pushed = true;
            }
            continue;
        }
        if (c.kind == R_UI_END) {
            if (window_open) end_window();
            continue;
        }
        if (collapsed) continue;

        switch (c.kind) {
        case R_UI_TEXT: {
            const char* begin = label.data();
            const char* end = begin + label.size();
            centre(ImGui::CalcTextSize(begin, end).x);
            if (shadowed) {
                const float  off = std::max(1.0f, std::round(ImGui::GetFontSize() / 14.0f));
                const ImVec2 at = ImGui::GetCursorScreenPos();
                const float  alpha = c.max > 0.0f ? c.max : 1.0f;
                ImGui::GetWindowDrawList()->AddText(ImVec2(at.x + off, at.y + off),
                                                    ImGui::GetColorU32(ImVec4(0.02f, 0.03f, 0.08f, 0.55f * alpha)),
                                                    begin, end);
            }
            if (c.max > 0.0f) ImGui::PushStyleColor(ImGuiCol_Text, ImVec4(c.x, c.y, c.min, c.max));
            ImGui::TextUnformatted(begin, end);
            if (c.max > 0.0f) ImGui::PopStyleColor();
            break;
        }
        case R_UI_SLIDER: {
            std::string format = "%.3f";
            if (const size_t sep = label.find('\x1f'); sep != std::string::npos) {
                format = label.substr(sep + 1);
                label.resize(sep);
            }
            const float width = c.y > 0.0f ? c.y * g_scale : ImGui::CalcItemWidth();
            centre(width + ImGui::GetStyle().ItemInnerSpacing.x +
                   ImGui::CalcTextSize(label.c_str(), nullptr, true).x);
            ImGui::SetNextItemWidth(width);
            const bool highlight = c.x != 0.0f;
            if (highlight) {
                ImVec4 selected = ImGui::GetStyleColorVec4(ImGuiCol_ButtonHovered);
                selected.w = 0.55f;
                ImGui::PushStyleColor(ImGuiCol_FrameBg, selected);
            }
            float v = c.value;
            if (ImGui::SliderFloat(label.c_str(), &v, c.min, c.max, format.c_str())) {
                c.value = v;
                c.result = 1;
            }
            if (highlight) ImGui::PopStyleColor();
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
        case R_UI_BUTTON: {
            const bool   highlight = c.value != 0.0f;
            const ImVec2 text = ImGui::CalcTextSize(label.c_str(), nullptr, true);
            const ImVec2 size(c.min * g_scale, c.max * g_scale);
            centre(size.x > 0.0f ? size.x : text.x + ImGui::GetStyle().FramePadding.x * 2.0f);
            if (highlight) ImGui::PushStyleColor(ImGuiCol_Button, ImGui::GetStyleColorVec4(ImGuiCol_ButtonHovered));
            if (ImGui::Button(label.c_str(), size)) c.result = 1;
            if (highlight) ImGui::PopStyleColor();
            break;
        }
        case R_UI_SEPARATOR:
            ImGui::Separator();
            break;
        case R_UI_PROGRESS: {
            centre(c.min * g_scale);
            ImGui::ProgressBar(std::clamp(c.value, 0.0f, 1.0f),
                               ImVec2(c.min > 0.0f ? c.min * g_scale : -FLT_MIN, c.max * g_scale), label.c_str());
            break;
        }
        case R_UI_SAME_LINE:
            ImGui::SameLine(0.0f, c.value > 0.0f ? c.value * g_scale : -1.0f);
            break;
        default:
            break;
        }
    }
    if (window_open) end_window();

    ImGui::Render();
    rotate_draw_data(ImGui::GetDrawData(), transform);
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
