// Android bits: the Vulkan surface comes from an ANativeWindow, and messages
// go to logcat (stdout/stderr are discarded on Android).
#include "vk_common.h"

#include <android/log.h>
#include <android/native_window.h>

// The GPU driver's Vulkan loader, found by volk.
VkResult platform_load_vulkan() { return volkInitialize(); }

bool platform_create_surface(VkInstance instance, void* native_window, VkSurfaceKHR* surface) {
    VkAndroidSurfaceCreateInfoKHR info{VK_STRUCTURE_TYPE_ANDROID_SURFACE_CREATE_INFO_KHR};
    info.window = static_cast<ANativeWindow*>(native_window);
    return vkCreateAndroidSurfaceKHR(instance, &info, nullptr, surface) == VK_SUCCESS;
}

void platform_log(const char* message) {
    __android_log_write(ANDROID_LOG_INFO, "CliffCrack", message);
}
