package game

import (
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"CliffCrack/engine/audio"
	"CliffCrack/engine/camera"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/input"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/render"
	"CliffCrack/game/arena"
)

const (
	arenaMouseSens = 0.0022 // radians per pixel
	arenaStickYaw  = 3.2    // radians per second at full right-stick tilt
	arenaStickPch  = 2.2
	arenaNear      = 0.05     // close enough that the weapon model isn't clipped
	arenaFar       = farPlane // past the sky dome, and the chasm runs off into the haze

	flashTime   = 0.05
	feedLife    = 3.5 // s a kill-feed line stays up
	hitMarkTime = 0.14
	padHoldTime = 0.3 // s to hold the pad's X to pick up rather than reload
	maxBursts   = 160 // effect puffs alive at once
	breakSounds = 3   // per frame, so a collapse doesn't deafen
	pulseRise   = 0.7 // m a launch pad's pulses climb

)

// local is the player on this machine: player 0, except for a guest in an
// online match.
var local = 0

// Bot difficulty choices, for the debug window.
var botSkills = []struct {
	name  string
	skill arena.BotSkill
}{{"easy", arena.BotEasy}, {"normal", arena.BotNormal}, {"hard", arena.BotHard}}

// arenaLight is the sky and lighting: a clear mountain afternoon, the sun
// raking across the chasm so its walls stand out, and a cool haze that
// swallows the drop.
var arenaLight = struct {
	zenith     [4]float32
	haze       mathx.Vec3
	sun, shade mathx.Vec3
	sunDir     mathx.Vec3
	fog        float32
}{
	zenith: mathx.SRGB(0.33, 0.52, 0.84, 1), haze: srgb3(0.80, 0.85, 0.93),
	sun: srgb3(1.0, 0.92, 0.80).Scale(1.1), shade: srgb3(0.52, 0.62, 0.84).Scale(0.55),
	sunDir: mathx.Vec3{0.62, 0.6, 0.3}, fog: 0.0026,
}

// burst is a short-lived sphere: sparks, dust and explosions.
type burst struct {
	at     mathx.Vec3
	size   float32
	life   float32 // total
	age    float32
	grow   bool // expands (explosions, dust) rather than shrinking (sparks)
	lit    bool // shaded like a solid (dust, smoke) rather than glowing
	colour [4]float32
}

type feedLine struct {
	text string
	good bool // you did it
	age  float32
}

type arenaSounds struct {
	hit, headshot, hurt, kill, reload, empty, jump, land *audio.Sound
	swing, thud, launch, boom, swap, boost               *audio.Sound
	tick, fight, win, lose, collapse                     *audio.Sound
	armour, pop, recharge, splat                         *audio.Sound
	throw, stick, stickyBoom, pickup                     *audio.Sound
	magOut, magIn, charge, shellIn, pump                 *audio.Sound
	guns                                                 [len(arena.WeaponNames)]*audio.Sound
	breaks                                               [arena.MaterialCount]*audio.Sound
}

// Arena is the first-person mode: a best-of-three duel of single-life
// rounds against a bot, on a generated site of destructible buildings. The
// simulation lives in package arena; this maps the local player's input,
// runs the bot, draws it all and adds effects, sounds and the HUD.
type Arena struct {
	sc       *scenery
	sound    *audio.Mixer
	settings *Settings
	as       *arenaAssets
	sfx      arenaSounds
	rng      *rand.Rand

	match *arena.Match
	round *arena.Arena     // the round the effects belong to
	level []render.DrawCmd // the indestructible blocks
	trim  []render.DrawCmd // their glowing outlines, in the colours of whoever starts at each end
	ends  [2][4]float32    // south and north trim colours this round
	owned []render.Mesh    // the level's meshes, freed when a new site replaces it
	bots  []*arena.Bot     // per player; nil for the local player
	// Autopilot hands the local player to a bot too (for demos and scripted tests).
	Autopilot bool
	autoBot   *arena.Bot
	skill     int    // index into botSkills
	fixedSeed uint64 // non-zero: every match uses this seed (repeatable captures)
	inputs    []arena.Input

	debugOpen bool // the F1 window is up: the mouse is for the UI unless the right button is held
	locked    bool

	net          *netPlay // an online match (nil: against the bot)
	inputBlocked bool     // a menu's open over an online match: it plays on, without us
	lastIn       *input.State
	wantsMenu    bool // back to the main menu (the online match is over for us)

	// Practice is the firing range rather than a match against the bot.
	Practice    bool
	startWeapon string // hand the local player this weapon each round (for screenshots)

	balls      []paintball
	splats     []splat
	splatCount int
	bursts     []burst
	feed       []feedLine
	popped     float32 // 0..1 flash as your own armour breaks
	killMark   float32 // s left of the kill marker
	armourMark bool    // the last hit marker only marked their armour
	hitDirs    []hitDir
	charging   bool // your armour was recharging last frame
	lastShield float32
	flash      float32 // seconds of muzzle flash left
	hitMark    float32
	headMark   bool    // the last hit marker was a headshot
	hurt       float32 // 0..1 red flash when you take damage
	shake      float32 // camera shake strength, decays
	bob        float32 // walk-cycle phase for the weapon sway
	strides    []float32
	prevPull   bool    // trigger state last frame (for the pad's "pressed")
	padHold    float32 // s the pad's X has been held (tap: reload, hold: pick up)
	padHeld    bool    // ... and it's picked up this hold
	elapsed    float32
	glass      []render.DrawCmd // scratch: translucent draws, drawn after the solids
	viewProj   mathx.Mat4       // last frame's, for placing name tags
	lastTick   int              // the countdown second (or phase) last announced
	fovKick    float32          // 0..1 widening of the view as a launch pad throws you
	throwAnim  float32          // 0..1 the gun dipping as you throw a grenade
	reloadEase float32          // 0..1 the gun turned for a reload
	reloadT    float32          // how far through the reload it was last frame
	lastPump   float32          // the shotgun's time since its last shot, last frame
}

