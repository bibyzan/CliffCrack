// Package game is the gameplay layer. It only talks to the engine through
// plain Go types; it never touches cgo or Vulkan.
//
// The App switches between the main menu, the Run arcade mode and the Engine
// Demo sandbox.
package game

import (
	"time"

	"CliffCrack/engine/audio"
	"CliffCrack/engine/input"
	"CliffCrack/engine/render"
	"CliffCrack/engine/ui"
)

// Mode is what the App is showing.
type Mode int

const (
	ModeMenu Mode = iota
	ModeRun
	ModeDemo
)

// ParseMode maps "menu", "run" or "demo" to a Mode.
func ParseMode(s string) (Mode, bool) {
	switch s {
	case "menu":
		return ModeMenu, true
	case "run":
		return ModeRun, true
	case "demo":
		return ModeDemo, true
	}
	return ModeMenu, false
}

// Stats are engine numbers shown in the debug UI.
type Stats struct {
	FPS     float32
	FrameMS float32
	Draws   int
}

type Options struct {
	Start     Mode
	Seed      uint64 // non-zero: every run uses this course
	Autopilot bool   // the player's runs steer themselves
	DebugUI   bool   // show the Engine Demo's debug window at startup (F1 toggles)
	Audio     *audio.Mixer
	Demo      DemoOptions
}

// App owns the modes and routes the frame to the active one.
type App struct {
	opts  Options
	mode  Mode
	menu  menu
	run   *Run  // the menu backdrop and the Run mode share one Run
	demo  *Demo // built the first time it is opened
	debug map[Mode]bool
	quit  bool

	pending Mode // a menu click, applied on the next Update
	picked  bool
	in      *input.State // this frame's input, for choosing button prompts
}

func NewApp(opts Options) (*App, error) {
	sc, err := newScenery()
	if err != nil {
		return nil, err
	}
	a := &App{
		opts:  opts,
		run:   newRun(sc, opts.Audio, opts.Seed),
		debug: map[Mode]bool{ModeDemo: opts.DebugUI},
		menu:  newMenu(),
	}
	a.run.Autopilot = opts.Autopilot
	if err := a.enter(opts.Start); err != nil {
		return nil, err
	}
	return a, nil
}

// enter switches to mode.
func (a *App) enter(mode Mode) error {
	switch mode {
	case ModeMenu:
		a.run.start(true)
	case ModeRun:
		a.run.start(false)
	case ModeDemo:
		if a.demo == nil {
			d, err := NewDemo(a.opts.Demo)
			if err != nil {
				return err
			}
			a.demo = d
		}
	}
	a.mode = mode
	return nil
}

// Quit reports that the player chose to leave the game.
func (a *App) Quit() bool { return a.quit }

// Update advances the active mode. mouseFree is false while the UI has the mouse.
func (a *App) Update(dt float32, in *input.State, mouseFree bool) {
	a.in = in
	if debugPressed(in) {
		a.debug[a.mode] = !a.debug[a.mode]
	}
	if a.picked {
		a.picked = false
		a.choose(a.pending)
		return
	}
	switch a.mode {
	case ModeMenu:
		if in.Pressed(input.KeyEscape) {
			a.quit = true
			return
		}
		a.run.Update(dt, in)
		if item, ok := a.menu.update(in, a.run); ok {
			a.choose(item)
		}
	case ModeRun:
		if pausePressed(in) || a.run.wantsMenu {
			a.choose(ModeMenu)
			return
		}
		a.run.Update(dt, in)
	case ModeDemo:
		if pausePressed(in) || in.PadPressed(input.PadB) {
			a.choose(ModeMenu)
			return
		}
		a.demo.Update(dt, in, mouseFree)
	}
}

// choose acts on a menu item; quitting is the item after the modes.
func (a *App) choose(m Mode) {
	if m == menuQuit {
		a.quit = true
		return
	}
	if err := a.enter(m); err != nil {
		logf("can't open %v: %v", m, err)
		a.enter(ModeMenu)
	}
}

// CursorLocked reports whether the mouse should be captured (demo mouse-look).
func (a *App) CursorLocked() bool {
	return a.mode == ModeDemo && a.demo.CursorLocked()
}

// Render returns the active mode's frame parameters and draw list.
func (a *App) Render(aspect float32, out []render.DrawCmd) (render.FrameParams, []render.DrawCmd) {
	if a.mode == ModeDemo {
		return a.demo.Render(aspect, out)
	}
	return a.run.Render(aspect, out)
}

// UI describes this frame's interface: menus, HUD and (F1) debug windows.
func (a *App) UI(b *ui.Builder, s Stats) {
	switch a.mode {
	case ModeMenu:
		if item, ok := a.menu.ui(b, a.run.best, a.in); ok {
			a.pending, a.picked = item, true
		}
	case ModeRun:
		a.run.UI(b, a.in)
		if a.debug[ModeRun] {
			a.run.DebugUI(b, s)
		}
	case ModeDemo:
		if a.debug[ModeDemo] {
			a.demo.DebugUI(b, s)
		}
	}
}

// menuQuit is the menu's third item.
const menuQuit Mode = ModeDemo + 1

// menu is the title screen over the self-playing run.
type menu struct {
	choice int
	move   *audio.Sound
	pick   *audio.Sound
}

var menuItems = []struct {
	label string
	mode  Mode
}{
	{"Run", ModeRun},
	{"Engine Demo", ModeDemo},
	{"Quit", menuQuit},
}

func newMenu() menu {
	return menu{
		move: audio.Blip(40*time.Millisecond, 700, 700, 0.25),
		pick: audio.Blip(120*time.Millisecond, 520, 1040, 0.5),
	}
}

// update handles keyboard navigation and reports a chosen item.
func (m *menu) update(in *input.State, r *Run) (Mode, bool) {
	step := navY(in)
	if in.Pressed(input.KeyTab) {
		step = 1
	}
	if step != 0 {
		m.choice = (m.choice + step + len(menuItems)) % len(menuItems)
		r.play(m.move, 1)
	}
	if confirmPressed(in) {
		r.play(m.pick, 1)
		return menuItems[m.choice].mode, true
	}
	return 0, false
}
