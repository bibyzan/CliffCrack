#include "ui.h"

#include <imgui.h>
#include <imgui_impl_vulkan.h>

#ifdef __APPLE__
#include <CoreText/CoreText.h>
#endif

#include <algorithm>
#include <cfloat>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <string>
#include <unordered_map>

namespace {

bool     g_ready = false;
VkImageView (*g_texture_view)(uint32_t) = nullptr;
std::unordered_map<uint32_t, VkDescriptorSet> g_images; // RTexture -> ImGui's descriptor set for it
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

#ifdef __APPLE__
// The file behind an installed font (iOS keeps them outside any fixed path).
std::string font_file(const char* name) {
    std::string    path;
    CFStringRef    cf_name = CFStringCreateWithCString(nullptr, name, kCFStringEncodingUTF8);
    CTFontRef      font = CTFontCreateWithName(cf_name, 18.0, nullptr);
    CFURLRef       url = static_cast<CFURLRef>(CTFontCopyAttribute(font, kCTFontURLAttribute));
    char           buf[1024];
    if (url && CFURLGetFileSystemRepresentation(url, true, reinterpret_cast<UInt8*>(buf), sizeof buf)) path = buf;
    if (url) CFRelease(url);
    CFRelease(font);
    CFRelease(cf_name);
    return path;
}
#endif

// Uses Windows' Bahnschrift (a sporty DIN face) or Segoe UI if present (DIN
// Alternate on iOS, Roboto on Android), otherwise ImGui's built-in font.
// ImGui 1.92 rasterises glyphs at whatever size is asked for, so scaled text
// stays sharp.
void load_font() {
    ImGuiIO& io = ImGui::GetIO();
#ifdef __APPLE__
    // CoreText substitutes a fallback for a missing name, so check it's a .ttf.
    if (const std::string din = font_file("DINAlternate-Bold");
        din.size() > 4 && din.compare(din.size() - 4, 4, ".ttf") == 0 &&
        io.Fonts->AddFontFromFileTTF(din.c_str(), 18.0f)) {
        // Glyphs DIN lacks (the · in taglines) come from Helvetica Neue.
        if (const std::string fallback = font_file("HelveticaNeue"); !fallback.empty()) {
            ImFontConfig merge;
            merge.MergeMode = true;
            io.Fonts->AddFontFromFileTTF(fallback.c_str(), 18.0f, &merge);
        }
        return;
    }
#endif
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
    // Backend-owned pool: the font atlas, and the game's textures used as images.
    init.DescriptorPoolSize = IMGUI_IMPL_VULKAN_MINIMUM_SAMPLED_IMAGE_POOL_SIZE + 128;
    g_texture_view = info.texture_view;
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

constexpr float kPi = 3.14159265358979f;

// Colour from sRGB components, for draw-list drawing (the swapchain is sRGB).
static ImU32 srgb(float r, float g, float b, float a = 1.0f) {
    return ImGui::GetColorU32(ImVec4(srgb_to_linear(r), srgb_to_linear(g), srgb_to_linear(b), a));
}

static ImU32 lerp_color(ImU32 a, ImU32 b, float t) {
    const ImVec4 x = ImGui::ColorConvertU32ToFloat4(a), y = ImGui::ColorConvertU32ToFloat4(b);
    return ImGui::GetColorU32(ImVec4(x.x + (y.x - x.x) * t, x.y + (y.y - x.y) * t, x.z + (y.z - x.z) * t,
                                     x.w + (y.w - x.w) * t));
}

// Draws a speedometer-style dial at the cursor: a 270-degree arc open at the
// bottom, filled up to the value in colours running white -> orange -> red,
// with ticks, a needle and the value in the middle.
static void draw_gauge(const RUICmd& c, const std::string& unit) {
    const float  size = (c.x > 0.0f ? c.x : 200.0f) * g_scale;
    const ImVec2 origin = ImGui::GetCursorScreenPos();
    ImGui::Dummy(ImVec2(size, size));
    ImDrawList* dl = ImGui::GetWindowDrawList();

    const float  range = c.max > c.min ? c.max - c.min : 1.0f;
    const float  t = std::clamp((c.value - c.min) / range, 0.0f, 1.0f);
    const ImVec2 centre(origin.x + size * 0.5f, origin.y + size * 0.5f);
    const float  radius = size * 0.44f;
    const float  thick = size * 0.075f;
    const float  start = 0.75f * kPi, sweep = 1.5f * kPi; // from bottom-left, clockwise to bottom-right
    auto angle = [&](float f) { return start + sweep * f; };
    auto at = [&](float a, float r) { return ImVec2(centre.x + std::cos(a) * r, centre.y + std::sin(a) * r); };

    const ImU32 white = srgb(1.0f, 1.0f, 1.0f), orange = srgb(0.96f, 0.52f, 0.16f), red = srgb(1.0f, 0.30f, 0.18f);
    auto heat = [&](float f) { return f < 0.5f ? lerp_color(white, orange, f * 2.0f) : lerp_color(orange, red, f * 2.0f - 1.0f); };

    // Backing disc and track.
    dl->AddCircleFilled(centre, radius + thick * 1.1f, srgb(0.05f, 0.07f, 0.14f, 0.72f), 64);
    dl->PathArcTo(centre, radius, angle(0.0f), angle(1.0f), 64);
    dl->PathStroke(srgb(1.0f, 1.0f, 1.0f, 0.14f), thick);
    if (c.y > 0.0f && c.y < 1.0f) { // red zone
        dl->PathArcTo(centre, radius, angle(c.y), angle(1.0f), 24);
        dl->PathStroke(srgb(1.0f, 0.30f, 0.18f, 0.35f), thick);
    }
    // The filled part, in short segments so the colour can run along it.
    const int segments = std::max(1, static_cast<int>(t * 48.0f));
    for (int i = 0; i < segments && t > 0.0f; ++i) {
        const float f0 = t * i / segments, f1 = t * (i + 1) / segments;
        dl->PathArcTo(centre, radius, angle(f0), angle(f1) + 0.01f, 4);
        dl->PathStroke(heat(f1), thick);
    }

    // Ticks every tenth of the range (labelled every other), small ones between.
    ImFont* font = ImGui::GetFont();
    const float label_size = size * 0.085f;
    for (int i = 0; i <= 20; ++i) {
        const float f = i / 20.0f;
        const bool  major = i % 2 == 0;
        const float a = angle(f);
        const float r0 = radius - thick * (major ? 1.25f : 0.9f), r1 = radius - thick * 0.55f;
        dl->AddLine(at(a, r0), at(a, r1), srgb(1.0f, 1.0f, 1.0f, major ? 0.8f : 0.4f), major ? 2.0f * g_scale : 1.0f * g_scale);
        if (major && i % 4 == 0 && i > 0 && i < 20) { // the ends would crowd the readout
            char text[16];
            std::snprintf(text, sizeof text, "%.0f", c.min + range * f);
            const ImVec2 ts = font->CalcTextSizeA(label_size, FLT_MAX, 0.0f, text);
            const ImVec2 p = at(a, radius - thick * 2.2f);
            dl->AddText(font, label_size, ImVec2(p.x - ts.x * 0.5f, p.y - ts.y * 0.5f), srgb(1.0f, 1.0f, 1.0f, 0.7f), text);
        }
    }

    // Needle with a hub.
    const float a = angle(t);
    dl->AddLine(at(a + kPi, radius * 0.12f), at(a, radius * 0.92f), heat(t), size * 0.022f);
    dl->AddCircleFilled(centre, size * 0.04f, heat(t), 24);
    dl->AddCircleFilled(centre, size * 0.02f, srgb(0.05f, 0.07f, 0.14f), 16);

    // The value, big, with the unit under it, low in the dial's open bottom.
    char value[16];
    std::snprintf(value, sizeof value, "%.0f", c.value);
    const float  value_size = size * 0.2f, unit_size = size * 0.075f;
    const ImVec2 vs = font->CalcTextSizeA(value_size, FLT_MAX, 0.0f, value);
    const ImVec2 vp(centre.x - vs.x * 0.5f, centre.y + radius * 0.3f);
    dl->AddText(font, value_size, ImVec2(vp.x + 2.0f, vp.y + 2.0f), srgb(0.02f, 0.03f, 0.08f, 0.55f), value);
    dl->AddText(font, value_size, vp, heat(t), value);
    const ImVec2 us = font->CalcTextSizeA(unit_size, FLT_MAX, 0.0f, unit.c_str());
    dl->AddText(font, unit_size, ImVec2(centre.x - us.x * 0.5f, vp.y + vs.y * 0.92f), srgb(0.86f, 0.88f, 0.96f, 0.85f),
                unit.c_str());
}

// Draws a disc or ring behind every window (on-screen touch controls), with
// its label centred in it.
static void draw_circle(const RUICmd& c, uint32_t packed, const std::string& label) {
    auto channel = [&](int shift) { return static_cast<float>((packed >> shift) & 0xffu) / 255.0f; };
    const ImU32  color = srgb(channel(0), channel(8), channel(16), channel(24));
    const ImVec2 centre(c.x, c.y);
    ImDrawList*  dl = ImGui::GetBackgroundDrawList();
    if (c.min > 0.0f) {
        dl->AddCircle(centre, c.value, color, 0, c.min);
    } else {
        dl->AddCircleFilled(centre, c.value, color);
    }
    if (label.empty()) return;
    ImFont*      font = ImGui::GetFont();
    const float  size = c.max > 0.0f ? c.max : ImGui::GetFontSize();
    const ImVec2 ts = font->CalcTextSizeA(size, FLT_MAX, 0.0f, label.c_str());
    dl->AddText(font, size, ImVec2(centre.x - ts.x * 0.5f, centre.y - ts.y * 0.5f), srgb(1.0f, 1.0f, 1.0f, 0.9f),
                label.c_str());
}

// Draws an RTexture (e.g. an icon), tinted, behind every window.
static void draw_image(const RUICmd& c, uint32_t packed) {
    const uint32_t texture = static_cast<uint32_t>(c.max);
    auto           it = g_images.find(texture);
    if (it == g_images.end()) {
        const VkImageView view = g_texture_view ? g_texture_view(texture) : VK_NULL_HANDLE;
        if (!view || g_images.size() >= 128) return;
        it = g_images.emplace(texture, ImGui_ImplVulkan_AddTexture(view, VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL)).first;
    }
    auto        channel = [&](int shift) { return static_cast<float>((packed >> shift) & 0xffu) / 255.0f; };
    const float hw = c.min * 0.5f, hh = c.value * 0.5f;
    ImGui::GetBackgroundDrawList()->AddImage(ImTextureRef(reinterpret_cast<ImTextureID>(it->second)),
                                             ImVec2(c.x - hw, c.y - hh), ImVec2(c.x + hw, c.y + hh), ImVec2(0, 0),
                                             ImVec2(1, 1), srgb(channel(0), channel(8), channel(16), channel(24)));
}

void ui_forget_texture(uint32_t texture) {
    if (auto it = g_images.find(texture); it != g_images.end()) {
        ImGui_ImplVulkan_RemoveTexture(it->second);
        g_images.erase(it);
    }
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

#ifdef __APPLE__
// Metal GPUs of the older families (the iOS Simulator's among them) can't
// draw with a base vertex, and the ImGui backend draws each list with one:
// its offset into the vertex buffer shared by all the lists. So every list's
// vertices move into the last list, the others' indices are rebased onto
// them (ImDrawIdx is 32-bit on Apple, see CMakeLists.txt), and each list is
// then drawn with a base vertex of 0.
static_assert(sizeof(ImDrawIdx) == 4, "flatten_vertices needs 32-bit ImGui indices");

void flatten_vertices(ImDrawData* data) {
    if (data->CmdLists.Size < 2) return;
    ImDrawList*          last = data->CmdLists.back();
    ImVector<ImDrawVert> all;
    all.resize(data->TotalVtxCount);
    int base = 0;
    for (ImDrawList* list : data->CmdLists) {
        for (ImDrawIdx& i : list->IdxBuffer) i += static_cast<ImDrawIdx>(base);
        for (ImDrawCmd& c : list->CmdBuffer) c.VtxOffset = 0;
        std::memcpy(all.Data + base, list->VtxBuffer.Data, sizeof(ImDrawVert) * static_cast<size_t>(list->VtxBuffer.Size));
        base += list->VtxBuffer.Size;
        if (list != last) list->VtxBuffer.resize(0);
    }
    last->VtxBuffer.swap(all);
}
#endif

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
        const uint32_t packed = c.result; // R_UI_CIRCLE's colour comes in here
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
        if (c.kind == R_UI_CIRCLE) {
            draw_circle(c, packed, label);
            continue;
        }
        if (c.kind == R_UI_IMAGE) {
            draw_image(c, packed);
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
        case R_UI_GAUGE:
            draw_gauge(c, label);
            break;
        default:
            break;
        }
    }
    if (window_open) end_window();

    ImGui::Render();
    rotate_draw_data(ImGui::GetDrawData(), transform);
#ifdef __APPLE__
    flatten_vertices(ImGui::GetDrawData());
#endif
    ImGui_ImplVulkan_RenderDrawData(ImGui::GetDrawData(), cmd);

    if (out) {
        out->want_mouse = io.WantCaptureMouse ? 1 : 0;
        out->want_keyboard = io.WantCaptureKeyboard ? 1 : 0;
    }
}

void ui_shutdown() {
    if (!g_ready) return;
    for (auto& [texture, set] : g_images) ImGui_ImplVulkan_RemoveTexture(set);
    g_images.clear();
    ImGui_ImplVulkan_Shutdown();
    ImGui::DestroyContext();
    g_ready = false;
}