func newArena(sc *scenery, mixer *audio.Mixer, settings *Settings, seed uint64, practice bool) (*Arena, error) {
	m := &Arena{
		Practice:  practice,
		fixedSeed: seed,
		sc:        sc,
		sound:     mixer,
		settings:  settings,
		skill:     1,
		rng:       rand.New(rand.NewPCG(11, 13)),
		sfx:       newArenaSounds(),
	}
	as, err := newArenaAssets()
	if err != nil {
		return nil, err
	}
	m.as = as
	if err := m.start(); err != nil {
		return nil, err
	}
	return m, nil
}

// start begins a new match on a new site.
func (m *Arena) start() error {
	seed := m.fixedSeed
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	if m.Practice {
		m.match = arena.NewRange() // the dummies are driven by the match
		m.bots = make([]*arena.Bot, len(m.match.Arena.Players))
	} else {
		m.match = arena.NewMatch(seed, 2)
		bot := arena.NewBot(seed + 1)
		bot.Skill = botSkills[m.skill].skill
		m.bots = []*arena.Bot{nil, bot}
	}
	m.autoBot = arena.NewBot(seed + 2)
	n := len(m.match.Arena.Players)
	m.inputs = make([]arena.Input, n)
	m.strides = make([]float32, n)

	if err := m.buildLevel(); err != nil {
		return err
	}
	m.feed = m.feed[:0]
	m.newRound()
	return nil
}

// buildLevel makes the meshes for the match's site.
func (m *Arena) buildLevel() error {
	for _, mesh := range m.owned {
		render.DestroyMesh(mesh)
	}
	draws, meshes, err := m.as.levelDraws(m.sim().Level)
	m.owned = meshes
	m.level = draws
	return err
}

// newRound clears the last round's effects and the bots' memories.
func (m *Arena) newRound() {
	m.round = m.match.Arena
	for k, name := range arena.WeaponNames {
		if m.startWeapon != "" && strings.EqualFold(name, m.startWeapon) {
			me := m.me()
			if me.Other() == arena.WeaponKind(k) {
				me.Slots[1-me.Active] = me.Current // don't carry two
			}
			me.Slots[me.Active], me.Current = arena.WeaponKind(k), arena.WeaponKind(k)
		}
	}
	// Each end's trim takes the colour of the player who starts there.
	south, north := teamGlow[0], teamGlow[1]
	if m.me().Body.Position[2] < 0 {
		south, north = north, south
	}
	m.trim = m.as.trimDraws(m.round.Level, south, north)
	m.ends = [2][4]float32{south, north}
	m.fovKick = 0
	m.balls, m.splats, m.bursts = m.balls[:0], m.splats[:0], m.bursts[:0]
	m.popped, m.charging, m.killMark, m.hitDirs = 0, false, 0, m.hitDirs[:0]
	m.flash, m.hitMark, m.hurt, m.shake, m.elapsed = 0, 0, 0, 0, 0
	m.lastTick = 0
	clear(m.strides)
	for _, b := range append(m.bots, m.autoBot) {
		if b != nil {
			b.Reset()
		}
	}
}

// restart is start from the pause menu, logging (rather than returning) errors.
func (m *Arena) restart() {
	if err := m.start(); err != nil {
		logf("arena: restart: %v", err)
	}
}

// sim is the current round; me is the local player in it.
func (m *Arena) sim() *arena.Arena { return m.match.Arena }
func (m *Arena) me() *arena.Player { return m.match.Arena.Players[local] }

// Update advances the match. mouseFree is false while the UI has the mouse.
// Esc is handled by the App.
func (m *Arena) Update(dt float32, in *input.State, mouseFree bool) {
	m.elapsed += dt
	m.lastIn = in
	held := in.MouseDown(input.MouseRight) && (mouseFree || m.locked)
	m.locked = (!m.debugOpen || held) && !m.inputBlocked
	if m.net != nil {
		m.updateOnline(dt, in, mouseFree)
		return
	}

	if m.match.Phase == arena.PhaseMatchOver && m.match.Timer < -1 &&
		(confirmPressed(in) || (m.locked && in.MousePressed(input.MouseLeft))) {
		m.restart() // rematch
		return
	}

	a := m.sim()
	for i, p := range a.Players {
		switch {
		case i == local && m.Autopilot:
			m.inputs[i] = m.autoBot.Think(a, p, dt)
		case i == local:
			m.inputs[i] = m.input(in, dt, mouseFree)
		case m.bots[i] != nil:
			m.inputs[i] = m.bots[i].Think(a, p, dt)
		default:
			m.inputs[i] = arena.Input{} // a dummy on the range: the match moves it
		}
	}
	ev := m.match.Step(dt, m.inputs)
	for i, b := range m.bots {
		if b != nil {
			b.Hear(a, a.Players[i], &ev)
		}
	}
	if m.Autopilot {
		m.autoBot.Hear(a, a.Players[local], &ev)
	}
	m.effects(dt, ev)
	m.announce()
	if m.match.Arena != m.round {
		m.newRound()
	}
}

