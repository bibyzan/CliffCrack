# vkgame

A game engine with a **Go host** driving a **native C++ Vulkan 1.3 renderer**.

```
┌──────────────────────── game.exe (Go) ────────────────────────┐
│ cmd/game      main loop                                        │
│ game/         gameplay: builds a draw list each frame          │
│ engine/       platform (GLFW window/input), math, render API   │
└───────────────────────────────┬────────────────────────────────┘
                                │ ~5 cgo calls per frame, POD only
┌───────────────────────────────▼────────────────────────────────┐
│ renderer.dll (C++)   renderer/include/renderer.h is the API    │
│ Vulkan 1.3: dynamic rendering, sync2, volk, vk-bootstrap, VMA  │
└────────────────────────────────────────────────────────────────┘
```

## The boundary rules

1. **Coarse calls.** A handful per frame (`begin`, `draw(list)`, `end`), never one per object.
2. **Plain data only.** Handles, flat arrays and fixed-size structs. No Go pointers are ever stored on the C side.
3. **Layouts match exactly.** Each C struct in `renderer.h` has a Go twin (e.g. `RDrawCmd` ↔ `render.DrawCmd`), and the Go twin is size-checked at startup.

## Prerequisites (Windows)

| Tool | Why | Get it |
|---|---|---|
| Go 1.25+ | host + gameplay | `winget install GoLang.Go` |
| GCC, CMake, Ninja | cgo needs GCC; builds the renderer | [MSYS2](https://www.msys2.org), then in the *UCRT64* shell: `pacman -S --needed mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-cmake mingw-w64-ucrt-x86_64-ninja`, and add `C:\msys64\ucrt64\bin` to `PATH` |
| Vulkan SDK | `glslc` shader compiler + validation layers | [vulkan.lunarg.com](https://vulkan.lunarg.com/sdk/home) |
| Git | CMake fetches the C++ dependencies | already installed |

You don't need to link against the Vulkan loader: volk loads `vulkan-1.dll` from your GPU driver at runtime.

## Build and run

```powershell
./build.ps1 -Run              # Debug build, validation layers on
./build.ps1 -Config Release
build/bin/game.exe -validation=false -vsync=false
build/bin/game.exe -model path/to/model.glb   # show a glTF model in the centre
build/bin/game.exe -screenshot out.png -frames 90   # render 90 frames, save the last, exit
go test ./engine/...          # math, geometry and glTF tests (no GPU needed)
```

The output goes into `build/bin/`: `renderer.dll`, `game.exe` and `shaders/*.spv`.
### Controls

| Mode | Input | Action |
|---|---|---|
| Orbit (default) | left/right mouse drag | rotate around the scene |
| | scroll | zoom |
| | *(no input for 3 s)* | slow auto-spin |
| Fly | **Tab** | toggle orbit / fly |
| | hold right mouse | look around |
| | **W A S D**, **Q / E** | move, down / up (**Shift** = faster) |
| Any | **F1** | show / hide the debug UI (stats, lighting, time scale, spawn cubes) |
| | **F12** | save `screenshot-<time>.png` (read back from the GPU) |
| | **Esc** | quit |

### Debug UI

Dear ImGui runs inside the renderer, but Go describes the UI: `engine/ui.Builder`
turns `b.Slider("sun", &sun, 0, 3)`-style calls into a flat command list (labels packed
into one byte buffer), `render.UI` draws it in a single cgo call, and `Builder.Apply`
writes edits back to your variables. Buttons report their click on the next frame.
While the UI has the mouse, the camera ignores clicks and scrolling.

### Hot-reloadable scripts

Gameplay behaviours in [`scripts/`](scripts/) are plain Go files run by the
[yaegi](https://github.com/traefik/yaegi) interpreter. Any exported
`func Name(w *scene.World, e *scene.Entity, dt float32)` is a behaviour; the game
attaches them by name (`g.script("Bob")`). Save a file while the game runs and it
reloads within a quarter second — if the edit doesn't compile, the error is printed
and the previous version keeps running; a script that panics is disabled until the
next reload. Scripts can import `vkgame/engine/scene`, `mathx`, `gfx` and the standard
library. `go vet ./...` type-checks them like normal code.

`-scripts <dir>` picks another directory, `-scripts none` disables scripting. After
changing the exported API of `scene`/`mathx`/`gfx`, run `go generate ./engine/script/...`
to refresh the interpreter's symbol tables.

Colours: the swapchain is sRGB and shaders work in linear space, so write colours
with `mathx.Hex(0xRRGGBB)` / `mathx.SRGB(...)` — they convert picker values to linear.

## Layout

```
renderer/            C++ Vulkan renderer (CMake project)
  include/renderer.h   the C API (the only thing Go sees)
  src/                 implementation
  shaders/             GLSL, compiled to SPIR-V at build time
engine/
  render/              cgo wrapper around renderer.h (the only cgo package)
  gfx/                 renderer data types (handles, DrawCmd, FrameParams), pure Go
  scene/               entity world: hierarchy, transforms, renderables, behaviours
  ui/                  debug UI builder (immediate-mode widgets -> command list)
  audio/               Go mixer (voices, pan, loops, WAV, synth blips) -> oto/WASAPI
  platform/            GLFW window; feeds OS events into input
  input/               per-frame keyboard/mouse state (pressed/released edges, deltas)
  camera/              fly + orbit cameras, perspective lens
  mathx/               Vulkan-convention vectors, matrices, colours
  geom/                CPU mesh data + procedural shapes (cube, plane, sphere)
  asset/               file loaders (glTF / GLB)
  script/              yaegi host: loads scripts/, hot-reloads, exposes behaviours
game/                gameplay code (pure Go, no cgo)
scripts/             hot-reloadable behaviours (interpreted at runtime)
cmd/game/            main package
```

## Roadmap

- [x] Meshes: `r_create_mesh` uploads vertex/index buffers through VMA; glTF parsed in Go
- [x] Depth buffer, perspective camera, directional lighting
- [x] Per-frame uniforms (camera, sun, ambient) — Blinn-Phong + hemisphere ambient
- [x] Textures: `r_create_texture` with mipmaps, bindless descriptor table
- [x] glTF materials (base colour factor + texture), parts grouped by material
- [x] Frame capture (`-screenshot`, F12)
- [x] Dear ImGui debug overlay driven by a Go-built command list (F1)
- [x] Input abstraction + orbit/fly camera controls
- [x] Entity model: scene.World with hierarchy, generational IDs, behaviours
- [x] Hot-reloadable gameplay scripts with [Yaegi](https://github.com/traefik/yaegi)
- [x] Audio: Go mixer on oto (WASAPI), positional pan/attenuation, `-audio=false` to mute
- [ ] Physics
