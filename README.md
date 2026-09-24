# Cliff Crack

A game built on its own engine: a **Go host** driving a **native C++ Vulkan 1.3 renderer**.

The main menu offers two modes:

- **Run**, the arcade mode. You're dropped off a cliff and ride a ball down an endless,
  procedurally generated mountain. Steer around rocks and pines, jump the cracks, and go
  as far as you can. The first thing you hit ends the run.
- **Engine Demo**, the physics sandbox. Roll the checker ball around the arena, bump the
  spinning cubes and drop piles of balls.

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
build/bin/game.exe -mode run                  # skip the menu: menu | run | demo
build/bin/game.exe -mode run -seed 42         # replay one course (default: a new one each run)
build/bin/game.exe -mode run -autopilot -screenshot out.png -frames 600   # a self-driving run
build/bin/game.exe -model path/to/model.glb   # show a glTF model in the centre
build/bin/game.exe -screenshot out.png -frames 90   # 90 fixed 1/60 s frames, save the last, exit
build/bin/game.exe -drop 60                   # start with 60 physics balls
build/bin/game.exe -hold W -screenshot out.png -frames 110   # scripted input for tests
go test ./engine/... ./game/course   # engine and course-generation tests (no GPU needed)
```

The output goes into `build/bin/`: `renderer.dll`, `game.exe` and `shaders/*.spv`.
### Controls

| Where | Input | Action |
|---|---|---|
| Menu | **W / S**, arrows | choose (or click) |
| | **Enter** / **Space** | select |
| | **Esc** | quit |
| Run | **A / D**, left/right | steer |
| | **W** / **S** | tuck (cruise 15% faster) / brake |
| | **Space** | jump (hit a kicker's lip to clear a crack) |
| | mouse | look around (the cursor is captured while riding; with F1 open, hold the right button). The camera swings back behind the ball when you let go; steering always follows the direction of travel |
| | **R**, **Enter** | ride again after a wipeout |
| | **Esc** | pause |
| Engine Demo, orbit (default) | **W A S D** | roll the ball (relative to the camera) |
| | **Space** / **R** | jump / reset the ball |
| | left/right mouse drag | orbit the camera around the ball |
| | scroll | zoom |
| | *(no input for 3 s)* | slow auto-spin |
| Fly | **Tab** | toggle orbit / fly |
| | hold right mouse | look around |
| | **W A S D**, **Q / E** | move, down / up (**Shift** = faster) |
| | **Esc** | pause |
| Any | **F1** | show / hide the debug window (stats and tuning; Run's has the seed and an autopilot toggle) |
| | **F12** | save `screenshot-<time>.png` (read back from the GPU) |

#### Gamepad

Any controller GLFW recognises works on the desktop, and on Android any gamepad works,
including a handheld's built-in controller. The layout is Xbox-style: A is the bottom face
button. The on-screen hints switch to gamepad buttons as soon as you use one.

| Where | Input | Action |
|---|---|---|
| Menu | d-pad / left stick | choose |
| | **A** / **Start** | select |
| Run | left stick, d-pad | steer (analog on the stick) |
| | **RT** / **LT** | tuck / brake (analog) |
| | **A** | jump |
| | right stick | look around |
| | **Start** | pause |
| Wipeout card | d-pad, **A** | choose |
| | **Y** / **B** | ride again / main menu |
| Engine Demo | left stick | roll the ball |
| | right stick, **LB / RB** | orbit, zoom |
| | **A** / **X** / **Y** | jump / drop a ball / reset |
| | **B** / **Start** | pause |
| Any | **View** (Select) | show / hide the debug window |

On Android the back button works like Esc, and touching the screen works like the mouse,
so you can tap menu buttons. Dragging a finger in Run looks around.

## Android

`build-android.ps1` builds an arm64 APK. It needs the Android SDK with an NDK, build-tools
and a platform, plus a JDK (Android Studio installs all of these). It finds the SDK through
`ANDROID_HOME` or the usual install locations.

```powershell
./build-android.ps1 -Run        # build, install on the USB-connected device and start it
./build-android.ps1 -Run -Log   # ... and follow the game's log (logcat, tag CliffCrack)
```

There is no Gradle and no Java of our own. The app is a `NativeActivity`:

1. The renderer is built with CMake using the NDK's toolchain, giving `librenderer.so`.
2. The Go code is built with `-buildmode=c-shared`, giving `libcliffcrack.so`. It contains
   `engine/platform`'s NativeActivity glue.
3. `aapt2`, `zipalign` and `apksigner` package the two libraries and the SPIR-V shaders into
   `build-android/CliffCrack.apk`, signed with the debug key.

Specifics:

- **Pre-rotation**: a phone or handheld panel is usually portrait, even when the game is
  played in landscape. The renderer draws into a swapchain in the panel's native orientation
  and rotates everything itself, so the compositor doesn't rotate every frame. The scene
  gets the rotation folded into its view-projection matrix. The ImGui draw data is remapped
  after layout. The game sees the upright size through `render.DisplaySize`.
- **Window lifecycle**: Android destroys the window when the app goes to the background.
  `render.SetWindow` drops the Vulkan surface before the window is released, and makes a
  new one when the app returns.
- **Fullscreen**: the activity runs in sticky immersive mode and keeps the screen on. It is
  locked to landscape, either way up. The UI is scaled up for the screen's density.
- **Logs**: stdout and stderr go to logcat.
- **Quitting**: quitting from the menu ends the process, so the next launch starts clean.

Tested on an AYANEO (Konkr) Pocket FIT: Snapdragon 8 Gen 3, Adreno 750, Android 14. It
runs at 144 fps, the display's full refresh rate. Scripts and `-model` are desktop-only.

### Pause and settings

Esc, Start or the Android back button pauses Run and the Engine Demo. The game freezes
and the mouse is released. The pause menu offers **Resume**, **Restart run** (Run only),
**Settings** and **Main menu**, and pressing Esc, Start or B again resumes.

The settings screen is also on the main menu. It has:

- **Field of view** (35–90°): Run widens it a little at speed.
- **Look sensitivity**.
- **Invert look up/down**.
- **Volume**.

Changes show immediately, even behind the pause menu. You can drag the sliders with the
mouse, or pick a row with up/down and adjust it with left/right (holding repeats; the left
stick adjusts smoothly). Settings are saved when you leave the screen, to
`%AppData%\CliffCrack\settings.json` on Windows and the app's private storage on Android.

### Run mode

The course comes from `game/course` and is pure Go. It is a function of a seed: one
height function gives a meandering snow channel. Snow berms and ridged-noise peaks rise
beside it. Cracks cut across it, each with a kicker ramp before it and a clear landing
zone after.

It gets harder and faster all the way down:

- **Harder**: difficulty rises without limit (about 0.46 at 1 km, 0.71 at 2 km,
  0.85 at 3 km). The channel narrows, moguls grow, and cracks get wider and closer
  together. Rocks and pines get denser, and **gates** appear more and more often: walls
  of rocks across the channel with a single gap.
- **Faster**: the slope steepens from ~17° to ~29° over 3 km. On the ground the ride
  pushes the ball up to a **cruise speed** that climbs from 72 km/h to about 180 km/h,
  and drag only bites above it.
- **Zones**: every 500 m you enter a new named zone (*The Drop*, *Pine Line*,
  *Boulder Field*, and so on), announced with a banner.

The
level is built in 48 m chunks. Each chunk's mesh samples that same height function, and
the physics collides with it through a heightfield collider. What you see is what you
ride on. Chunks stream in as you go: the game keeps about 480 m ahead and builds at most
one new chunk per frame (~0.7 ms). The renderer frees old chunk meshes once no frame in
flight uses them, so streaming never stalls the GPU.

The look comes from per-draw shading flags on the engine side:

- **Faceted flat shading**: normals come from screen-space derivatives, so no duplicated
  vertices are needed.
- **Snow shading**: up-facing faces keep their colour and steep faces turn to dark rock.
- **A procedural sky**: a gradient with a sun disc on an inside-out dome.
- **Distance fog**: faded towards a sun-warmed haze, so the layered backdrop ranges dissolve
  into the horizon.

Lighting is a low golden sun with cool blue shade. `ride` (the rules and physics) has no
graphics, so `go test ./game` plays whole runs headless with an autopilot. The same
autopilot rides the menu backdrop.

### UI

The menu, HUD and debug windows are Dear ImGui with a custom theme: navy translucent
panels, rounded corners, the ball's orange as the accent, and Windows' Bahnschrift font
(ImGui's built-in font if it's missing). Besides the usual widgets, windows can be
pinned to a screen fraction, centred, transparent (text then gets a drop shadow) and
scaled (fonts are rasterised at that size). There is also coloured text, a progress bar,
same-line items, and a dial gauge (Run's speedometer). The gauge is drawn with ImGui's
draw lists: an arc that heats from white through orange to red, ticks, a needle and a
readout.

Dear ImGui runs inside the renderer, but Go describes the UI: `engine/ui.Builder`
turns `b.Slider("sun", &sun, 0, 3)`-style calls into a flat command list (labels packed
into one byte buffer), `render.UI` draws it in a single cgo call, and `Builder.Apply`
writes edits back to your variables. Buttons report their click on the next frame.
While the UI has the mouse, the camera ignores clicks and scrolling.

### Physics

`engine/physics` steps at a fixed 120 Hz with sequential impulses (restitution,
Coulomb friction, rolling). Bodies are drawn with `Body.Interpolated(world.Alpha())`,
between their last two steps. Drawing the raw state stutters on displays that aren't a
multiple of 120 Hz: some frames run no step, then the next one jumps. Dynamic bodies are spheres; they collide with each other
and with static or kinematic spheres and oriented boxes. In the demo the ground and
walls are static, the ring cubes and centre sphere are kinematic (they follow their
entities, so scripts can move them and they shove balls around), and "drop ball"
spawns dynamic balls. The player is a dynamic ball steered marble-style: input adds
spin and ground friction turns it into motion; cubes wobble and bonk when bumped.
`go test ./game` needs `renderer.dll` on `PATH` (the package links it), e.g. `build/bin`. Impacts come back as events and play positional sounds.
Tests check resting contact, bounce height, momentum, rolling on a ramp, kinematic
pushes and that a 60-ball pile never gains energy.

### Hot-reloadable scripts

Gameplay behaviours in [`scripts/`](scripts/) are plain Go files run by the
[yaegi](https://github.com/traefik/yaegi) interpreter. Any exported
`func Name(w *scene.World, e *scene.Entity, dt float32)` is a behaviour; the game
attaches them by name (`g.script("Bob")`). Save a file while the game runs and it
reloads within a quarter second — if the edit doesn't compile, the error is printed
and the previous version keeps running; a script that panics is disabled until the
next reload. Scripts can import `CliffCrack/engine/scene`, `mathx`, `gfx` and the standard
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
  physics/             rigid bodies: dynamic spheres vs spheres/oriented boxes/heightfields
  noise/               seeded gradient noise, FBM and ridged noise for procedural content
  platform/            GLFW window (desktop) or NativeActivity (Android); feeds OS events into input
  input/               per-frame keyboard/mouse/gamepad state (edges, deltas, deadzones)
  camera/              fly + orbit cameras, perspective lens
  mathx/               Vulkan-convention vectors, matrices, colours
  geom/                CPU mesh data + procedural shapes (cube, plane, sphere, grid, cone, icosphere)
  asset/               file loaders (glTF / GLB)
  script/              yaegi host: loads scripts/, hot-reloads, exposes behaviours
game/                gameplay code (pure Go, no cgo): app/menu, Run mode, Engine Demo
  course/              Run mode's seeded level generator (no GPU; testable on its own)
scripts/             hot-reloadable behaviours (interpreted at runtime)
cmd/game/            main package (an .exe on desktop, a c-shared library on Android)
android/             AndroidManifest.xml for the APK
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
- [x] Physics: pure-Go rigid bodies (dynamic spheres; static/kinematic spheres and boxes)
- [x] Heightfield terrain collider, seeded noise, streamed procedural terrain (Run mode)
- [x] Per-draw shading flags (flat, snow, unlit, sky), distance fog, deferred mesh freeing
- [x] Main menu, HUD and game-over card (anchored/overlay UI windows, scaled fonts)
- [x] Gamepad support (GLFW on desktop, Android input), touch as mouse
- [x] Android build: NativeActivity + c-shared Go, pre-rotated swapchain, window lifecycle

### Next ideas

- Shadows (a sun shadow map) and PBR materials (metallic/roughness from glTF)
- Dynamic boxes and a broadphase in physics; physics bodies as scene components
- Text input and more widgets in the debug UI; an entity inspector
- Frustum culling and instancing once scenes get large
- Run: saved best distances, a daily seed, snow spray and wind audio, more obstacle kinds