// announce plays the countdown ticks and the round results.
func (m *Arena) announce() {
	mt := m.match
	if mt.Practice {
		return
	}
	switch mt.Phase {
	case arena.PhaseCountdown:
		if s := int(math.Ceil(float64(mt.Timer))); s != m.lastTick && s > 0 {
			m.lastTick = s
			m.play(m.sfx.tick, 1)
		}
	case arena.PhaseFight:
		if m.lastTick != -1 {
			m.lastTick = -1
			m.play(m.sfx.fight, 1)
		}
	case arena.PhaseRoundOver:
		if m.lastTick != -2 {
			m.lastTick = -2
			switch {
			case mt.RoundWinner == local:
				m.play(m.sfx.win, 1)
			case mt.RoundWinner >= 0:
				m.play(m.sfx.lose, 1)
			}
		}
	}
}

// input maps keyboard, mouse and gamepad to the local player's intent.
func (m *Arena) input(in *input.State, dt float32, mouseFree bool) arena.Input {
	var c arena.Input
	c.Move[0] = in.Axis(input.KeyA, input.KeyD) + in.PadAxis(input.PadLeftX)
	c.Move[1] = in.Axis(input.KeyS, input.KeyW) - in.PadAxis(input.PadLeftY)
	if l := float32(math.Hypot(float64(c.Move[0]), float64(c.Move[1]))); l > 1 {
		c.Move[0], c.Move[1] = c.Move[0]/l, c.Move[1]/l
	}

	var yaw, pitch float32
	if m.locked {
		dx, dy := in.MouseDelta()
		yaw += float32(dx) * arenaMouseSens
		pitch -= float32(dy) * arenaMouseSens
	}
	if x, y := in.PadStick(true); x != 0 || y != 0 {
		yaw += x * arenaStickYaw * dt
		pitch -= y * arenaStickPch * dt
	}
	s := m.settings.LookSensitivity / m.me().Zoom() // finer through a scope
	c.Look = [2]float32{yaw * s, pitch * s * m.settings.lookSign()}
	// Aim down the sights: the right button (unless it's holding the look
	// with the F1 window open) or the left trigger.
	c.Aim = (m.locked && !m.debugOpen && in.MouseDown(input.MouseRight)) || in.PadAxis(input.PadLeftTrigger) > 0.4

	pull := in.PadAxis(input.PadRightTrigger) > 0.5
	c.Fire = (m.locked && in.MouseDown(input.MouseLeft)) || pull
	c.FirePressed = (m.locked && in.MousePressed(input.MouseLeft)) || (pull && !m.prevPull)
	m.prevPull = pull

	c.Jump = in.Pressed(input.KeySpace) || in.PadPressed(input.PadA)
	c.Sprint = in.Down(input.KeyLeftShift) || in.PadDown(input.PadLStick)
	c.Melee = in.Pressed(input.KeyF) || in.PadPressed(input.PadRB)
	c.Throw = in.Pressed(input.KeyG) || in.PadPressed(input.PadLB)
	c.SwitchGrenade = in.Pressed(input.KeyQ) || in.PadPressed(input.PadB)

	// R reloads, E picks up. On a pad, X does both, as in Halo: tap to
	// reload, hold to pick up.
	c.Reload = in.Pressed(input.KeyR)
	c.Interact = in.Pressed(input.KeyE)
	switch {
	case in.PadDown(input.PadX):
		m.padHold += dt
		if m.padHold >= padHoldTime && !m.padHeld {
			c.Interact, m.padHeld = true, true
		}
	case m.padHold > 0:
		c.Reload = c.Reload || !m.padHeld
		m.padHold, m.padHeld = 0, false
	}

	// The two weapons: 1 and 2, the mouse wheel, or the pad's Y.
	for i, k := range []input.Key{input.Key1, input.Key2} {
		if in.Pressed(k) {
			c.Select = i + 1
		}
	}
	if (m.locked && mouseFree && in.Scroll() != 0) || in.PadPressed(input.PadY) {
		c.Cycle = 1
	}
	return c
}

// playerName is how the HUD refers to a player.
func (m *Arena) playerName(p *arena.Player) string {
	switch {
	case p == nil:
		return "THE SITE"
	case p.ID == local:
		return "YOU"
	case m.net != nil:
		name, _ := m.netName(p)
		return strings.ToUpper(name)
	case m.Practice:
		return "DUMMY"
	}
	return "BOT"
}

