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
	"CliffCrack/online"
)

// Mode is what the App is showing.
type Mode int

const (
	ModeMenu Mode = iota
	ModeRun
	ModeDemo
	ModeArena
	ModeRange  // the Arena's firing range
	ModeOnline // the lobby browser; an online match itself plays in ModeArena
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
	case "online":
		return ModeOnline, true
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
	Server    string // online: the coordinator (overrides the setting)
	Name      string // online: the name to go by (overrides the setting)
	AutoHost  bool   // online: make a room and start as soon as someone's in (-host)
	AutoJoin  bool   // online: join the first open room (-join)
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
	lobby onlineScreen
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
	if a.arena != nil && a.arena.Online() {
		a.arena.leaveOnline() // leaving an online match, for whatever's next
	}
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
	case ModeOnline:
		if a.run.ride == nil {
			a.run.start(true) // the menu's ride, behind the lobby (when started straight into it)
		}
		server := a.settings.Server
		if a.opts.Server != "" {
			server = a.opts.Server
		}
		if server == "" {
			server = DefaultServer
		}
		name := a.settings.playerName()
		if a.opts.Name != "" {
			name = a.opts.Name
		}
		a.lobby.open(server, name)
		a.lobby.autoHost, a.lobby.autoJoin = a.opts.AutoHost, a.opts.AutoJoin
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
		// The game stays frozen, unless it's online: then it plays on
		// without us.
		if a.mode == ModeArena && a.arena.Online() {
			a.arena.inputBlocked = true
			a.arena.Update(dt, in, false)
			a.arena.inputBlocked = false
		}
		return
	}

	switch a.mode {
	case ModeOnline:
		start, back := a.lobby.update(in)
		switch {
		case back:
			a.lobby.close()
			a.enter(ModeMenu)
		case start != nil:
			a.startOnline(start)
		}
		a.run.Update(dt, in, false) // the menu's ride carries on behind
	case ModeMenu:
		if in.Pressed(input.KeyEscape) || in.PadPressed(input.PadB) {
			if a.menu.page != pageMain {
				a.menu.open(pageMain) // up a page
				return
			}
			if in.Pressed(input.KeyEscape) {
				a.quit = true
				return
			}
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
		if a.arena.wantsMenu {
			a.arena.wantsMenu = false
			a.choose(ModeMenu)
			return
		}
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
	if a.mode == ModeArena && a.arena.Online() {
		return []pauseAction{pauseResume, pauseSettings, pauseMainMenu} // (Main menu leaves the match)
	}
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
	case menuArenaPage:
		a.menu.open(pageArena)
		return
	case menuMorePage:
		a.menu.open(pageMore)
		return
	case menuBack:
		a.menu.open(pageMain)
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
	case ModeOnline:
		a.lobby.ui(b, a.in)
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
	menuSettings Mode = ModeOnline + 1 + iota
	menuQuit
	menuArenaPage // open the Arena page: against the bot, or online
	menuMorePage  // open the More page: the firing range, the engine demo
	menuBack      // back to the main page
)

// menu is the title screen over the self-playing run: a main page, and
// pages under Arena and More.
type menu struct {
	page   menuPage
	choice int
	move   *audio.Sound
	pick   *audio.Sound
}

type menuPage int

const (
	pageMain menuPage = iota
	pageArena
	pageMore
)

type menuItem struct {
	label string
	mode  Mode
}

var menuPages = [...][]menuItem{
	pageMain: {
		{"Run", ModeRun},
		{"Arena", menuArenaPage},
		{"More", menuMorePage},
		{"Settings", menuSettings},
		{"Quit", menuQuit},
	},
	pageArena: {
		{"Versus bot", ModeArena},
		{"Online", ModeOnline},
		{"Back", menuBack},
	},
	pageMore: {
		{"Firing Range", ModeRange},
		{"Engine Demo", ModeDemo},
		{"Back", menuBack},
	},
}

// items are the current page's items.
func (m *menu) items() []menuItem { return menuPages[m.page] }

// open goes to a page, its first item picked.
func (m *menu) open(p menuPage) { m.page, m.choice = p, 0 }

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
		n := len(m.items())
		m.choice = (m.choice + step + n) % n
		r.play(m.move, 1)
	}
	if confirmPressed(in) {
		r.play(m.pick, 1)
		return m.items()[m.choice].mode, true
	}
	return 0, false
}

// startOnline leaves the lobby for the Arena mode, playing the match the
// host started over the lobby's session.
func (a *App) startOnline(start *online.StartMsg) {
	if a.arena == nil {
		m, err := newArena(a.sc, a.opts.Audio, &a.settings, a.opts.Seed, false)
		if err != nil {
			logf("online: %v", err)
			return
		}
		a.arena = m
	}
	session := a.lobby.session
	a.lobby.session = nil // the match owns it now
	a.arena.startOnline(session, start)
	a.mode = ModeArena
}
