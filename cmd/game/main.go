// Command game is the host executable: Go owns main, the loop and gameplay;
// renderer.dll does the Vulkan work.
package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"vkgame/engine/audio"
	"vkgame/engine/gfx"
	"vkgame/engine/input"
	"vkgame/engine/platform"
	"vkgame/engine/render"
	uiPkg "vkgame/engine/ui"
	"vkgame/game"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	validation := flag.Bool("validation", true, "enable Vulkan validation layers (if installed)")
	vsync := flag.Bool("vsync", true, "wait for vertical sync")
	model := flag.String("model", "", "optional .gltf/.glb file to show in the centre of the scene")
	screenshot := flag.String("screenshot", "", "render -frames frames at a fixed 60 Hz step, save the last one to this PNG and exit")
	frames := flag.Int("frames", 120, "number of frames to render before taking -screenshot")
	ui := flag.Bool("ui", true, "show the debug UI at startup (F1 toggles)")
	sound := flag.Bool("audio", true, "enable audio output")
	drop := flag.Int("drop", 0, "number of physics balls to drop at startup")
	scripts := flag.String("scripts", "", `hot-reloadable scripts directory (default: ./scripts, else the repo's scripts/; "none" disables)`)
	flag.Parse()

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exeDir := filepath.Dir(exe)
	shaderDir := filepath.Join(exeDir, "shaders")

	win, err := platform.NewWindow("vkgame", 1280, 720)
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
	})
	if err != nil {
		return err
	}
	defer render.Shutdown() // runs before win.Destroy: surface goes before window

	win.OnFramebufferResize(render.Resize)

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

	g, err := game.New(game.Options{
		Model:      *model,
		ScriptsDir: scriptsDir(*scripts, exeDir),
		Audio:      mixer,
		DropBalls:  *drop,
	})
	if err != nil {
		return err
	}
	var draws []render.DrawCmd

	var (
		rendered    int    // frames completed so far
		capturePath string // non-empty: capture the next frame to this file
		showUI      = *ui
		uiBuilder   uiPkg.Builder
		uiWantMouse bool    // the debug UI used the mouse last frame
		fps         float32 // smoothed
	)

	in := win.Input()
	last := platform.Time()
	for !win.ShouldClose() {
		in.NewFrame()
		platform.PollEvents()
		if in.Pressed(input.KeyEscape) {
			win.SetShouldClose(true)
		}
		if in.Pressed(input.KeyF1) {
			showUI = !showUI
			uiWantMouse = false
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
		g.Update(dt, in, !uiWantMouse)
		win.SetCursorLocked(g.CursorLocked())

		width, height := win.FramebufferSize()
		if width == 0 || height == 0 { // minimized: sleep until something happens
			platform.WaitEvents()
			continue
		}

		var params render.FrameParams
		params, draws = g.Render(float32(width)/float32(height), draws)
		if capturePath != "" {
			render.CaptureNextFrame()
		}
		if !render.BeginFrame(params) {
			continue // the capture request carries over to the next frame
		}
		render.Draw(draws)
		if showUI {
			uiBuilder.Reset()
			g.DebugUI(&uiBuilder, game.Stats{FPS: fps, FrameMS: 1000 / max(fps, 1), Draws: len(draws)})
			out := render.UI(uiInput(win, dt), uiBuilder.Cmds, uiBuilder.Labels)
			uiWantMouse = out.WantMouse != 0
		}
		render.EndFrame()
		if showUI {
			uiBuilder.Apply()
		}
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