// effects turns the step's events into paintballs, bursts, sounds, shake
// and HUD feedback, and ages the ones already running.
func (m *Arena) effects(dt float32, ev arena.Events) {
	me := m.me()
	m.updatePaint(dt)
	for i := range m.bursts {
		m.bursts[i].age += dt
	}
	m.bursts = keep(m.bursts, func(b burst) bool { return b.age < b.life })
	for i := range m.feed {
		m.feed[i].age += dt
	}
	m.feed = keep(m.feed, func(f feedLine) bool { return f.age < feedLife })
	m.flash = max(m.flash-dt, 0)
	m.hitMark = max(m.hitMark-dt, 0)
	m.killMark = max(m.killMark-dt, 0)
	for i := range m.hitDirs {
		m.hitDirs[i].age += dt
	}
	m.hitDirs = keep(m.hitDirs, func(d hitDir) bool { return d.age < hitDirLife })
	m.popped *= float32(math.Exp(-1.5 * float64(dt)))
	// Your armour starting to come back.
	if charging := me.Shield < arena.MaxShield && me.Shield > 0 && !me.Dead && me.Shield > m.lastShield; charging && !m.charging {
		m.play(m.sfx.recharge, 0.6)
		m.charging = true
	} else if !charging {
		m.charging = false
	}
	m.lastShield = me.Shield
	m.hurt *= float32(math.Exp(-3 * float64(dt)))
	m.shake *= float32(math.Exp(-6 * float64(dt)))
	m.fovKick *= float32(math.Exp(-2.5 * float64(dt)))
	m.throwAnim *= float32(math.Exp(-7 * float64(dt)))
	_, reloadingNow := me.Reloading()
	if reloadingNow && !me.Dead {
		m.reloadEase = min(m.reloadEase+dt*7, 1)
	} else {
		m.reloadEase = max(m.reloadEase-dt*5, 0)
	}
	m.reloadCues(me)

	// Walk cycles: the weapon sway, and everyone's legs.
	for i, p := range m.sim().Players {
		v := p.Body.Velocity
		if speed := float32(math.Hypot(float64(v[0]), float64(v[2]))); p.OnGround() && speed > 0.5 && !p.Dead {
			m.strides[i] += dt * speed * 1.6
			if p == me {
				m.bob = m.strides[i]
			}
		}
	}

	shotSounds := 0
	for _, s := range ev.Shots {
		from := weaponMuzzle(s.By)
		if s.By == me {
			m.flash = flashTime
			from = m.muzzle()
		}
		if shotSounds < 2 { // a shotgun blast is one sound, not twelve
			shotSounds++
			if snd := m.sfx.guns[s.Weapon]; snd != nil {
				if s.By == me {
					m.play(snd, 0.8)
				} else {
					m.playAt(snd, s.From, 0.9)
				}
			}
		}
		if g := arena.Guns[s.Weapon]; g != nil && len(m.balls) < maxBalls {
			m.balls = append(m.balls, paintball{from: from, to: s.To, normal: s.Normal, chunk: s.Chunk, speed: g.BallSpeed,
				size: ballSize(s.Weapon), colour: paintColor[team(s.By)], player: s.Victim != nil})
			if s.Victim != nil {
				m.balls[len(m.balls)-1].normal = mathx.Vec3{}
			}
		}
	}
	for _, h := range ev.Hurts {
		if h.Popped {
			// Armour breaking: a burst of the shooter's paint and a pop.
			at := h.Victim.Chest()
			col := paintColor[1-team(h.Victim)]
			m.addBurst(burst{at: at, size: 0.9, life: 0.25, grow: true, colour: withAlpha(col, 0.7)})
			m.addBurst(burst{at: at, size: 0.5, life: 0.4, grow: true, colour: withAlpha(uiWhite, 0.5)})
			m.playAt(m.sfx.pop, at, 1)
		}
		switch {
		case h.Victim == me:
			if h.Armour {
				m.hurt = min(m.hurt+0.1+h.Damage/200, 0.5)
				m.play(m.sfx.armour, 0.7)
			} else {
				m.hurt = min(m.hurt+0.25+h.Damage/60, 1)
				m.play(m.sfx.hurt, 0.9)
			}
			m.shake = max(m.shake, 0.2+h.Damage/150)
			if h.By != nil && h.By != me {
				m.hitDirs = append(m.hitDirs, hitDir{from: h.By.Chest()})
			} else if h.From != (mathx.Vec3{}) {
				m.hitDirs = append(m.hitDirs, hitDir{from: h.From})
			}
			if h.Popped {
				m.popped = 1
			}
		case h.By == me:
			m.hitMark, m.headMark, m.armourMark = hitMarkTime, h.Head, h.Armour
			switch {
			case h.Head:
				m.play(m.sfx.headshot, 0.9)
			case h.Armour:
				m.play(m.sfx.armour, 0.7)
			default:
				m.play(m.sfx.hit, 0.8)
			}
		}
	}
	for _, k := range ev.Kills {
		at := k.Victim.Body.Position.Add(mathx.Vec3{0, 0.8, 0})
		m.addBurst(burst{at: at, size: 1.1, life: 0.3, grow: true, colour: paintColor[1-team(k.Victim)]})
		m.addBurst(burst{at: at, size: 0.6, life: 0.18, grow: true, colour: suitColor[team(k.Victim)]})
		how := k.Weapon.Cause()
		if k.Head {
			how += " · HEADSHOT"
		}
		if k.By == me && k.Victim != me {
			m.killMark = killMarkLen
		}
		line := feedLine{text: m.playerName(k.By) + "  [" + how + "]  " + m.playerName(k.Victim), good: k.By == me && k.Victim != me}
		if k.By == nil || k.By == k.Victim {
			line.text = m.playerName(k.Victim) + "  [" + how + "]  SELF"
		}
		m.feed = append(m.feed, line)
		m.playAt(m.sfx.kill, at, 1)
	}
	heard := 0
	eye := me.Eye(1)
	var fell int          // pieces that collapsed this step ...
	var fellAt mathx.Vec3 // ... and their middle
	for _, b := range ev.Breaks {
		// Dust: a puff the size of the piece, in its colour, and a few
		// chips thrown off it.
		size := max(b.Half[0], b.Half[1], b.Half[2]) * 1.4
		life := float32(0.55)
		if b.Collapsed {
			life = 1.2
			fell++
			fellAt = fellAt.Add(b.At)
		}
		m.addBurst(burst{at: b.At, size: size, life: life, grow: true, lit: true, colour: withAlpha(dustColor[b.Mat], 0.7)})
		if !b.Collapsed {
			for range 2 {
				off := mathx.Vec3{m.rng.Float32() - 0.5, m.rng.Float32() - 0.3, m.rng.Float32() - 0.5}.Scale(size)
				m.addBurst(burst{at: b.At.Add(off), size: 0.08 + 0.1*m.rng.Float32(), life: 0.35, lit: true,
					colour: withAlpha(dustColor[b.Mat], 0.9)})
			}
		}
		if heard < breakSounds {
			m.playAt(m.sfx.breaks[b.Mat], b.At, 0.8)
			heard++
		}
	}
	if fell >= 6 {
		// Something big came down: a rumble and a shake, stronger close by.
		at := fellAt.Scale(1 / float32(fell))
		dist := at.Sub(eye).Len()
		m.shake = max(m.shake, min(float32(fell)/40, 1)*clampf(1-dist/40, 0.1, 1))
		m.playAt(m.sfx.collapse, at, min(float32(fell)/20, 1))
	}
	for _, s := range ev.Smashes {
		if s.By == me {
			m.shake = max(m.shake, 0.35)
		}
		m.addBurst(burst{at: s.At.Add(s.Normal.Scale(0.05)), size: 0.35, life: 0.15, grow: true, colour: flashColor})
		m.playAt(m.sfx.thud, s.At, 1)
	}
	for _, st := range ev.Stuck {
		m.playAt(m.sfx.stick, st.Grenade.Position(), 1)
	}
	for _, x := range ev.Explosions {
		at := x.At
		dist := at.Sub(eye).Len()
		m.shake = max(m.shake, 1.2*clampf(1-dist/25, 0.15, 1))
		fire := explosionColor
		switch x.Kind {
		case arena.Sticky:
			fire = paintColor[team(x.By)] // a burst of paint
		case arena.Frag:
			fire = lerpColor(explosionColor, paintColor[team(x.By)], 0.4)
		}
		m.addBurst(burst{at: at, size: arena.BlastRadius * 0.9, life: 0.35, grow: true, colour: fire})
		m.addBurst(burst{at: at, size: arena.BlastRadius * 0.5, life: 0.2, grow: true, colour: tracerColor})
		m.addBurst(burst{at: at.Add(mathx.Vec3{0, 0.8, 0}), size: arena.BlastRadius * 1.1, life: 1.6, grow: true, lit: true,
			colour: withAlpha(smokeColor, 0.55)})
		if x.Kind == arena.Sticky {
			m.playAt(m.sfx.stickyBoom, at, 1)
		} else {
			m.playAt(m.sfx.boom, at, 1)
		}
	}
	for _, act := range ev.Actions {
		mine := act.By == me
		var sound *audio.Sound
		volume := float32(0.8)
		switch act.Kind {
		case arena.ActSwing:
			sound = m.sfx.swing
		case arena.ActLaunch:
			sound, volume = m.sfx.launch, 0.9
			if mine {
				m.shake = max(m.shake, 0.15)
			}
		case arena.ActSwitch:
			sound, volume = m.sfx.swap, 1
		case arena.ActEmpty:
			sound, volume = m.sfx.empty, 1
		case arena.ActReload:
			sound = m.sfx.reload
		case arena.ActJump:
			sound, volume = m.sfx.jump, 0.6
		case arena.ActLand:
			sound, volume = m.sfx.land, min(1, act.Value/10)
		case arena.ActThrow:
			sound, volume = m.sfx.throw, 0.8
			if mine {
				m.throwAnim = 1
			}
		case arena.ActPickup:
			sound, volume = m.sfx.pickup, 0.9
		case arena.ActBoost:
			sound, volume = m.sfx.boost, 1
			if mine {
				m.shake, m.fovKick = max(m.shake, 0.5), 1
			}
		}
		switch {
		case sound == nil:
		case mine:
			m.play(sound, volume)
		default:
			m.playAt(sound, act.By.Body.Position, volume)
		}
	}
}

