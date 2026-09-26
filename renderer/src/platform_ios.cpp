// iOS bits: Vulkan is MoltenVK (Vulkan on Metal), shipped inside the app as
// Frameworks/MoltenVK.framework, and the surface comes from the game view's
// CAMetalLayer.
#include "vk_common.h"

#include <dlfcn.h>

#include <cstdio>

// volk only looks in the usual library locations, not in the app bundle, so
// MoltenVK is opened here and volk is handed its entry point.
VkResult platform_load_vulkan() {
    void* moltenvk = dlopen("@executable_path/Frameworks/MoltenVK.framework/MoltenVK", RTLD_NOW | RTLD_LOCAL);
    if (!moltenvk) {
        std::fprintf(stderr, "[renderer] can't load MoltenVK: %s\n", dlerror());
        return VK_ERROR_INITIALIZATION_FAILED;
    }
    auto get_proc = reinterpret_cast<PFN_vkGetInstanceProcAddr>(dlsym(moltenvk, "vkGetInstanceProcAddr"));
    if (!get_proc) return VK_ERROR_INITIALIZATION_FAILED;
    volkInitializeCustom(get_proc);
    return VK_SUCCESS;
}

// native_window is the view's CAMetalLayer.
bool platform_create_surface(VkInstance instance, void* native_window, VkSurfaceKHR* surface) {
    VkMetalSurfaceCreateInfoEXT info{VK_STRUCTURE_TYPE_METAL_SURFACE_CREATE_INFO_EXT};
    info.pLayer = static_cast<const CAMetalLayer*>(native_window);
    return vkCreateMetalSurfaceEXT(instance, &info, nullptr, surface) == VK_SUCCESS;
}

// stderr shows in Xcode's console and in `xcrun simctl launch --console`.
void platform_log(const char* message) {
    std::fprintf(stderr, "%s\n", message);
}
