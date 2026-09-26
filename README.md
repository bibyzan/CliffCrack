# Cliff Crack

A game built on its own engine: a **Go host** driving a **native C++ Vulkan 1.3 renderer**.

The main menu has Run, Arena (versus the bot, or Online), More (the Firing Range and
the Engine Demo), Settings and Quit:

- **Run**, the arcade mode. You're dropped off a cliff and ride a ball down an endless,
  procedurally generated mountain. Steer around rocks and pines, jump the cracks, and go
  as far as you can. The first thing you hit ends the run.
- **Arena**, a first-person duel: best of three single-life rounds against a bot. You're
  launched from a cliff across a chasm into a Halo 5 Breakout-style arena of white
  panels and team-coloured light, hung on girders over a bottomless drop. Nearly all of
  it breaks, in the spirit of THE FINALS, and you get a sledgehammer, rifle and grenade
  launcher to tear it apart. Play the bot, or **online**: find or make a room in the
  lobby browser and play up to four players, the site's destruction and all shared.
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
build/bin/game.exe -mode run                  # skip the menu: menu | run | arena | range | demo
build/bin/game.exe -mode arena -autopilot -seed 11 -screenshot out.png -frames 700   # bot vs bot on site 11
build/bin/game.exe -mode run -seed 42         # replay one course (default: a new one each run)
build/bin/game.exe -mode run -seed 12 -from 800   # start 800 m down the course (try a particular section)
build/bin/game.exe -mode run -autopilot -screenshot out.png -frames 600   # a self-driving run
build/bin/game.exe -model path/to/model.glb   # show a glTF model in the centre
build/bin/game.exe -screenshot out.png -frames 90   # 90 fixed 1/60 s frames, save the last, exit
build/bin/game.exe -drop 60                   # start with 60 physics balls
build/bin/game.exe -hold W -screenshot out.png -frames 110   # scripted input for tests
build/bin/game.exe -mode arena -hold W -click -screenshot out.png -frames 250   # ... -click holds the left button
build/bin/game.exe -mode range -weapon sniper -aim -screenshot out.png -frames 90    # the firing range, sniper scoped (-aim holds the right button)
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
| | mouse | look (the cursor is captured; with F1 open, hold the right button) |
| | left button | fire (hold for the rifle; click per shot for the rest) |
| | right button | aim down the sights (the sniper's scope; **Shift** there holds your breath) |
| | **1** / **2**, scroll | your two weapons: pick one, or swap |
| | **E** | pick up the weapon at your feet (swapping it for the one in hand) |
| | **F** | swing the hammer |
| | **G** / **Q** | throw a grenade / switch frags and stickies |
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
| Arena | left stick / right stick | move / look |
| | **RT** / **LT** | fire, aim down the sights |
| | **A** / **L3** | jump / sprint (hold breath when scoped) |
| | **X** | reload; hold to pick up |
| | **Y** | swap weapons |
| | **RB** / **LB** / **B** | hammer / throw a grenade / switch grenades |
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

## iOS

`build-ios.sh` builds the app for the iOS Simulator (default) or an iPhone/iPad. It needs
Xcode, Go, and CMake, Ninja and glslc (`brew install cmake ninja shaderc`).

```bash
./build-ios.sh --run                     # build, boot the Simulator, install, start, follow the log
./build-ios.sh --run --sim "iPad Air 11-inch (M3)"
./build-ios.sh --run -- -mode run        # the game's own flags go after --
./build-ios.sh --device --sign "Apple Development: …" --profile CliffCrack.mobileprovision --run
```

### On your iPhone

Apple only lets signed apps onto a phone. A free Apple ID is enough (the app then
lasts 7 days before it needs rebuilding); a paid developer account lasts a year. Once:

1. **Xcode → Settings → Accounts → +**: sign in with your Apple ID. This makes your
   "Personal Team".
2. On the phone, **Settings → Privacy & Security → Developer Mode**: turn it on (the phone
   restarts). Plug it into the Mac and tap **Trust**.
3. Make the provisioning profile: in Xcode, **File → New → Project → iOS → App**, name it
   anything, pick your team, and set the **bundle identifier** to your own, e.g.
   `com.yourname.cliffcrack` (bundle ids are unique across Apple, so `com.cliffcrack.game`
   may be taken). Choose your phone as the run destination and press **Run** once. Xcode
   makes a signing certificate and a profile for that id. You can delete the project.
4. With a free account, the first launch is blocked until you trust yourself on the
   phone: **Settings → General → VPN & Device Management → your Apple ID → Trust**.

Then, with the phone plugged in (or on the same Wi-Fi once it's been paired):

```bash
./build-ios.sh --device --run                                  # finds the profile made for *cliffcrack*
./build-ios.sh --device --bundle-id com.yourname.cliffcrack --run
```

The script finds the identity and profile itself (`ios/find-signing.py`), signs the app
and MoltenVK, installs with `devicectl` and starts the game with its log in the terminal.

There is no Xcode project, in the same spirit as the Android build:

1. The renderer is built with CMake for iOS (`CMAKE_SYSTEM_NAME=iOS`) as a static
   `librenderer.a`, with its dependencies.
2. The Go code is built with `-buildmode=c-archive`, giving `libcliffcrack.a`. It contains
   `engine/platform`'s UIKit glue (`ios.m`).
3. clang links `ios/main.m` and both libraries into `CliffCrack.app`, which also gets the
   SPIR-V shaders, `ios/Info.plist` and MoltenVK.

Specifics:

- **Vulkan is MoltenVK** (Vulkan on Metal), downloaded once into `build-ios/deps` and
  embedded as `Frameworks/MoltenVK.framework`. The renderer opens it itself and hands its
  entry point to volk. The surface is the game view's `CAMetalLayer`.
- **Threads**: UIKit keeps the main thread; the game loop runs on its own thread and reads
  touches and lifecycle changes from a queue.
- **Touch controls** in Run: a floating analog stick on the left half (it appears under
  your thumb; left/right steers, push up to tuck, pull down to brake), a JUMP button bottom
  right, drag anywhere else on the right to look around, and pause top left. The first
  finger also acts as the mouse, so menus work by tapping. The controls hide while a
  gamepad is in use.
- **Touch controls** in the Arena (and the Firing Range), after phone shooters: the stick
  moves (pushed all the way forward, it sprints) and dragging on the right half looks,
  FIRE included, so you can aim while you shoot. Round the bottom-right corner: FIRE,
  JUMP, AIM (a toggle), RELOAD, SWAP, NADE, FRAG/STICKY and HAMMER; PICK UP appears when
  there's a weapon at your feet. Touch aiming gets the pad's aim assist. The helmet's
  loadout moves to the bottom centre (weapons) and top left (grenades), clear of them.
- **Scenes**: the window comes from a `UIWindowSceneDelegate`; from iOS 26, UIKit stops an
  app built with the new SDK that doesn't use scenes.
- **Logs**: in the Simulator they're in the terminal. On a phone they go to
  `Documents/cliffcrack.log` in the app's container (the last run's):
  `xcrun devicectl device copy from --device <id> --domain-type appDataContainer
  --domain-identifier <bundle id> --source Documents/cliffcrack.log --destination .`
- **Background**: iOS doesn't let a background app use the GPU, so the game stops drawing
  as soon as it isn't frontmost.
- **The Simulator's GPU** is an older Metal family: it can't draw with a base vertex (the
  UI's vertices are flattened into one range on Apple) and MoltenVK doesn't claim dynamic
  texture-array indexing there (the renderer relies on Metal's instead).
- Gamepads and keyboards aren't read on
  iOS yet, and the Engine Demo has no touch controls.

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

- **Rounds**: you start in a launch bay jutting from the chasm wall beyond your end,
  looking out over the arena. During the 3 s countdown you can look around and pick a
  weapon, but not move or fire. At FIGHT the bay's pad fires you across the 12 m gap
  into the arena, with a whoosh, a jolt and a widening of the view. The round has a 2:30 clock: when time runs out the
  healthier player takes it, and level health is a draw. Every round starts on a fresh
  copy of the same site, with the players swapping ends.
- **The chasm**: the arena hangs between faceted rock walls like Run mode's
  mountainsides, with snow on their ledges and peaks above. The chasm runs off into the
  haze on both sides, and far below is water. Fall in (or get blown in) and you're out;
  whoever hurt you in the last 6 s gets the kill.
- **The arena** is a compact 40 × 60 m deck of white clean-sim panels outlined in glowing
  trim, in a valley of mountains, with a 29 m spire in the middle. Each end's trim glows
  in the colour of whoever starts there, so it follows you when you swap ends. The layout is the same every time:
  - a floor of 2 m plates laid on steel girders over the drop, with a low parapet round
    the edge
  - **the keep** in the middle: an 18 × 12 m deck at 2.5 m on columns, with grand
    stairs up to it from each end, and a 13 × 8 m top tier at 5 m reached by stairs
    from the bridges
  - **the spire** rising from the top tier to 29 m: a 4 m concrete core with a 4.5 × 5 m
    concrete balcony every 3 m (8 to 26 m), turning a quarter each time, and a walled
    cap on top. The balconies never reach into the corners between them, so the corners
    stay clear all the way up. Pads on the top tier's edges and on every balcony hop you
    up to the next. Or shoot out the core and bring the upper floors down.
  - **jump pads** either side of the grand stairs at each end fire you from the floor
    straight up a corner of the spire onto its 14 m balcony
  - bridges from the keep across to raised ledges along the sides, with stairs down
    from the ledges' ends
  - a concrete perch tower (5 m) either side of the middle at each end, with a lift pad,
    and a **sky bridge** from its top across to the mountain
  - **the mountains**: a ridge of the same snow-capped rock as the chasm walls runs the
    length of the chasm on each side and climbs into the walls at both ends, so the
    arena sits in one continuous range. A spur of it, rising out of the chasm, reaches
    in beside the arena. Step off the side ledge
    onto its saddle (2.5 m) and climb rock ramps to its lower terrace (5 m, where the
    sky bridge lands), its upper terrace (7.5 m) and its summit (10 m), where a pad
    throws you onto the pinnacle (12.5 m). The rock doesn't break.
  - a launch bay on each chasm wall, with two pads (room for 2v2 later)
- **Cover** is generated from the seed: low walls, head-high walls (some with windows,
  some glazed), L-shaped cover, crates, and the odd bunker or glasshouse. It's placed in
  the south half and mirrored through the centre into the north half, so both ends play
  the same. It never blocks the stairs, pads, towers or landing zones.
- **Launch pads** throw whoever stands on them (flying over one doesn't count). Each is
  aimed at a landing spot and works out the throw from wherever you stepped on it. In
  the air you can steer the throw, but not speed it up or slow it down, so every pad
  lands you where it's aimed (and a launch from the bay can't be braked into the pit).
  A pad whose floor is blown out is gone.
- **Stairs** are solid, breakable steps, but each tread collides as its stretch of a
  smooth ramp, so you walk up them at full speed. You also step up onto anything up to
  35 cm high.
- **Players** have **armour** (100) over their health (60). Armour takes hits first,
  and paint from a gun stops there: the shot that breaks it does no more. Once it
  breaks you're **popped** (a burst of paint and a pop): exposed, and one headshot from
  a precision gun (the pistol or the sniper) kills you. Blasts, hammer blows, rubble
  and falls carry on through armour into health. Armour recharges 4 s after you last
  took damage. There's no bar: the HUD reads ARMOUR, ARMOUR CRACKED (under half), or
  ARMOUR dimmed red with your health under it once it's gone, and your view pulses.
  Other players wear a faint glowing shell while their armour holds, and their suit
  picks up the shooter's paint as it goes. Name tags are just names. Movement is a
  fixed-rotation physics sphere at the feet,
  with the eye 1.25 m above it. Velocity eases towards the input direction: quickly on
  the ground, slowly in the air. Gravity is 15 m/s² for snappy jumps. The body is a
  sphere at the feet, so the space above your head is checked too: jump under a low
  deck and you stop short of it rather than putting your head (and the camera) through.
- **Momentum**: faster than you can run (off a pad, a blast or a hop), you keep your
  speed on the ground and steer. You only start sliding back to running pace a quarter
  second after landing, so jumping again straight away keeps it all: bunny hop off a
  pad and carry the speed across the map. A jump pressed just before you land is taken
  as you land. Pull back to brake. Shots, blows and blasts use a separate **hitbox**: a
  capsule for the body and a sphere for the head.
- **Two weapons, a hammer and grenades**, as in Halo: you carry two weapons (you
  start with the rifle and the pistol), swing the sledgehammer as your melee whatever
  you're holding, and throw grenades. Everything else is picked up: **E** (hold **X**)
  takes the weapon at your feet in place of the one in hand, which is dropped there.
  Walking over a weapon you already carry takes its ammo; walking over grenades takes
  them. Guns have a magazine and a reserve (shown as 24 | 144): run the reserve dry and
  you can't reload.
- **In the arena** everyone starts with the rifle and pistol. The sniper, the launcher,
  shotguns and crates of frags and stickies spawn at random spots each match (the same
  at both ends): up the spire, on the ledges, bridges, keep, towers and mountains, and
  on the floor where you land. A spawned weapon hovers and turns over a glowing ring (a
  column of light over the power weapons) and comes back 25 to 45 s after it's taken.
  Dropped weapons are cleared after 30 s.
- **Grenades** (two frags and a sticky to start, up to four of each): **frags** bounce,
  settle and go off 2.2 s after the throw (130 at the centre, 5 m); **stickies** stick
  to whatever they touch first, players included, glowing and chirping, and go off 1.6 s
  later (200, 4 m): stuck to someone, that's them done.
- **The guns are paintball markers**, after Halo: Combat Evolved's loadout: bright
  plastic bodies with a paint hopper fed in from the side (clear of the sights), a gas
  tank, rails, grips and guards, and proper sights: a holographic sight with a red ring
  reticle on the rifle, three-dot tritium irons on the pistol, a big ghost ring and a
  glowing fibre bead on the shotgun, a scope on the sniper and a ladder on the
  launcher. Shots are instant, but you see each paintball fly out at the gun's speed and
  burst into a splat of the shooter's colour (yours cyan, theirs orange) that stays for
  30 s, or until the piece it's on breaks.
- **Aim down the sights** (right button / LT): the gun comes up to your eye, close in
  so the sight frames the target, the view zooms and its edges darken. Spread tightens
  and bloom grows more slowly, but you move slower and can't sprint. The sniper goes to
  a full scope: a tinted lens with a cyan rim, duplex posts, mil-dots, range ticks and
  an illuminated centre. Scoped in, the aim sways; hold sprint to hold your breath and
  steady it for up to 3 s (run out and it shakes harder). A scoped sniper's lens glints
  for everyone else to see. Taking a hit while zoomed 2× or more knocks you out of your
  sights for half a second.
- **Hipfire bloom**: every shot widens the spread, which settles back over a moment;
  moving and jumping widen it further at the hip. The crosshair is drawn as wide as the
  cone your next shot can go anywhere in, so you can see it bloom.
- **Sledgehammer** (F): a 0.7 s swing that lands 0.22 s in, with 2.8 m reach, the gun
  put aside while it swings. Two blows down a player (80 each, through armour) and knock
  them back. Against a structure it
  spreads 120 damage over a 0.75 m radius, enough to hole brick or wood in one hit;
  concrete takes two.
- **Rifle**: 48 rounds (and 144 in reserve) at 800 rpm, 9 a ball. Wild from the hip and blooms fast;
  its climb has to be pulled down.
- **Pistol**: 12 rounds (48), one per click, 40 a ball. Three to the body pop armour,
  then one to the head kills. 2× zoom.
- **Shotgun** (a pickup): 8 pumps (16) of 12 pellets (14 each). Point-blank it all but kills
  through full armour; past 7 m it falls away fast. Its pellets shred walls.
- **Sniper** (a pickup): 4 rounds (12), 110 a ball. A headshot always kills; a body shot pops
  armour and a second one finishes. Wild from the hip, dead on through the 5× scope.
- **Grenade launcher** (a pickup): six rounds of paint grenades (12 more), 2.2 s reload. Grenades arc under gravity and go
  off on impact, on reaching a player, or after 2.5 s. The 4.2 m blast does up to 120 to
  players and destroys chunks. Your own grenades hurt you at half damage, so a rocket
  jump costs some health.
- **The firing range** (More → Firing Range on the main menu, or `-mode range`): a platform over
  the chasm with a firing line, a board 12 m out to read your grouping and bloom off
  the paint, distance posts every 10 m, and dummies from 10 to 85 m (two of them
  strafing) with armour like a player's. Downed dummies get back up after 1.5 s. Walls
  of wood, brick and concrete, a bunker and a glasshouse are there to shoot through.
  A **weapon table** by the line has every weapon and crates of both grenades, none of
  which run out, and reloads there don't use up your reserve.
  The HUD shows what's under the crosshair and how far, your accuracy and headshots.
- **Everything but the girders, the bays and the mountains breaks**: the floor, the
  keep, bridges, sky bridges, ledges, towers, stairs and parapets are structures like
  the cover (about 2,500 chunks in all). Blow out a sky bridge and whoever's on it drops.
- **Structures** are built from axis-aligned box *chunks*. Walls are cut into panels of
  about 1 × 0.75 m, floors into tiles, and columns into posts, with door and window
  openings (glazed ones get glass panes). Each chunk has a material and HP scaled by its
  size (up to 2.5×): glass 4, wood 45, brick 90, panel 110, floor plate 120, concrete
  160, metal 600.
- **Support**: chunks that share a face hold each other up, across structures, so a
  wall standing on the floor comes down with the floor. After any damage, load spreads
  out from the anchors (chunks resting on a girder): up into anything resting on a
  supported chunk, which then spans afresh, and sideways at the cost of the distance
  covered, but never down. Each material has a **span**, how far it can reach out from
  support: glass 2.5 m, brick 3.5 m, wood and panel 4.5 m, concrete and plate 7 m, metal
  10 m. Anything further out falls. Shoot out the keep's columns and the middle of its
  deck drops while the edges hang off the stairs and bridges. A building also comes down
  once it has lost 65% of its original footing.
- **Debris**: a broken chunk shatters into 2–7 physics pieces (a few big ones and a
  spray of chips; glass into shards), and collapsing chunks drop whole. Rubble falling
  faster than 7 m/s **crushes**: it hurts players it lands on (credited to whoever broke
  it loose) and damages the structures it hits, so a collapse can bring down what's
  below. Rubble clears after 3–12 s or once it falls into the chasm, with at most 700
  pieces kept. A big collapse rumbles and shakes the view.
- **The look**: nothing in the arena is a plain box. The guns, the characters and the
  first-person arms are built from chamfered boxes (every edge cut back, octagonal in
  section) and faceted gems, flat-shaded per facet like the mountains; each model is
  baked into one mesh per colour at first use, so the detail costs a few draws. The
  characters have round helmets with wraparound visors, shoulder domes and knee pads;
  the arms are tapered prisms with elbow pads and gloved fists. Structure chunks and
  rubble have bevelled edges.
- **HUD width** (in Settings): how much of the screen's width the HUD spans, 30 to 100%,
  centred. On an ultrawide, bring it in (about 50% on 32:9 gives a 16:9 box) so the
  armour bar, weapons, ammo, grenades, score and kill feed sit where you're looking.
  Run mode's speedometer and best distance follow it too.
- **Aim assist (gamepad only)**: light, and never with the mouse. With the crosshair on
  or just beside an enemy you can see (within 60 m), the right stick turns up to 45%
  slower; while you're moving the stick or yourself, the aim drifts gently onto their
  chest (faster with the sights up). It never snaps, and does nothing while you're still.
- **The helmet HUD** is drawn in the world a hand's width from your eye, framed by
  faint visor brackets: your **armour bar** across the top (segments that drain, flash
  when hit, sweep back as it recharges, amber when low and red once it's gone, with
  health pips under it); **hit markers** round the crosshair (white, pale blue on
  armour, red for the head, bigger for a kill); **red chevrons** pointing at whoever
  just shot you; and **SVG icons** (`game/icons`, rasterized by `engine/svgicon`) for
  your two weapons, the rounds left in the magazine (spent ones dim) and your frags
  and stickies. Text is just the score, magazine and reserve, the kill feed, name tags
  and banners.
- **First-person arms** hold every weapon (two-bone arms reaching from out of view):
  the firing hand on the grip, the other on the fore grip or pump. **Reloads are
  animated**: the rifle, pistol and sniper drop their magazine, the hand fetches a
  fresh one, seats it and works the charging handle, slide or bolt; the shotgun rolls
  to show its port and takes shells one at a time (fire to break off); the launcher
  swings its drum out and swaps it. Each step has its sound, and the shotgun's pump
  racks after every shot. Throwing a grenade takes the off hand off the gun.
- Tracers of paint, dust in each material's colour, explosions, screen
  shake and panned sounds do the rest. The sounds are synthesised at startup
  (`audio.Synth`: layers of swept tones and filtered noise, with an echo for the chasm):
  gas pops for the markers, a wet splut where paint lands, a tink off armour and a
  glassy shatter when it pops, a crack, crunch, tinkle or clang for each material, and
  booms and collapses that echo off the walls. The bot is a blocky soldier whose legs, head and
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
- In the open it strafes and keeps to a comfortable range, jumping now and then. It
  swings the hammer point-blank, and otherwise picks whichever of its two weapons suits
  the range: the shotgun close, the rifle in the middle, the pistol at range and the
  sniper far off (skipping one that's empty and reloading). With the pistol or sniper it
  aims down the sights, and goes for the head once your armour's gone (always, with the
  sniper).
- It goes for weapons better than its worst one, and grenades when it's short: while
  roaming, or on the way past when you're not close, swapping out the lesser gun.
- When it has lost sight of you it heads for where it last saw or heard you, lobbing a
  frag (or a launcher round) at that spot on a ballistic arc. It hammers through walls in its way, hops
  low obstacles, and sidesteps whatever it can't break. It never walks or jumps off an
  edge or into a hole blown in the floor: it turns along it instead.
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
- movement, momentum (hopping keeps your speed, landing slides then stops, pulling back
  brakes), headroom (a jump under a deck keeps your eye below it), hitboxes and headshots, cover, each weapon against players and structures,
  rocket jumps and self-damage
- countdowns, round wins, side swaps, best of three, timeouts and draws; the launch bays
  firing only once the round is live and landing you in the arena (even holding back),
  the jump pad reaching the keep's top, the lift pads the towers and the hop pads the
  spire (every pad a level up, and not onto another pad), the floor's jump pads the
  spire's third balcony, and the pinnacle, both
  flights of stairs climbable, and the mountain climbable
  from the side ledge to the summit, and from a tower over its sky bridge
- the site: the same seed gives the same arena, its cover is mirrored exactly, and,
  linked together, everything stands before any damage
- structures: support, spans, collapse, the footing rule, every blueprint in every
  rotation; blowing out the floor drops what stands on it, the keep comes down without
  its columns, a pad dies with its floor, falling rubble hurts, and a player knocked into
  the pit is credited to whoever hit them
- the bot: it kills a standing target but not instantly, and it can't see through walls
  but hears gunfire. It breaks through a wall to reach a hidden player, the skill levels
  rank easy < normal < hard, and two bots finish a whole match.

The physics engine gained `World.Raycast`, `Body.FixedRotation`, `Body.Ignore` (a
grenade doesn't hit the player who fired it) and a uniform-grid broadphase for static
bodies (4 m cells, rebuilt only when statics change), which keeps a site of thousands of
chunks cheap. Short rays (up to 12 m) only test the statics in the grid cells they cross,
unrotated boxes skip the rotation maths, and dynamic bodies find each other by sort and
sweep, so hundreds of pieces of rubble can fall at once.

### Online multiplayer

Up to four players, one of them hosting. Pick **Arena → Online** on the main menu: the lobby
browser lists the rooms on the server; **Create a room**, or pick one to join. In a
room you see who's in it and whether each player's connected; the host starts the match
once everyone is. A room drops out of the list once it's playing.

```
            coordinator (Go; fly.io)                  GET /rooms, /healthz, /ws
             lobby: rooms, join codes
             WebRTC signalling only
            /                      \
   host (runs the match)  <== WebRTC data channels ==>  guests (up to 3)
     steps arena.Match with            fast: inputs up / snapshots down
     everyone's inputs                 reliable: every event down
```

- **The coordinator** (`coordinator`, `cmd/coordinator`) is a small HTTP and WebSocket
  server: it lists rooms, lets players create and join them (by a four-letter code),
  and relays WebRTC offers, answers and ICE candidates between a room's host and its
  guests. No game traffic passes through it. It's ready for fly.io (`Dockerfile`,
  `fly.toml`, a `/healthz` check); for now run it on your network:

  ```powershell
  go run ./cmd/coordinator             # listens on :8080 and prints this machine's LAN address
  build/bin/game.exe -server 192.168.1.20:8080                   # everyone else points at it
  build/bin/game.exe -mode online -server 192.168.1.20:8080 -host   # make a room, start when someone joins
  build/bin/game.exe -mode online -server 192.168.1.20:8080 -join   # join the first open room
  ```

  A phone has no command line, so bake the server into the Android build:
  `./build-android.ps1 -Run -Server 192.168.1.20:8080` (the APK now asks for network
  access, and links with `-checklinkname=0` for pion's Android interface lookup).
  The server and your name can also go in `settings.json` (`"server"`, `"name"`); by
  default the server is the public one on fly.io and your name is your account's. An address
  on a private network or `localhost` uses plain `ws://`; a public name uses `wss://`.
- **The public coordinator** runs on fly.io at `cliffcrack-coordinator.fly.dev`, the
  games' default server (one machine: rooms live in its memory; deploy with
  `fly deploy --ha=false`). When a player connects it hands them the ICE servers to use:
  STUN (Cloudflare's and Google's, free) so players on ordinary home networks connect
  directly, and TURN relays for the rest (phone networks, strict routers) if configured
  with fly secrets: `CF_TURN_KEY_ID` and `CF_TURN_API_TOKEN` for Cloudflare's TURN, or
  `TURN_URLS` with `TURN_SECRET` (coturn) or `TURN_USERNAME`/`TURN_CREDENTIAL`. TURN
  credentials are fetched per player and short-lived; none are built into the game.
- **Links** (`online`) are WebRTC data channels (pion, pure Go) from the host to each
  guest: an unordered, unreliable channel for the stream of inputs and snapshots, and a
  reliable, ordered one for events and control. On a LAN the players' own addresses are
  enough; across the internet we'll add a STUN server, and TURN for strict NATs.
- **The host runs the match.** Each guest sends its input every frame (held buttons as
  they are, presses as running counts, so a lost packet never loses a jump or a reload;
  aim as an absolute direction, applied at once on the guest so looking never lags).
  The host steps `arena.Match` with everyone's input and sends back, every step, the
  events (reliably) and, every other step, a snapshot of everything else.
- **Everything is shared.** Snapshots carry every player's movement, aim, recoil, scope
  sway, armour and health, weapons, magazines, reserves, reloads, hammer swings and
  grenades, plus every grenade in flight or stuck, and every pickup. Events carry every
  shot, hit, kill, hammer blow, explosion, sticky sticking and action, and **every chunk
  of the site that is chipped, breaks or collapses**, by ID. Each guest builds the same
  site from the match's seed, so chunk IDs agree, and applies the host's breaks and
  collapses itself (throwing its own rubble, which is cosmetic): the destruction is the
  same on every screen. Round changes, the score and rematches come from the host too.
- If a guest leaves, their player stands still; if the host leaves, the match ends for
  everyone with a message. Menus don't pause an online match: it plays on behind them.
- Tests: the coordinator's rooms and signalling; a real WebRTC link on this machine;
  two sessions finding each other through a coordinator and linking; presses surviving
  lost packets; and whole matches mirrored from a host to a client, through JSON and
  over a link that drops a fifth of the fast packets, ending with every chunk of the
  site (hundreds broken and collapsed) and every player's state identical.
- **Hiding the lag.** Online matches tick at a fixed 60 Hz on every machine (looking
  still updates every frame). Each packet from a guest carries its last three inputs,
  and the host queues them and uses one per tick, in order: the host moves the guest
  exactly as the guest did. The guest **predicts** its own movement at once; each
  snapshot says which of its inputs the host has used, so the guest resets to the host's
  position and replays the rest (`online.Predictor`), easing out whatever's left
  instead of snapping. Its sights go up and down at once too. Everyone else is drawn
  **interpolated** 100 ms behind the host between snapshots (`online.Interp`), so they
  glide rather than jump. Tested over a simulated hotspot (60-150 ms, 10% loss): the
  guest's prediction stays within about 9 cm of the host on average, where without it
  the guest would see itself over a metre behind.
- **Lean messages.** Reliable frames only go out when something happened or the
  standing changed; the clock rides in snapshots. Pickups are left out of a snapshot
  when they haven't changed. Messages over 400 bytes are deflated: a full snapshot is
  under 900 bytes, one packet.
- **Lag compensation.** Each guest input carries the moment of the host's match it
  was seeing everyone else at (its interpolated view). The host keeps 0.6 s of every
  player's positions, and while that guest's weapons act it puts the other players back
  where the guest saw them (up to 350 ms back), then returns them: aim where they are
  on your screen and it hits. Only players move back; the site is as it is now.
- Not yet: a binary wire format, and host migration.

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
  platform/            GLFW window (desktop), NativeActivity (Android) or UIKit (iOS); feeds OS events into input
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
cmd/game/            main package (an .exe on desktop, a c-shared library on Android, a c-archive on iOS)
android/             AndroidManifest.xml for the APK
ios/                 the app's main.m and Info.plist
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
- [x] iOS build: MoltenVK, UIKit + c-archive Go, multi-touch, on-screen controls for Run and Arena

### Next ideas

- Shadows (a sun shadow map) and PBR materials (metallic/roughness from glTF)
- Dynamic boxes in physics (debris is sphere-collided for now); physics bodies as scene components
- Text input and more widgets in the debug UI; an entity inspector
- Frustum culling and instancing once scenes get large
- Online: client-side prediction and reconciliation, a binary wire format, STUN/TURN
  and deploying the coordinator to fly.io, team modes for four players
- Run: saved best distances, a daily seed, snow spray and wind audio, more obstacle kinds