func (m *Arena) addBurst(b burst) {
	if len(m.bursts) < maxBursts {
		m.bursts = append(m.bursts, b)
	}
}

// lerpColor mixes a towards b by t (0..1).
func lerpColor(a, b [4]float32, t float32) [4]float32 {
	for i := range a {
		a[i] += (b[i] - a[i]) * t
	}
	return a
}

// keep filters s in place.
func keep[T any](s []T, ok func(T) bool) []T {
	out := s[:0]
	for _, v := range s {
		if ok(v) {
			out = append(out, v)
		}
	}
	clear(s[len(out):])
	return out
}

// CursorLocked reports whether the mouse should be captured for aiming.
func (m *Arena) CursorLocked() bool { return m.locked }

func (m *Arena) play(s *audio.Sound, volume float32) {
	if m.sound != nil {
		m.sound.Play(s, volume, 0)
	}
}

// playAt pans and attenuates a sound by where it is relative to the view.
func (m *Arena) playAt(s *audio.Sound, pos mathx.Vec3, volume float32) {
	if m.sound == nil {
		return
	}
	p := m.view().TransformPoint(pos) // camera space: +X right
	dist := p.Len()
	pan := float32(0)
	if dist > 0 {
		pan = p[0] / dist
	}
	m.sound.Play(s, volume/(1+0.04*dist), pan)
}

