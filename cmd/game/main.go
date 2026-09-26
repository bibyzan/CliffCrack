// Command game is the host executable: Go owns main, the loop and gameplay;
// renderer.dll (librenderer.so on Android) does the Vulkan work.
//
// On Android this package is built as a shared library loaded by
// NativeActivity; main_android.go starts run from there instead of main. On
// iOS it's a static library linked into the app, and main_ios.go does the same.
package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"CliffCrack/engine/audio"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/input"
	"CliffCrack/engine/platform"
	"CliffCrack/engine/render"
	uiPkg "CliffCrack/engine/ui"
	"CliffCrack/game"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	validation := flag.Bool("validation", validationDefault, "enable Vulkan validation layers (if installed)")
	vsync := flag.Bool("vsync", true, "wait for vertical sync")
	model := flag.String("model", "", "optional .gltf/.glb file to show in the centre of the scene")
	screenshot := flag.String("screenshot", "", "render -frames frames at a fixed 60 Hz step, save the last one to this PNG and exit")
	frames := flag.Int("frames", 120, "number of frames to render before taking -screenshot")
	mode := flag.String("mode", "menu", `start in "menu", "run", "arena", "range" (the firing range), "online" (the lobby browser) or "demo"`)
	seed := flag.Uint64("seed", 0, "Run mode course seed (0 = a new course every run)")
	autopilot := flag.Bool("autopilot", false, "Run mode steers itself (for demos and scripted tests)")
	from := flag.Float64("from", 0, "Run mode: start this many metres down the course (with -seed, to try a particular section)")
	ui := flag.Bool("ui", true, "show the Engine Demo's debug UI at startup (F1 toggles the debug window in any mode)")
	sound := flag.Bool("audio", true, "enable audio output")
	drop := flag.Int("drop", 0, "number of physics balls to drop at startup")
	hold := flag.String("hold", "", `keys to hold down every frame, e.g. "W" or "WD" (for scripted tests)`)
	click := flag.Bool("click", false, "hold the left mouse button every frame (for scripted tests)")
	weapon := flag.String("weapon", "", `Arena: start holding this weapon ("rifle", "pistol", "shotgun", "sniper" or "launcher"; for screenshots)`)
	server := flag.String("server", "", `online: the coordinator, e.g. "192.168.1.20:8080" (default: the setting, else localhost:8080)`)
	name := flag.String("name", "", "online: the name to go by (default: the setting, else your account name)")
	hostRoom := flag.Bool("host", false, "online: make a room, and start the match as soon as someone's connected")
	joinRoom := flag.Bool("join", false, "online: join the first open room")
	tap := flag.String("tap", "", `keys to press on given frames, e.g. "R@60,G@90" (for scripted tests)`)
	aim := flag.Bool("aim", false, "hold the right mouse button every frame: aim down the sights (for scripted tests)")
	touch := flag.Bool("touch", platform.TouchScreen(), "show the touch screen's controls and settings (-touch=false: a keyboard's, e.g. to test them on a phone)")
	scripts := flag.String("scripts", "", `hot-reloadable scripts directory (default: ./scripts, else the repo's scripts/; "none" disables)`)
	flag.Parse()
	startMode, ok := game.ParseMode(*mode)
	if !ok {
		return fmt.Errorf("unknown -mode %q (want menu, run, arena, range, online or demo)", *mode)
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exeDir := filepath.Dir(exe)
	shaderDir, err := platform.ShaderDir()
	if err != nil {
		return err
	}

	win, err := platform.NewWindow("Cliff Crack", 1280, 720)
	if err != nil {
		return err
	}
	defer win.Destroy()

	fbWidth, fbHeight := win.FramebufferSize()
	err = render.Init(render.Config{
		Window:     win.NativeHandle(),
		Width:      fbWidth,
		Height:     fbHeight,
		ShaderDir:  shaderDir,
		Validation: *validation,
		VSync:      *vsync,
		UIScale:    platform.UIScale(),
	})
	if err != nil {
		return err
	}
	defer render.Shutdown() // runs before win.Destroy: surface goes before window

	win.OnFramebufferResize(render.Resize)
	win.OnNativeWindow(render.SetWindow)

	var mixer *audio.Mixer
	if *sound {
		mixer = audio.NewMixer()
		dev, err := audio.Open(mixer)
		if err != nil {
			fmt.Fprintln(os.Stderr, "audio disabled:", err)
			mixer = nil
		} else {
			defer dev.Close()
		}
	}

	dataDir, err := platform.DataDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "settings won't be saved:", err)
		dataDir = ""
	}
	app, err := game.NewApp(game.Options{
		Start:     startMode,
		Seed:      *seed,
		Autopilot: *autopilot,
		Weapon:    *weapon,
		Server:    *server,
		AutoHost:  *hostRoom,
		AutoJoin:  *joinRoom,
		Name:      *name,
		StartAt:   float32(*from),
		DebugUI:   *ui,
		Audio:     mixer,
		DataDir:   dataDir,
		Touch:     *touch,
		Demo: game.DemoOptions{
			Model:      *model,
			ScriptsDir: scriptsDir(*scripts, exeDir),
			Audio:      mixer,
			DropBalls:  *drop,
		},
	})
	if err != nil {
		return err
	}
	var draws []render.DrawCmd

	var (
		rendered    int    // frames completed so far
		capturePath string // non-empty: capture the next frame to this file
		uiBuilder   uiPkg.Builder
		uiWantMouse bool    // the debug UI used the mouse last frame
		fps         float32 // smoothed
	)

	in := win.Input()
	last := platform.Time()
	for !win.ShouldClose() {
		in.NewFrame()
		platform.PollEvents()
		for _, k := range strings.ToUpper(*hold) {
			in.KeyEvent(input.Key(k), true) // letters and space use their ASCII codes
		}
		if *click {
			in.ButtonEvent(input.MouseLeft, true)
		}
		if *aim {
			in.ButtonEvent(input.MouseRight, true)
		}
		taps := map[rune]bool{} // each key down on any of its frames, up otherwise
		for _, t := range strings.Split(*tap, ",") {
			var k rune
			var at int
			if n, _ := fmt.Sscanf(strings.ToUpper(t), "%c@%d", &k, &at); n == 2 {
				taps[k] = taps[k] || rendered == at
			}
		}
		for k, down := range taps {
			in.KeyEvent(input.Key(k), down)
		}
		if in.Pressed(input.KeyF12) && capturePath == "" {
			capturePath = time.Now().Format("screenshot-20060102-150405.png")
		}
		if *screenshot != "" && rendered >= *frames-1 {
			capturePath = *screenshot
		}

		now := platform.Time()
		dt := float32(now - last)
		last = now
		if *screenshot != "" {
			dt = 1.0 / 60 // deterministic: -frames N always simulates N/60 seconds
		}
		if dt > 0 {
			fps += (1/dt - fps) * 0.05
		}
		app.Update(dt, in, !uiWantMouse)
		if app.Quit() {
			break
		}
		win.SetCursorLocked(app.CursorLocked())

		width, height := render.DisplaySize() // what the player sees (can be rotated from the window)
		if fw, fh := win.FramebufferSize(); fw == 0 || fh == 0 || width == 0 || height == 0 {
			platform.WaitEvents() // minimized or in the background: sleep until something happens
			continue
		}

		var params render.FrameParams
		params, draws = app.Render(float32(width)/float32(height), draws)
		if capturePath != "" {
			render.CaptureNextFrame()
		}
		if !render.BeginFrame(params) {
			continue // the capture request carries over to the next frame
		}
		render.Draw(draws)
		uiBuilder.Reset()
		app.UI(&uiBuilder, game.Stats{FPS: fps, FrameMS: 1000 / max(fps, 1), Draws: len(draws)})
		out := render.UI(uiInput(win, dt), uiBuilder.Cmds, uiBuilder.Labels)
		uiWantMouse = out.WantMouse != 0
		render.EndFrame()
		uiBuilder.Apply()
		rendered++

		if capturePath != "" {
			if err := saveCapture(capturePath); err != nil {
				return err
			}
			capturePath = ""
			if *screenshot != "" {
				return nil
			}
		}
	}
	return nil
}

