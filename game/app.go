// Package game is the gameplay layer. It only talks to the engine through
// plain Go types; it never touches cgo or Vulkan.
//
// The App switches between the main menu, the Run arcade mode, the Arena
// first-person duel on a destructible site, and the Engine Demo sandbox.
package game

import (
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
	ModeArena
	ModeRange // the Arena's firing range
)

// ParseMode maps "menu", "run", "arena", "range" or "demo" to a Mode.
func ParseMode(s string) (Mode, bool) {
	switch s {
	case "menu":
		return ModeMenu, true
	case "run":
		return ModeRun, true
	case "demo":
		return ModeDemo, true
	case "arena":
		return ModeArena, true
	case "range":
		return ModeRange, true
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
	Seed      uint64  // non-zero: every run uses this course
	StartAt   float32 // non-zero: runs start this many metres down the course (for testing sections)
	Autopilot bool    // the player's runs steer themselves
	Weapon    string  // Arena: start holding this weapon (by name; for screenshots)
	DebugUI   bool    // show the Engine Demo's debug window at startup (F1 toggles)
	Audio     *audio.Mixer
	Demo      DemoOptions
	DataDir   string // where settings are saved ("" = don't save)
}

// overlay is a screen shown over the frozen (or, on the main menu, idling) game.
type overlay int

const (
	overlayNone overlay = iota
	overlayPause
	overlaySettings
)

// App owns the modes and routes the frame to the active one.
type App struct {
	opts  Options
	mode  Mode
	menu  menu
	sc    *scenery
	run   *Run   // the menu backdrop and the Run mode share one Run
	demo  *Demo  // built the first time it is opened
	arena *Arena // likewise
	debug map[Mode]bool
	quit  bool

	pending Mode // a menu click, applied on the next Update
	picked  bool
	in      *input.State // this frame's input, for choosing button prompts

	settings       Settings
	overlay        overlay
	settingsReturn overlay // where Back leaves the settings screen for
	pause          pauseMenu
	settingsUI     settingsScreen
}

func NewApp(opts Options) (*App, error) {
	sc, err := newScenery()
	if err != nil {
		return nil, err
	}
	a := &App{
		opts:     opts,
		sc:       sc,
		debug:    map[Mode]bool{ModeDemo: opts.DebugUI},
		menu:     newMenu(),
		settings: DefaultSettings(),
	}
	if opts.DataDir != "" {
		s, err := LoadSettings(opts.DataDir)
		if err != nil {
			logf("settings: %v (using defaults)", err)
		}
		a.settings = s
	}
	a.run = newRun(sc, opts.Audio, opts.Seed, &a.settings)
	a.run.Autopilot = opts.Autopilot
	a.run.startAt = opts.StartAt
	a.opts.Demo.Settings = &a.settings
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
	case ModeArena, ModeRange:
		practice := mode == ModeRange
		if m := a.arena; m != nil {
			m.Practice = practice
			if err := m.start(); err != nil {
				return err
			}
			break
		}
		m, err := newArena(a.sc, a.opts.Audio, &a.settings, a.opts.Seed, practice)
		if err != nil {
			return err
		}
		m.Autopilot = a.opts.Autopilot
		m.startWeapon = a.opts.Weapon
		m.newRound() // again, now it knows what to hand you
		a.arena = m
	}
	a.mode = mode
	return nil
}

// Quit reports that the player chose to leave the game.
func (a *App) Quit() bool { return a.quit }

// Update advances the active mode. mouseFree is false while the UI has the mouse.
func (a *App) Update(dt float32, in *input.State, mouseFree bool) {
	a.in = in
	if a.opts.Audio != nil {
		a.opts.Audio.SetVolume(a.settings.Volume / 100)
	}
	if debugPressed(in) {
		a.debug[a.mode] = !a.debug[a.mode]
	}
	if a.picked {
		a.picked = false
		a.choose(a.pending)
		return
	}

	switch a.overlay {
	case overlaySettings:
		if a.settingsUI.update(in, &a.settings, dt) {
			a.saveSettings()
			a.overlay = a.settingsReturn
		}
		if a.mode == ModeMenu {
			a.run.Update(dt, in, false) // the backdrop keeps riding behind the settings
		}
		return
	case overlayPause:
		if act, ok := a.pause.update(in, a.pauseItems()); ok {
			a.pauseAction(act)
		}
		return // the game stays frozen
	}

	switch a.mode {
	case ModeMenu:
		if in.Pressed(input.KeyEscape) {
			a.quit = true
			return
		}
		a.run.Update(dt, in, mouseFree)
		if item, ok := a.menu.update(in, a.run); ok {
			a.choose(item)
		}
	case ModeRun:
		if a.run.wantsMenu {
			a.choose(ModeMenu)
			return
		}
		if pausePressed(in) {
			a.openPause()
			return
		}
		a.run.debugOpen = a.debug[ModeRun]
		a.run.Update(dt, in, mouseFree)
	case ModeDemo:
		if pausePressed(in) || in.PadPressed(input.PadB) {
			a.openPause()
			return
		}
		a.demo.Update(dt, in, mouseFree)
	case ModeArena, ModeRange:
		if pausePressed(in) {
			a.openPause()
			return
		}
		m := a.arena
		m.debugOpen = a.debug[a.mode]
		m.Update(dt, in, mouseFree)
	}
}