// eye is the camera position: the local player's eye, sinking to the
// ground after they die.
func (m *Arena) eye() mathx.Vec3 {
	me := m.me()
	eye := me.Eye(m.sim().Phys.Alpha())
	if me.Dead {
		t := clampf((m.sim().Time-me.DiedAt)/0.6, 0, 1)
		eye[1] -= (arena.EyeHeight + arena.PlayerRadius - 0.35) * smooth(t)
	}
	return eye
}

// viewAngles is the player's yaw and pitch plus camera shake.
func (m *Arena) viewAngles() (yaw, pitch float32) {
	p := m.me()
	yaw, pitch = p.ViewYaw(), p.ViewPitch()
	if m.shake > 0.001 {
		t := float64(m.elapsed)
		yaw += m.shake * 0.025 * float32(math.Sin(t*47)+0.5*math.Sin(t*83))
		pitch += m.shake * 0.02 * float32(math.Sin(t*53+1)+0.5*math.Sin(t*97))
	}
	return yaw, pitch
}

// camWorld is the camera's world transform (camera looks down its -Z).
func (m *Arena) camWorld() mathx.Mat4 {
	eye := m.eye()
	yaw, pitch := m.viewAngles()
	return mathx.Translate(eye[0], eye[1], eye[2]).Mul(mathx.RotateY(-yaw)).Mul(mathx.RotateX(pitch))
}

func (m *Arena) view() mathx.Mat4 {
	eye := m.eye()
	yaw, pitch := m.viewAngles()
	return mathx.LookAt(eye, eye.Add(camera.Direction(yaw, pitch)), mathx.Vec3{0, 1, 0})
}

// weaponModel places the current weapon in front of the camera: down and to
// the right at the hip, swaying as you walk, dropping out of view while
// switching, and animated by its own action (recoil, reload dip, hammer
// swing). With the sights up a gun comes to the middle, its sight on the
// line of the eye.
func (m *Arena) weaponModel() mathx.Mat4 {
	me := m.me()
	w := &me.Weapons
	ads := smooth(w.ADS)
	offset := mathx.Vec3{0.16, -0.16, -0.4}
	sway := 1 - 0.85*ads
	offset[0] += 0.012 * sway * float32(math.Sin(float64(m.bob)))
	offset[1] += 0.01 * sway * float32(math.Abs(math.Cos(float64(m.bob))))
	offset[1] -= 0.35 * w.Switching / arena.SwitchTime // lowered while swapping
	var pitch, roll, yaw float32
	size := float32(0.55) // modelled at real scale; drawn this close they need shrinking
	// Throwing a grenade: the gun dips out of the way of the throwing arm.
	offset[1] -= 0.22 * m.throwAnim
	pitch -= 0.5 * m.throwAnim

	// Reloading: the gun turns to show where the ammo goes, and comes in a
	// little, for the supporting hand to work on it.
	if e := smooth(m.reloadEase); e > 0 {
		style := reloadMag
		if mk, ok := m.markerFor(heldKind(w)); ok {
			style = mk.reload
		}
		switch style {
		case reloadShells: // the port underneath, turned towards you
			roll, pitch = 0.75*e, 0.15*e
		case reloadDrum: // the drum's side up
			roll, pitch = -0.35*e, 0.2*e
		default: // the magazine well
			roll, pitch = 0.35*e, 0.12*e
		}
		offset = offset.Add(mathx.Vec3{-0.04 * e, 0.02 * e, 0.05 * e})
	}
	switch held := heldKind(w); held {
	case arena.WeaponLauncher:
		size = 0.45
		offset = offset.Add(mathx.Vec3{0.03, -0.02, 0.06 * w.Launcher.Kick})
		pitch += 0.18 * w.Launcher.Kick
	case arena.WeaponHammer:
		// Held up at the right, head leaning over; a swing winds up, then
		// brings the head down across the centre of the view and recovers.
		size = 0.7
		offset = offset.Add(mathx.Vec3{0.08, -0.12, -0.05})
		pitch, roll = -0.35, 0.3
		s := hammerPose(w.Hammer.Progress())
		pitch -= 1.25 * s
		yaw = 0.35 * max(s, 0)
		offset[0] -= 0.12 * max(s, 0)
		offset[2] -= 0.1 * max(s, 0)
	default:
		mk, ok := m.markerFor(held)
		if !ok {
			break
		}
		kick := w.States[held].Kick
		size = mk.size
		// Sights up: the sight sits on the eye's line, a hand's width out.
		aimed := mathx.Vec3{0, 0, -mk.relief}.Sub(mk.sight.Scale(size))
		offset = offset.Add(aimed.Sub(offset).Scale(ads))
		offset[2] += 0.03 * kick
		pitch += 0.07 * kick * (1 - 0.6*ads)
		roll *= 1 - ads
	}
	return m.camWorld().
		Mul(mathx.Translate(offset[0], offset[1], offset[2])).
		Mul(mathx.RotateY(yaw)).
		Mul(mathx.RotateX(pitch)).
		Mul(mathx.RotateZ(roll)).
		Mul(mathx.Scale(size, size, size))
}

// smooth is smoothstep on 0..1.
func smooth(t float32) float32 {
	t = clampf(t, 0, 1)
	return t * t * (3 - 2*t)
}

// muzzle is the rifle's barrel tip in world space.
func (m *Arena) muzzle() mathx.Vec3 {
	if mk, ok := m.markerFor(m.me().Current); ok {
		return m.weaponModel().TransformPoint(mk.muzzle)
	}
	return m.eye()
}

