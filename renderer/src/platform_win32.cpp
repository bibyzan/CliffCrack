// Win32 bits kept out of renderer.cpp so it doesn't pull in <windows.h>.
#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>

#include "vk_common.h"

#include <cstdio>

// The GPU driver's Vulkan loader, found by volk.
VkResult platform_load_vulkan() { return volkInitialize(); }

bool platform_create_surface(VkInstance instance, void* native_window, VkSurfaceKHR* surface) {
    VkWin32SurfaceCreateInfoKHR info{VK_STRUCTURE_TYPE_WIN32_SURFACE_CREATE_INFO_KHR};
    info.hinstance = GetModuleHandleW(nullptr);
    info.hwnd = static_cast<HWND>(native_window);
    return vkCreateWin32SurfaceKHR(instance, &info, nullptr, surface) == VK_SUCCESS;
}

void platform_log(const char* message) {
    std::fprintf(stderr, "%s\n", message);
}
