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
    void*       native_window;     // HWND on Windows, ANativeWindow* on Android
    uint32_t    width;             // framebuffer size in pixels
    uint32_t    height;
    const char* shader_dir;        // directory containing the compiled *.spv files
    int32_t     enable_validation; // non-zero: request Vulkan validation layers
    int32_t     vsync;             // non-zero: FIFO present mode
    float       ui_scale;          // UI size multiplier for dense screens (0 = 1)
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
    float fog_color[4];      // linear RGB haze/horizon colour, w = density per unit (0 = no fog)
    float clear_color[4];    // linear RGBA
} RFrameParams;

// Per-draw shading options (RDrawCmd::flags).
enum {
    R_DRAW_FLAT  = 1 << 0, // faceted: one normal per triangle, from screen-space derivatives
    R_DRAW_SNOW  = 1 << 1, // colour only faces that point up; steep faces become darker rock
    R_DRAW_UNLIT = 1 << 2, // colour as-is, no lighting (still fogged)
    R_DRAW_SKY   = 1 << 3, // procedural sky: fog colour at the horizon to `color` overhead, sun disc; no fog
};

// One draw of one mesh. Must match render.DrawCmd in Go.
// Everything before `mesh` (88 bytes) is sent as shader push constants.
typedef struct RDrawCmd {
    float    model[16]; // column-major object-to-world transform
    float    color[4];  // linear RGBA, multiplied with the texture
    RTexture texture;
    uint32_t flags;     // R_DRAW_*
    RMesh    mesh;
} RDrawCmd;

// ---- Debug UI (Dear ImGui) -------------------------------------------------
// The UI is described as a flat command list each frame, drawn by the renderer
// in a single call. Labels live in one shared text buffer (no pointers inside
// the commands), and widget results are written back into the same commands.

enum {
    R_UI_WINDOW = 1,    // begin a window: label = title, x/y = initial position (0,0 = auto),
                        //   value = R_UI_WINDOW_* flags, max = font scale (0 = 1)
    R_UI_END,           // end the current window
    R_UI_TEXT,          // label = text; if max > 0, drawn in colour (x, y, min, max) = linear RGBA
    R_UI_SLIDER,        // float slider: value in/out, min..max; x != 0 highlights it, y = width in
                        //   pixels (0 = default). The label may end in "\x1f<printf format>" for
                        //   how the value is shown (default "%.3f").
    R_UI_CHECKBOX,      // value in/out: 0 or 1
    R_UI_BUTTON,        // result = 1 on the frame it was clicked; value != 0 highlights it,
                        //   min/max = size in pixels (0 = fit the label)
    R_UI_SEPARATOR,
    R_UI_PROGRESS,      // bar filled to value (0..1), min/max = size in pixels (0 = default), label overlaid
    R_UI_SAME_LINE,     // keep the next widget on this line, value = spacing in pixels (0 = default)
    R_UI_GAUGE,         // dial: value within min..max (shown as the big number), x = diameter in
                        //   pixels, y = where the red zone starts (fraction of the range, 0 = none),
                        //   label = the unit under the number
    R_UI_CIRCLE,        // disc behind every window (e.g. touch controls): x/y = centre and value =
                        //   radius in pixels, min = ring width (0 = filled), result (in) = colour as
                        //   packed sRGB 0xAABBGGRR; label centred in it, max = text size (0 = default)
};

// Window options, passed in the R_UI_WINDOW command's value.
enum {
    R_UI_WINDOW_OVERLAY   = 1 << 0, // no title bar, can't be moved, resized or collapsed
    R_UI_WINDOW_ANCHORED  = 1 << 1, // x/y are fractions of the screen, kept there every frame;
                                    //   the same fraction of the window sits on that point
    R_UI_WINDOW_NO_BACKGROUND = 1 << 2, // transparent; text gets a drop shadow to stay readable
    R_UI_WINDOW_CENTERED  = 1 << 3,     // centre each line of text and each button in the window
};

// Must match gfx.UICmd in Go.
typedef struct RUICmd {
    uint32_t kind;
    uint32_t label_offset; // into the text buffer
    uint32_t label_length;
    uint32_t result;       // out: 1 if the widget was changed/clicked this frame
    float    value;        // in/out
    float    min, max;
    float    x, y;
} RUICmd;

// Must match gfx.UIInput in Go.
typedef struct RUIInput {
    float    mouse_x, mouse_y; // framebuffer pixels; negative = no mouse (e.g. cursor captured)
    float    wheel;            // vertical scroll this frame
    uint32_t mouse_buttons;    // bit 0 left, bit 1 right, bit 2 middle
    float    delta_time;       // seconds
} RUIInput;

// Must match gfx.UIOutput in Go.
typedef struct RUIOutput {
    int32_t want_mouse;        // the UI is using the mouse: don't send it to the game
    int32_t want_keyboard;
} RUIOutput;

// Returns 1 on success, 0 on failure (see r_last_error).
R_API int32_t r_init(const RInitDesc* desc);

// Message for the most recent failure. Valid until the next renderer call.
R_API const char* r_last_error(void);

// Call when the framebuffer size changes. The swapchain is rebuilt lazily.
R_API void r_resize(uint32_t width, uint32_t height);

// Replaces the native window (Android: the OS destroys it when the app goes to
// the background and makes a new one when it returns). NULL drops the surface;
// frames are skipped until a window is set again. Call before the old window
// is released.
R_API void r_set_window(void* native_window);

// The size of the image the game sees, in pixels: the swapchain extent as
// displayed. On a display the renderer draws rotated (Android pre-rotation,
// e.g. a portrait panel used in landscape) width and height are swapped
// relative to the swapchain. 0 x 0 while there is no swapchain.
R_API void r_display_size(uint32_t* width, uint32_t* height);

// Uploads a triangle-list mesh to GPU memory (blocking). The arrays are copied,
// so the caller may free them immediately. Returns 0 on failure.
R_API RMesh r_create_mesh(const RVertex* vertices, uint32_t vertex_count,
                          const uint32_t* indices, uint32_t index_count);

// Frees a mesh. The handle is invalid immediately; the GPU memory is released
// once the frames in flight that may use it have finished, so this never
// stalls and is fine for streaming. Meshes still alive at r_shutdown are freed there.
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

// Draws the debug UI over the frame. Call between r_begin_frame and
// r_end_frame, normally after the scene's r_draw. text holds all labels.
R_API void r_ui(const RUIInput* input, RUICmd* cmds, uint32_t count,
                const char* text, uint32_t text_length, RUIOutput* out);

R_API void r_end_frame(void);

// Asks for the next completed frame to be copied back to CPU memory.
R_API void r_capture_next_frame(void);

// Call right after the r_end_frame of a captured frame. Waits for the GPU and
// copies the image into rgba as tightly packed, sRGB-encoded 8-bit RGBA.
// Pass rgba = NULL to only query width/height. Returns 1 on success, 0 if no
// capture is pending or capacity is too small.
R_API int32_t r_read_capture(uint8_t* rgba, uint32_t capacity, uint32_t* width, uint32_t* height);

R_API void r_shutdown(void);

#ifdef __cplusplus
}
#endif

#endif // VKGAME_RENDERER_H
