// renderer.h - the C API between the Go host and the native Vulkan renderer.
//
// Rules for this boundary (keeps cgo cheap and legal):
//   * Coarse-grained calls: a handful per frame, never one per object.
//   * Only plain-old-data crosses it: handles, flat arrays, fixed-size structs.
//   * The renderer never stores a pointer it was given; it copies what it needs.
#ifndef VKGAME_RENDERER_H
#define VKGAME_RENDERER_H

#include <stdint.h>

#if defined(_WIN32) && defined(RENDERER_BUILD)
#define R_API __declspec(dllexport)
#elif defined(RENDERER_BUILD)
#define R_API __attribute__((visibility("default")))
#else
#define R_API
#endif

#ifdef __cplusplus
extern "C" {
#endif

typedef struct RInitDesc {
    void*       native_window;     // HWND on Windows
    uint32_t    width;             // framebuffer size in pixels
    uint32_t    height;
    const char* shader_dir;        // directory containing the compiled *.spv files
    int32_t     enable_validation; // non-zero: request Vulkan validation layers
    int32_t     vsync;             // non-zero: FIFO present mode
} RInitDesc;

// One draw of the built-in test triangle. Must match render.DrawCmd in Go.
typedef struct RDrawCmd {
    float mvp[16];  // column-major model-view-projection (Vulkan clip space)
    float color[4]; // RGBA tint
} RDrawCmd;

// Returns 1 on success, 0 on failure (see r_last_error).
R_API int32_t r_init(const RInitDesc* desc);

// Message for the most recent failure. Valid until the next renderer call.
R_API const char* r_last_error(void);

// Call when the framebuffer size changes. The swapchain is rebuilt lazily.
R_API void r_resize(uint32_t width, uint32_t height);

// Starts a frame. Returns 0 if the frame should be skipped (minimized, swapchain
// being rebuilt); in that case do not call r_draw / r_end_frame.
R_API int32_t r_begin_frame(const float clear_color[4]);

R_API void r_draw(const RDrawCmd* cmds, uint32_t count);

R_API void r_end_frame(void);

R_API void r_shutdown(void);

#ifdef __cplusplus
}
#endif

#endif // VKGAME_RENDERER_H