// Render returns the frame parameters and the draw list (appended to out[:0]).
func (m *Arena) Render(aspect float32, out []render.DrawCmd) (render.FrameParams, []render.DrawCmd) {
	eye := m.eye()
	fov := m.settings.fovRadians() * 1.25 * (1 + 0.18*m.fovKick) // a wider view suits first person; wider still in a launch
	fov = min(fov, 1.9)
	zoom := m.me().Zoom()
	fov = 2 * float32(math.Atan(math.Tan(float64(fov)/2)/float64(zoom))) // aiming down the sights magnifies
	proj := mathx.Perspective(fov, aspect, arenaNear, arenaFar)
	l := &arenaLight
	m.viewProj = proj.Mul(m.view())
	params := render.FrameParams{
		ViewProj:     m.viewProj,
		CameraPos:    eye,
		SunDirection: l.sunDir,
		SunColor:     l.sun,
		Ambient:      l.shade,
		FogColor:     l.haze,
		FogDensity:   l.fog,
		Clear:        [4]float32{l.haze[0], l.haze[1], l.haze[2], 1},
	}

	out = append(out[:0], render.DrawCmd{
		Model: mathx.Translate(eye[0], eye[1], eye[2]).Mul(mathx.Scale(skyRadius, skyRadius, skyRadius)),
		Color: l.zenith, Flags: gfx.DrawSky, Mesh: m.sc.sky,
	})
	out = m.as.chasm.appendDraws(out, !m.Practice) // the range runs where the ridges would be
	out = append(out, m.level...)
	out = append(out, m.trim...)
	out = m.appendPads(out)
	out = m.appendStructures(out)
	for i, p := range m.sim().Players {
		if p != m.me() {
			out = m.appendCharacter(out, p, m.strides[i])
		}
	}
	out = m.appendDebris(out)
	out = m.appendPickups(out)
	out = m.appendPaint(out)
	out = append(out, m.glass...) // translucent: after every solid
	out = m.appendEffects(out)
	scoped := false
	if mk, ok := m.markerFor(m.me().Current); ok && mk.scope && m.me().ADS > 0.85 {
		scoped = true // looking through the scope, not at the gun
	}
	if !m.me().Dead && !scoped {
		out = m.appendWeapon(out)
	}
	out = m.appendHurt(out)
	out = m.appendReticle(out, fov)
	return params, m.appendHelmet(out, fov, aspect)
}

// appendStructures draws every standing chunk, darkening as it takes damage,
// and the glowing trim on the ones that carry the arena's outline. Glass
// goes to m.glass, drawn after the solids.
func (m *Arena) appendStructures(out []render.DrawCmd) []render.DrawCmd {
	m.glass = m.glass[:0]
	for _, s := range m.sim().Structures {
		for _, c := range s.Chunks {
			if !c.Alive {
				continue
			}
			if c.Trim {
				h := c.Half
				model := mathx.Translate(c.Centre[0], c.Top()-0.08, c.Centre[2]).Mul(mathx.Scale(h[0]+0.012, 0.04, h[2]+0.012))
				out = append(out, render.DrawCmd{Model: model, Color: trimColor(c.Centre, m.ends[0], m.ends[1]),
					Flags: gfx.DrawUnlit, Mesh: m.as.cube})
			}
			col := materialColor[c.Mat]
			shade := 0.55 + 0.45*c.Health()
			col[0], col[1], col[2] = col[0]*shade, col[1]*shade, col[2]*shade
			d := render.DrawCmd{
				Model:   mathx.Translate(c.Centre[0], c.Centre[1], c.Centre[2]).Mul(mathx.Scale(c.Half[0], c.Half[1], c.Half[2])),
				Color:   col,
				Texture: m.as.materials[c.Mat],
				Mesh:    m.as.bevel,
			}
			if c.Mat == arena.Glass {
				m.glass = append(m.glass, d)
			} else {
				out = append(out, d)
			}
		}
	}
	return out
}

// appendDebris draws rubble as tumbling boxes (glass shards translucent) and
// grenades in flight; pieces shrink away at the end of their life.
func (m *Arena) appendDebris(out []render.DrawCmd) []render.DrawCmd {
	alpha := m.sim().Phys.Alpha()
	for _, d := range m.sim().Debris {
		pos, rot := d.Body.Interpolated(alpha)
		h := d.Half.Scale(clampf((d.Life-d.Age)/0.6, 0, 1))
		model := mathx.Translate(pos[0], pos[1], pos[2]).Mul(rot.Mat4()).Mul(mathx.Scale(h[0], h[1], h[2]))
		dc := render.DrawCmd{Model: model, Color: materialColor[d.Mat], Texture: m.as.materials[d.Mat], Mesh: m.as.bevel}
		if d.Mat == arena.Glass {
			m.glass = append(m.glass, dc)
			continue
		}
		out = append(out, dc)
	}
	for _, g := range m.sim().Grenades {
		if g.Age < 0.06 && g.Owner == m.me() {
			continue // still leaving your hand: drawn this close it would fill the view
		}
		out = m.appendGrenade(out, g, alpha)
	}
	return out
}

