// Debug UI: Dear ImGui driven by the RUICmd command list (see renderer.h).
#pragma once

#include "vk_common.h"

#include "renderer.h"

#include <string>

struct UiInitInfo {
    VkInstance       instance;
    VkPhysicalDevice physical_device;
    VkDevice         device;
    uint32_t         queue_family;
    VkQueue          queue;
    uint32_t         image_count;
    VkFormat         color_format;
    VkFormat         depth_format;
    float            scale; // UI size multiplier (1 = desktop)
    VkImageView (*texture_view)(uint32_t texture); // an RTexture's view (VK_NULL_HANDLE if none), for R_UI_IMAGE
};

// Returns false (with a message in *error) if the UI can't be created; the
// renderer keeps working without it.
bool ui_init(const UiInitInfo& info, std::string* error);

// Rebuilds the UI pipeline if the attachment formats changed.
void ui_set_formats(VkFormat color_format, VkFormat depth_format);

// Runs the command list and records the UI into cmd, which must be inside a
// dynamic rendering pass with the formats given above. The UI is laid out in
// `display` (what the player sees); `transform` is the swapchain's
// pre-rotation, applied to the finished draw data so it lands upright in the
// rotated image.
void ui_frame(VkCommandBuffer cmd, VkExtent2D display, VkSurfaceTransformFlagBitsKHR transform,
              const RUIInput& input, RUICmd* cmds, uint32_t count, const char* text, uint32_t text_length,
              RUIOutput* out);

// Drops the UI's hold on an RTexture that's about to be destroyed. The device
// must be idle.
void ui_forget_texture(uint32_t texture);

// The device must be idle.
void ui_shutdown();
