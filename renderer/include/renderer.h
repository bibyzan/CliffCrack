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

// Interleaved vertex. Must match geom.Vertex in Go.
typedef struct RVertex {
    float position[3];
    float normal[3];
    float uv[2];
} RVertex;

// Opaque mesh handle; 0 is never a valid mesh.
typedef uint32_t RMesh;

// Texture handle. 0 is a built-in 1x1 white texture, so "untextured" needs no
// special case: the draw colour is used as-is.
typedef uint32_t RTexture;

enum {
    R_TEXTURE_SRGB = 1 << 0, // pixel data is sRGB-encoded colour (base colour maps)
};

// Per-frame scene data. Must match render.frameParams in Go.
// Everything except clear_color becomes the shaders' per-frame uniform buffer.
typedef struct RFrameParams {
    float view_proj[16];     // column-major, Vulkan clip space
    float camera_pos[4];     // world space, w unused
    float sun_direction[4];  // world space, *towards* the light, w unused
    float sun_color[4];      // linear RGB * intensity, w unused
    float ambient_color[4];  // linear RGB, w unused
    float clear_color[4];    // linear RGBA
} RFrameParams;

// One draw of one mesh. Must match render.DrawCmd in Go.
// Everything before `mesh` (84 bytes) is sent as shader push constants.
typedef struct RDrawCmd {
    float    model[16]; // column-major object-to-world transform
    float    color[4];  // linear RGBA, multiplied with the texture
    RTexture texture;
    RMesh    mesh;
} RDrawCmd;

// Returns 1 on success, 0 on failure (see r_last_error).
R_API int32_t r_init(const RInitDesc* desc);

// Message for the most recent failure. Valid until the next renderer call.
R_API const char* r_last_error(void);

// Call when the framebuffer size changes. The swapchain is rebuilt lazily.
R_API void r_resize(uint32_t width, uint32_t height);

// Uploads a triangle-list mesh to GPU memory (blocking). The arrays are copied,
// so the caller may free them immediately. Returns 0 on failure.
R_API RMesh r_create_mesh(const RVertex* vertices, uint32_t vertex_count,
                          const uint32_t* indices, uint32_t index_count);

// Frees a mesh. Waits for the GPU to go idle, so avoid calling it every frame.
// Any meshes still alive at r_shutdown are freed there.
R_API void r_destroy_mesh(RMesh mesh);

// Uploads tightly packed 8-bit RGBA pixels and builds a full mip chain
// (blocking). flags is a combination of R_TEXTURE_*. Returns 0 on failure.
R_API RTexture r_create_texture(const uint8_t* rgba, uint32_t width, uint32_t height, uint32_t flags);

// Frees a texture. Waits for the GPU to go idle. Destroying handle 0 is a no-op.
R_API void r_destroy_texture(RTexture texture);

// Starts a frame. Returns 0 if the frame should be skipped (minimized, swapchain
// being rebuilt); in that case do not call r_draw / r_end_frame.
R_API int32_t r_begin_frame(const RFrameParams* params);

R_API void r_draw(const RDrawCmd* cmds, uint32_t count);

R_API void r_end_frame(void);

R_API void r_shutdown(void);

#ifdef __cplusplus
}
#endif

#endif // VKGAME_RENDERER_H