// appendEffects draws the glowing, translucent effects.
func (m *Arena) appendEffects(out []render.DrawCmd) []render.DrawCmd {
	for _, b := range m.bursts {
		t := b.age / b.life
		size := b.size * (1 - t)
		if b.grow {
			size = b.size * (0.3 + 0.7*t)
		}
		flags := gfx.DrawUnlit
		if b.lit {
			flags = gfx.DrawFlat
		}
		out = append(out, render.DrawCmd{Model: bodyMatrix(b.at, mathx.QuatIdentity(), size),
			Color: withAlpha(b.colour, 1-t*t), Flags: flags, Mesh: m.sc.chip})
	}
	return out
}

// appendWeapon draws the first-person weapon and, for a gun, the puff of
// gas and paint at its muzzle as it fires.
func (m *Arena) appendWeapon(out []render.DrawCmd) []render.DrawCmd {
	model := m.weaponModel()
	held := heldKind(&m.me().Weapons)
	switch held {
	case arena.WeaponHammer:
		out = m.drawParts(out, model, hammerParts)
		return m.appendArms(out, model, nil, reloadPose{}, false)
	}
	mk, ok := m.markerFor(held)
	if !ok {
		return out // empty-handed
	}
	out = m.drawParts(out, model, mk.parts)
	// The parts that move: the pump, and the magazine through a reload.
	var pose reloadPose
	t, reloadingNow := m.me().Reloading()
	if reloadingNow {
		pose = reloading(mk, t)
	}
	if mk.pump != nil {
		out = m.drawParts(out, model.Mul(translate(m.pumpOffset())), mk.pump)
	}
	if mk.mag != nil {
		out = m.drawParts(out, model.Mul(translate(pose.magOff)), mk.mag)
	}
	out = m.appendArms(out, model, mk, pose, reloadingNow)
	if m.flash > 0 {
		size := 0.018 + 0.01*m.rng.Float32() // it's only ~40 cm from the eye
		at := model.TransformPoint(mk.muzzle.Add(mathx.Vec3{0, 0, -0.03}))
		out = append(out, render.DrawCmd{Model: bodyMatrix(at, mathx.QuatIdentity(), size), Color: withAlpha(uiWhite, 0.55),
			Flags: gfx.DrawUnlit, Mesh: m.sc.chip})
	}
	return out
}

// appendHurt tints the view red: a flash when you're hit, a pulse while
// your health is low, and a steady wash once you're down. It's a disc just
// in front of the camera.
func (m *Arena) appendHurt(out []render.DrawCmd) []render.DrawCmd {
	me := m.me()
	a := 0.45 * m.hurt
	if me.Popped() && !me.Dead {
		// Popped: a heartbeat, harder the less health there is.
		low := 1 - me.Health/arena.MaxHealth
		a = max(a, 0.08+0.12*low+(0.05+0.05*low)*float32(math.Sin(float64(m.elapsed)*6)))
	}
	a = max(a, 0.5*m.popped)
	if me.Dead {
		a = 0.3
	}
	if a < 0.01 {
		return out
	}
	model := m.camWorld().Mul(mathx.Translate(0, 0, -0.06)).Mul(mathx.RotateX(math.Pi / 2)).Mul(mathx.Scale(0.3, 1, 0.3))
	return append(out, render.DrawCmd{Model: model, Color: withAlpha(hurtColor, a), Flags: gfx.DrawUnlit, Mesh: m.sc.shadow})
}

// project maps a world point to the screen as fractions (0..1 from the top
// left), and reports whether it's in front of the camera and on screen.
func (m *Arena) project(p mathx.Vec3) (x, y float32, ok bool) {
	v := &m.viewProj
	cx := v[0]*p[0] + v[4]*p[1] + v[8]*p[2] + v[12]
	cy := v[1]*p[0] + v[5]*p[1] + v[9]*p[2] + v[13]
	cw := v[3]*p[0] + v[7]*p[1] + v[11]*p[2] + v[15]
	if cw <= 0.05 {
		return 0, 0, false
	}
	x, y = cx/cw*0.5+0.5, cy/cw*0.5+0.5 // clip-space Y points down
	return x, y, x > 0.02 && x < 0.98 && y > 0.02 && y < 0.98
}

// appendPads draws the launch pads: a glowing disc in their end's colour
// with a pulse rising off it. The launch bays' pads stay dim until the
// round goes live; a pad whose floor has been blown out is gone.
func (m *Arena) appendPads(out []render.DrawCmd) []render.DrawCmd {
	s := m.sim()
	for _, p := range s.Pads {
		if !s.PadWorks(p) {
			continue
		}
		col := m.ends[0]
		if p.Centre[2] < 0 {
			col = m.ends[1]
		}
		glow := float32(0.8)
		if p.Spawn && !s.Live {
			glow = 0.3 + 0.15*float32(math.Sin(float64(m.elapsed)*4))
		}
		c := p.Centre
		disc := func(y, r, a float32) {
			model := mathx.Translate(c[0], c[1]+y, c[2]).Mul(mathx.Scale(r, 1, r))
			out = append(out, render.DrawCmd{Model: model, Color: withAlpha(col, a), Flags: gfx.DrawUnlit, Mesh: m.sc.shadow})
		}
		disc(0.012, p.Radius, 0.35*glow+0.2)
		disc(0.02, p.Radius*0.55, 0.6*glow)
		// Pulses rise off the pad and fade, well below eye height: standing
		// on a pad, a disc passing the camera would wash over the view.
		for k := range 2 {
			t := float32(math.Mod(float64(m.elapsed)*0.9+float64(k)*0.5, 1))
			disc(0.05+t*pulseRise, p.Radius*(0.95-0.4*t), 0.45*glow*(1-t))
		}
	}
	return out
}
