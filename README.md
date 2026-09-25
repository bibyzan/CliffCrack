# Cliff Crack

A game built on its own engine: a **Go host** driving a **native C++ Vulkan 1.3 renderer**.

The main menu offers three modes:

- **Run**, the arcade mode. You're dropped off a cliff and ride a ball down an endless,
  procedurally generated mountain. Steer around rocks and pines, jump the cracks, and go
  as far as you can. The first thing you hit ends the run.
- **Arena**, a first-person duel: best of three single-life rounds against a bot. You're
  launched out of a bay into a Halo 5 Breakout-style arena of white panels and team-
  coloured light, with destructible cover in the spirit of THE FINALS and a
  sledgehammer, rifle and grenade launcher to tear it apart. It's the groundwork for an
  online multiplayer arena game.
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
build/bin/game.exe -mode run                  # skip the menu: menu | run | arena | demo
build/bin/game.exe -mode arena -autopilot -seed 11 -screenshot out.png -frames 700   # bot vs bot on site 11
build/bin/game.exe -mode run -seed 42         # replay one course (default: a new one each run)
build/bin/game.exe -mode run -seed 12 -from 800   # start 800 m down the course (try a particular section)
build/bin/game.exe -mode run -autopilot -screenshot out.png -frames 600   # a self-driving run
build/bin/game.exe -model path/to/model.glb   # show a glTF model in the centre
build/bin/game.exe -screenshot out.png -frames 90   # 90 fixed 1/60 s frames, save the last, exit
build/bin/game.exe -drop 60                   # start with 60 physics balls
build/bin/game.exe -hold W -screenshot out.png -frames 110   # scripted input for tests
build/bin/game.exe -mode arena -hold W -click -screenshot out.png -frames 250   # ... -click holds the left button
go test ./engine/... ./game/course ./game/arena   # engine, course and arena tests (no GPU needed)
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
| Arena | **W A S D** | move (**Shift** sprints) |
| | mouse | aim (the cursor is captured; with F1 open, hold the right button) |
| | left button | fire / swing (hold for automatic) |
| | **1 2 3**, scroll | hammer / rifle / launcher, or cycle |
| | **R** / **Space** | reload / jump |
| | **Esc** | pause (restart match, settings, main menu) |
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
| Arena | left stick / right stick | move / aim |
| | **RT** | fire / swing |
| | **A** / **X** / **L3** | jump / reload / sprint |
| | **RB** / **Y**, **LB** | next / previous weapon |
| | **Start** | pause |
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

After a warm-up, **special sections** break up the valley, a few hundred metres apart:

- **Ridge**: a warning tells you which side to climb. The path leaves the valley and rises
  onto that side's mountain, the same ridged peaks that wall the run. The crest pitches over
  those summits, the shoulders break into rock, and both sides fall away into the pit. Fall
  off and the run is over. Later the pit fills back in and the path drops into the valley.
- **Narrows**: the channel becomes a gorge through those mountains. It snakes, pinches and
  opens between ridged walls. A few rocks hug the walls, so you weave.

In plain valley, riding up the banks doesn't last. Past the channel's edge the snow slides
you back towards the path, harder the further up you are. Stay up there anyway and
snowballs come rolling down the bank at you, timed to cross your line. They knock you
about but don't end the run. Cracks only appear in plain valley.

The terrain is one height function in every section. A section blends in a *corridor*
around the path line: on a ridge, the summit of the side mountains, with the valley sunk
into a pit; in the narrows, a snaking gorge. Meshes are built around the path line, and
the physics, obstacles and autopilot all follow it too. Tests ride the autopilot through
a ridge and the narrows to prove they're rideable.

The level is built in 48 m chunks. Each chunk's mesh samples that same height function, and
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

### Arena mode

A first-person duel, and the groundwork for an online multiplayer arena game. The map
takes after Halo 5's Breakout, and the destruction after THE FINALS. You (blue) face a
bot (red) in a **best of three**: each round is **one life each**, and the last one
standing takes it. First to two rounds wins the match; Enter, A or a click starts a
rematch on a new site.

- **Rounds**: you start in a launch bay raised behind your end wall, looking out over
  the arena. During the 3 s countdown you can look around and pick a weapon, but not move
  or fire. At FIGHT the bay's pad fires you over the wall into the arena, with a whoosh,
  a jolt and a widening of the view. The round has a 2:30 clock: when time runs out the
  healthier player takes it, and level health is a draw. Every round starts on a fresh
  copy of the same site, with the players swapping ends.
- **The arena** is a compact 40 × 60 m box of white clean-sim panels outlined in glowing
  trim. Each end's trim glows in the colour of whoever starts there, so it follows you
  when you swap ends. The shell is the same every time:
  - a raised centre platform, with ramps down to the east and west
  - raised ledges along both side walls, with ramps at their ends
  - four tall pillars
  - a jump pad at each end that throws you onto the centre platform
  - a launch bay behind each end wall, with two pads (room for 2v2 later)
- **Cover** is generated from the seed: low walls, head-high walls (some with windows,
  some glazed), L-shaped cover, crates and the odd bunker. It's placed in the south half
  and mirrored through the centre into the north half, so both ends play the same. It
  never blocks the ramps, pads, pillars or landing zones.
- **Launch pads** throw whoever stands on them. In the air you can steer, but nothing
  slows you down, so a launch keeps its speed.
- **Players** have 150 health. Movement is a fixed-rotation physics sphere at the feet,
  with the eye 1.25 m above it. Velocity eases towards the input direction: quickly on
  the ground, slowly in the air. Gravity is 15 m/s² for snappy jumps. Bullets, blows and
  blasts use a separate **hitbox**: a capsule for the body and a sphere for the head,
  which takes 1.75× damage.