// uiInput converts this frame's mouse state for the debug UI. While the cursor
// is captured for camera look, the UI gets no mouse at all.
func uiInput(win *platform.Window, dt float32) gfx.UIInput {
	in := win.Input()
	u := gfx.UIInput{MouseX: -1, MouseY: -1, Wheel: float32(in.Scroll()), DeltaTime: dt}
	if win.CursorLocked() {
		u.Wheel = 0
		return u
	}
	x, y := win.ToFramebuffer(in.MousePos())
	u.MouseX, u.MouseY = float32(x), float32(y)
	for i, b := range []input.MouseButton{input.MouseLeft, input.MouseRight, input.MouseMiddle} {
		if in.MouseDown(b) {
			u.MouseButtons |= 1 << i
		}
	}
	return u
}

// scriptsDir resolves the -scripts flag. By default it prefers ./scripts and
// then the source tree's scripts/ (the exe lives in <repo>/build/bin), so
// edits to the checked-in scripts hot-reload straight into a dev build.
func scriptsDir(flagValue, exeDir string) string {
	switch flagValue {
	case "none":
		return ""
	case "":
		for _, dir := range []string{"scripts", filepath.Join(exeDir, "..", "..", "scripts")} {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return filepath.Clean(dir)
			}
		}
		return ""
	}
	return flagValue
}

func saveCapture(path string) error {
	img, err := render.ReadCapture()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Println("saved", path)
	return nil
}