// Paused reports whether the game is frozen behind the pause menu or settings.
func (a *App) Paused() bool { return a.overlay != overlayNone && a.mode != ModeMenu }

func (a *App) openPause() {
	a.overlay = overlayPause
	a.pause.open()
}

func (a *App) openSettings(from overlay) {
	a.overlay, a.settingsReturn = overlaySettings, from
	a.settingsUI.open()
}

// pauseItems are the pause menu's entries for the current mode.
func (a *App) pauseItems() []pauseAction {
	if a.mode == ModeRun || a.mode == ModeArena || a.mode == ModeRange {
		return []pauseAction{pauseResume, pauseRestart, pauseSettings, pauseMainMenu}
	}
	return []pauseAction{pauseResume, pauseSettings, pauseMainMenu}
}

func (a *App) pauseAction(act pauseAction) {
	switch act {
	case pauseResume:
		a.overlay = overlayNone
	case pauseRestart:
		a.overlay = overlayNone
		if a.mode == ModeArena || a.mode == ModeRange {
			a.arena.restart() // a new match on a new site, or a fresh range
		} else {
			a.run.start(false)
		}
	case pauseSettings:
		a.openSettings(overlayPause)
	case pauseMainMenu:
		a.overlay = overlayNone
		a.choose(ModeMenu)
	}
}

// saveSettings writes the settings to the data directory, if there is one.
func (a *App) saveSettings() {
	if a.opts.DataDir == "" {
		return
	}
	if err := a.settings.Save(a.opts.DataDir); err != nil {
		logf("settings: %v", err)
	}
}

// choose acts on a menu item; quitting is the item after the modes.
func (a *App) choose(m Mode) {
	switch m {
	case menuQuit:
		a.quit = true
		return
	case menuSettings:
		a.openSettings(overlayNone)
		return
	}
	if err := a.enter(m); err != nil {
		logf("can't open %v: %v", m, err)
		a.enter(ModeMenu)
	}
}

// CursorLocked reports whether the mouse should be captured for looking
// around (while riding in Run; while dragging or flying in the demo).
func (a *App) CursorLocked() bool {
	if a.overlay != overlayNone {
		return false // the mouse is for the menus
	}
	switch a.mode {
	case ModeRun:
		return a.run.CursorLocked()
	case ModeDemo:
		return a.demo.CursorLocked()
	case ModeArena, ModeRange:
		return a.arena.CursorLocked()
	}
	return false
}

// Render returns the active mode's frame parameters and draw list.
func (a *App) Render(aspect float32, out []render.DrawCmd) (render.FrameParams, []render.DrawCmd) {
	switch a.mode {
	case ModeDemo:
		return a.demo.Render(aspect, out)
	case ModeArena, ModeRange:
		return a.arena.Render(aspect, out)
	}
	return a.run.Render(aspect, out)
}

// UI describes this frame's interface: menus, HUD and (F1) debug windows.
func (a *App) UI(b *ui.Builder, s Stats) {
	switch a.overlay {
	case overlaySettings:
		a.settingsUI.ui(b, &a.settings, a.in)
		return
	case overlayPause:
		a.pause.ui(b, a.pauseItems(), a.in)
		return
	}
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
	case ModeArena, ModeRange:
		m := a.arena
		m.UI(b, a.in)
		if a.debug[a.mode] {
			m.DebugUI(b, s)
		}
	}
}

// Main-menu items that aren't modes.
const (
	menuSettings Mode = ModeRange + 1 + iota
	menuQuit
)

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
	{"Arena", ModeArena},
	{"Firing Range", ModeRange},
	{"Engine Demo", ModeDemo},
	{"Settings", menuSettings},
	{"Quit", menuQuit},
}

func newMenu() menu {
	return menu{
		move: uiMoveSound(),
		pick: uiPickSound(),
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