- **Sledgehammer** (1): a 0.7 s swing that lands 0.22 s in, with 2.8 m reach. Two blows
  down a player (80 each) and knock them back. Against a structure it spreads 120 damage
  over a 0.75 m radius, enough to hole brick or wood in one hit; concrete takes two.
- **Rifle** (2): hitscan at 600 rpm with a 30-round magazine and a 1.5 s reload. It does
  14 damage (24.5 to the head), chips wood and shatters glass. The spread widens while
  moving, jumping and firing bursts, and recoil kicks the view up and settles back.
- **Grenade launcher** (3): six rounds, 2.2 s reload. Grenades arc under gravity and go
  off on impact, on reaching a player, or after 2.5 s. The 4.2 m blast does up to 120 to
  players and destroys chunks. Your own grenades hurt you at half damage, so a rocket
  jump costs some health.
- **Structures** are built from axis-aligned box *chunks*. Walls are cut into panels of
  about 1 × 0.75 m, floors into 1.5 m tiles, and columns into storey-high posts, with door
  and window openings (glazed ones get glass panes). Each chunk has a material and HP
  scaled by its size: glass 4, wood 45, brick 90, concrete 160, metal 600.
- **Support**: chunks that share a face hold each other up. After any damage, a flood
  fill from the chunks on the ground finds everything still connected, and the rest
  collapses. A structure also comes down once it has lost 65% of its original footing.
  You can punch holes in a wall, but take out the ground floor and the whole tower falls.
- **Debris**: a broken chunk shatters into 1–4 physics pieces (glass into shards), and
  collapsing chunks drop whole. Rubble keeps colliding and clears after 3–12 s, with at
  most 450 pieces kept.
- **HUD and feedback**: the round score and clock, health, a name tag and health bar over
  the bot while it's in sight, a hit marker (red for headshots), a kill feed, the weapon
  bar and ammo, countdown and result banners. Taking damage flashes the view red and
  low health pulses it; tracers, dust in each material's colour, explosions, screen
  shake and panned sounds do the rest. The bot is a blocky soldier whose legs, head and
  weapon follow its movement and aim; it flashes when hit and topples when it dies.

#### The bot

`arena.Bot` plays through exactly the same `Input` a person does, one per step. It knows
the site's layout but not where you are:

- It **sees** you only within its field of view with nothing solid in between, and
  **hears** gunfire, launches and hammer blows within 45 m. Getting hurt also tells it
  where the shot came from.
- On a fresh sighting its aim starts off target and settles. It reacts after about a
  third of a second, and its aim always drifts a little. It tracks by following your
  motion across its view, as a player does.
- In the open it strafes and keeps to a comfortable range with the rifle, jumping now and
  then. Close in, or while its rifle reloads nearby, it charges with the hammer.
- When it has lost sight of you it heads for where it last saw or heard you, lobbing
  grenades at that spot on a ballistic arc. It hammers through walls in its way, hops
  low obstacles, and sidesteps whatever it can't break.
- Skill levels are easy, normal (the default) and hard, switchable in the F1 window. That
  window also has "bot holds fire", infinite ammo, autopilot and a new-match button.
  With `-autopilot` a second bot plays your side.

#### Built for multiplayer

The simulation is ready to run on a server:

- `package arena` has no graphics or input handling. `Arena.Step(dt, inputs)` takes one
  `Input` per player: movement, look deltas, fire, jump, reload and weapon choice. Those
  inputs are the only thing that drives a player.
- `Match` layers the rounds on top of `Arena`.
- Events report who did what (`Shot.By`, `Hurt`, `Kill`, `Explosion.By`, per-player
  `Action`s), so each client can pick its own effects and sounds.
- Players live in `Arena.Players` and are added with `AddPlayer`.
- The site is entirely determined by its seed. A client needs only the seed to build the
  level, then the chunk deaths and player states as they change.

The game layer already works this way: the local player's keyboard, mouse or pad input
and the bot's `Think` are just two sources of commands for the same step.

`game/arena`'s tests play all of it headless:
- movement, hitboxes and headshots, cover, each weapon against players and structures,
  rocket jumps and self-damage
- countdowns, round wins, side swaps, best of three, timeouts and draws; the launch bays
  firing only once the round is live and landing you in the arena, and the jump pads
  reaching the centre platform
- the site: the same seed gives the same arena, and its cover is mirrored exactly
- structures: support, collapse, the footing rule, every blueprint in every rotation
- the bot: it kills a standing target but not instantly, and it can't see through walls
  but hears gunfire. It breaks through a wall to reach a hidden player, the skill levels
  rank easy < normal < hard, and two bots finish a whole match.

The physics engine gained `World.Raycast`, `Body.FixedRotation`, `Body.Ignore` (a
grenade doesn't hit the player who fired it) and a uniform-grid broadphase for static
bodies (4 m cells, rebuilt only when statics change), which keeps a site of a thousand or
more chunks cheap.

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
game/                gameplay code (pure Go, no cgo): app/menu, Run mode, Arena, Engine Demo
  course/              Run mode's seeded level generator (no GPU; testable on its own)
  arena/               Arena rules: site generator, destructible structures, players,
                       hitboxes, weapons, rounds and the bot (no GPU)
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
- Dynamic boxes in physics (debris is sphere-collided for now); physics bodies as scene components
- Text input and more widgets in the debug UI; an entity inspector
- Frustum culling and instancing once scenes get large
- Arena online: a dedicated server stepping `arena.Match` from clients' `Input`s,
  snapshots of player state and chunk deaths, client-side prediction and interpolation
- Run: saved best distances, a daily seed, snow spray and wind audio, more obstacle kinds
