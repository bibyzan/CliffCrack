// Command game is the host executable: Go owns main, the loop and gameplay;
// renderer.dll does the Vulkan work.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

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

	g := game.New()
	clearColor := [4]float32{0.02, 0.02, 0.04, 1}
	var draws []render.DrawCmd

	last := platform.Time()
	for !win.ShouldClose() {
		platform.PollEvents()
		if win.KeyDown(platform.KeyEscape) {
			win.SetShouldClose(true)
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

		if !render.BeginFrame(clearColor) {
			continue
		}
		draws = g.Draw(float32(width)/float32(height), draws)
		render.Draw(draws)
		render.EndFrame()
	}
	return nil
}
