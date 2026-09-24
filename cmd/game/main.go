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

	"vkgame/engine/platform"
	"vkgame/engine/render"
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
	screenshot := flag.String("screenshot", "", "render -frames frames, save the last one to this PNG and exit")
	frames := flag.Int("frames", 120, "number of frames to render before taking -screenshot")
	flag.Parse()

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	shaderDir := filepath.Join(filepath.Dir(exe), "shaders")

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

	g, err := game.New(*model)
	if err != nil {
		return err
	}
	var draws []render.DrawCmd

	var (
		rendered    int    // frames completed so far
		capturePath string // non-empty: capture the next frame to this file
		f12WasDown  bool
	)

	last := platform.Time()
	for !win.ShouldClose() {
		platform.PollEvents()
		if win.KeyDown(platform.KeyEscape) {
			win.SetShouldClose(true)
		}
		f12 := win.KeyDown(platform.KeyF12)
		if f12 && !f12WasDown && capturePath == "" {
			capturePath = time.Now().Format("screenshot-20060102-150405.png")
		}
		f12WasDown = f12
		if *screenshot != "" && rendered >= *frames-1 {
			capturePath = *screenshot
		}

		now := platform.Time()
		dt := float32(now - last)
		last = now
		g.Update(dt)

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
		render.EndFrame()
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
