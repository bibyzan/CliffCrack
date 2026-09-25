package game

import (
	"math"
	"math/rand/v2"
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
	arenaNear      = 0.05 // close enough that the weapon model isn't clipped
	arenaFar       = 500

	tracerSpeed  = 260 // m/s: how fast a tracer streak travels down the bullet's path
	tracerLength = 5   // m
	flashTime    = 0.04
	holeLife     = 10 // s
	maxHoles     = 80
	feedLife     = 3.5 // s a kill-feed line stays up
	hitMarkTime  = 0.14
	maxBursts    = 160 // effect puffs alive at once
	breakSounds  = 3   // per frame, so a collapse doesn't deafen

	local = 0 // the player on this machine is player 0; the bot is player 1
)

// Bot difficulty choices, for the debug window.
var botSkills = []struct {
	name  string
	skill arena.BotSkill
}{{"easy", arena.BotEasy}, {"normal", arena.BotNormal}, {"hard", arena.BotHard}}

// arenaLight is the sky and lighting: a bright, hazy simulation space.
var arenaLight = struct {
	zenith     [4]float32
	haze       mathx.Vec3
	sun, shade mathx.Vec3
	sunDir     mathx.Vec3
	fog        float32
}{
	zenith: mathx.SRGB(0.42, 0.62, 0.92, 1), haze: srgb3(0.86, 0.90, 0.96),
	sun: srgb3(1.0, 0.95, 0.86).Scale(1.1), shade: srgb3(0.55, 0.65, 0.85).Scale(0.55),
	sunDir: mathx.Vec3{0.45, 0.8, 0.35}, fog: 0.006,
}

// tracer is the visible streak of one bullet.
type tracer struct {
	from, to mathx.Vec3
	age      float32
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

type hole struct {
	at, normal mathx.Vec3
	age        float32
}

type feedLine struct {
	text string
	good bool // you did it
	age  float32
}

type arenaSounds struct {
	shot, hit, headshot, hurt, kill, reload, empty, jump, land *audio.Sound
	swing, thud, launch, boom, swap, boost                     *audio.Sound
	tick, fight, win, lose                                     *audio.Sound
	breaks                                                     [arena.MaterialCount]*audio.Sound
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

	tracers  []tracer
	bursts   []burst
	holes    []hole
	feed     []feedLine
	flash    float32 // seconds of muzzle flash left
	hitMark  float32
	headMark bool    // the last hit marker was a headshot
	hurt     float32 // 0..1 red flash when you take damage
	shake    float32 // camera shake strength, decays
	bob      float32 // walk-cycle phase for the weapon sway
	strides  []float32
	prevPull bool // trigger state last frame (for the pad's "pressed")
	elapsed  float32
	glass    []render.DrawCmd // scratch: translucent draws, drawn after the solids
	viewProj mathx.Mat4       // last frame's, for placing name tags
	lastTick int              // the countdown second (or phase) last announced
	fovKick  float32          // 0..1 widening of the view as a launch pad throws you
}

func newArena(sc *scenery, mixer *audio.Mixer, settings *Settings, seed uint64) (*Arena, error) {
	m := &Arena{
		fixedSeed: seed,
		sc:        sc,
		sound:     mixer,
		settings:  settings,
		skill:     1,
		rng:       rand.New(rand.NewPCG(11, 13)),
		sfx: arenaSounds{
			shot:     audio.Blip(70*time.Millisecond, 1500, 170, 0.35),
			hit:      audio.Blip(35*time.Millisecond, 1900, 1700, 0.3),
			headshot: audio.Blip(60*time.Millisecond, 2600, 2400, 0.35),
			hurt:     audio.Blip(90*time.Millisecond, 220, 120, 0.6),
			kill:     audio.Blip(420*time.Millisecond, 380, 35, 1),
			reload:   audio.Blip(60*time.Millisecond, 900, 650, 0.35),
			empty:    audio.Blip(25*time.Millisecond, 2300, 2300, 0.2),
			jump:     audio.Blip(90*time.Millisecond, 300, 520, 0.25),
			land:     audio.Blip(70*time.Millisecond, 160, 80, 0.7),
			swing:    audio.Blip(140*time.Millisecond, 180, 420, 0.18),
			thud:     audio.Blip(120*time.Millisecond, 140, 60, 0.9),
			launch:   audio.Blip(110*time.Millisecond, 260, 120, 0.6),
			boom:     audio.Blip(650*time.Millisecond, 170, 28, 1),
			swap:     audio.Blip(35*time.Millisecond, 950, 950, 0.18),
			boost:    audio.Blip(450*time.Millisecond, 140, 900, 0.6),
			tick:     audio.Blip(90*time.Millisecond, 880, 880, 0.35),
			fight:    audio.Blip(300*time.Millisecond, 880, 1760, 0.45),
			win:      audio.Blip(600*time.Millisecond, 520, 1040, 0.5),
			lose:     audio.Blip(600*time.Millisecond, 440, 180, 0.5),
		},
	}
	m.sfx.breaks[arena.Wood] = audio.Blip(110*time.Millisecond, 420, 140, 0.45)
	m.sfx.breaks[arena.Brick] = audio.Blip(160*time.Millisecond, 260, 90, 0.55)
	m.sfx.breaks[arena.Concrete] = audio.Blip(200*time.Millisecond, 180, 60, 0.65)
	m.sfx.breaks[arena.Glass] = audio.Blip(150*time.Millisecond, 3200, 2300, 0.3)
	m.sfx.breaks[arena.Metal] = audio.Blip(260*time.Millisecond, 1200, 1100, 0.35)
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
	m.match = arena.NewMatch(seed, 2)
	bot := arena.NewBot(seed + 1)
	bot.Skill = botSkills[m.skill].skill
	m.bots = []*arena.Bot{nil, bot}
	m.autoBot = arena.NewBot(seed + 2)
	m.inputs = make([]arena.Input, 2)
	m.strides = make([]float32, 2)

	for _, mesh := range m.owned {
		render.DestroyMesh(mesh)
	}
	draws, meshes, err := m.as.levelDraws(m.sim().Level)
	m.owned = meshes
	if err != nil {
		return err
	}
	m.level = draws
	m.feed = m.feed[:0]
	m.newRound()
	return nil
}

// newRound clears the last round's effects and the bots' memories.
func (m *Arena) newRound() {
	m.round = m.match.Arena
	// Each end's trim takes the colour of the player who starts there.
	south, north := teamGlow[0], teamGlow[1]
	if m.me().Body.Position[2] < 0 {
		south, north = north, south
	}
	m.trim = m.as.trimDraws(m.round.Level, south, north)
	m.ends = [2][4]float32{south, north}
	m.fovKick = 0
	m.tracers, m.bursts, m.holes = m.tracers[:0], m.bursts[:0], m.holes[:0]
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
	held := in.MouseDown(input.MouseRight) && (mouseFree || m.locked)
	m.locked = !m.debugOpen || held

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
		default:
			m.inputs[i] = m.bots[i].Think(a, p, dt)
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
	s := m.settings.LookSensitivity
	c.Look = [2]float32{yaw * s, pitch * s * m.settings.lookSign()}

	pull := in.PadAxis(input.PadRightTrigger) > 0.5
	c.Fire = (m.locked && in.MouseDown(input.MouseLeft)) || pull
	c.FirePressed = (m.locked && in.MousePressed(input.MouseLeft)) || (pull && !m.prevPull)
	m.prevPull = pull

	c.Jump = in.Pressed(input.KeySpace) || in.PadPressed(input.PadA)
	c.Reload = in.Pressed(input.KeyR) || in.PadPressed(input.PadX)
	c.Sprint = in.Down(input.KeyLeftShift) || in.PadDown(input.PadLStick)

	// Weapons: 1/2/3, the mouse wheel, or the pad's shoulder buttons and Y.
	for i, k := range []input.Key{input.Key1, input.Key2, input.Key3} {
		if in.Pressed(k) {
			c.Select = i + 1
		}
	}
	switch {
	case m.locked && mouseFree && in.Scroll() < 0, in.PadPressed(input.PadRB), in.PadPressed(input.PadY):
		c.Cycle = 1
	case m.locked && mouseFree && in.Scroll() > 0, in.PadPressed(input.PadLB):
		c.Cycle = -1
	}
	return c
}

// playerName is how the HUD refers to a player.
func playerName(p *arena.Player) string {
	switch {
	case p == nil:
		return "THE SITE"
	case p.ID == local:
		return "YOU"
	}
	return "BOT"
}

// effects turns the step's events into tracers, bursts, holes, sounds, shake
// and HUD feedback, and ages the ones already running.
func (m *Arena) effects(dt float32, ev arena.Events) {
	me := m.me()
	for i := range m.tracers {
		m.tracers[i].age += dt
	}
	m.tracers = keep(m.tracers, func(t tracer) bool {
		return t.age*tracerSpeed-tracerLength < t.from.Sub(t.to).Len()
	})
	for i := range m.bursts {
		m.bursts[i].age += dt
	}
	m.bursts = keep(m.bursts, func(b burst) bool { return b.age < b.life })
	for i := range m.holes {
		m.holes[i].age += dt
	}
	m.holes = keep(m.holes, func(h hole) bool { return h.age < holeLife })
	for i := range m.feed {
		m.feed[i].age += dt
	}
	m.feed = keep(m.feed, func(f feedLine) bool { return f.age < feedLife })
	m.flash = max(m.flash-dt, 0)
	m.hitMark = max(m.hitMark-dt, 0)
	m.hurt *= float32(math.Exp(-3 * float64(dt)))
	m.shake *= float32(math.Exp(-6 * float64(dt)))
	m.fovKick *= float32(math.Exp(-2.5 * float64(dt)))

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

	for _, s := range ev.Shots {
		if s.By == me {
			m.flash = flashTime
			m.tracers = append(m.tracers, tracer{from: m.muzzle(), to: s.To})
			m.play(m.sfx.shot, 0.7)
		} else {
			m.tracers = append(m.tracers, tracer{from: weaponMuzzle(s.By), to: s.To})
			m.playAt(m.sfx.shot, s.From, 0.8)
		}
		switch {
		case s.Victim != nil:
			m.addBurst(burst{at: s.To, size: 0.1, life: 0.1, colour: suitColor[s.Victim.ID%len(suitColor)]})
		case s.Normal != (mathx.Vec3{}):
			m.addBurst(burst{at: s.To.Add(s.Normal.Scale(0.03)), size: 0.07, life: 0.07, colour: flashColor})
			if s.Chunk == nil || s.Chunk.Alive { // no hole in something that just broke
				if len(m.holes) == maxHoles {
					m.holes = m.holes[1:]
				}
				m.holes = append(m.holes, hole{at: s.To.Add(s.Normal.Scale(0.004)), normal: s.Normal})
			}
		}
	}
	for _, h := range ev.Hurts {
		switch {
		case h.Victim == me:
			m.hurt = min(m.hurt+0.25+h.Damage/60, 1)
			m.shake = max(m.shake, 0.25+h.Damage/100)
			m.play(m.sfx.hurt, 0.9)
		case h.By == me:
			m.hitMark, m.headMark = hitMarkTime, h.Head
			if h.Head {
				m.play(m.sfx.headshot, 0.9)
			} else {
				m.play(m.sfx.hit, 0.8)
			}
		}
	}
	for _, k := range ev.Kills {
		at := k.Victim.Body.Position.Add(mathx.Vec3{0, 0.8, 0})
		m.addBurst(burst{at: at, size: 1.1, life: 0.3, grow: true, colour: suitColor[k.Victim.ID%len(suitColor)]})
		m.addBurst(burst{at: at, size: 0.6, life: 0.18, grow: true, colour: tracerColor})
		how := arena.WeaponNames[k.Weapon]
		if k.Head {
			how += " · HEADSHOT"
		}
		line := feedLine{text: playerName(k.By) + "  [" + how + "]  " + playerName(k.Victim), good: k.By == me && k.Victim != me}
		if k.By == nil || k.By == k.Victim {
			line.text = playerName(k.Victim) + "  [" + how + "]  SELF"
		}
		m.feed = append(m.feed, line)
		m.playAt(m.sfx.kill, at, 1)
	}
	heard := 0
	for _, b := range ev.Breaks {
		// Dust: a puff the size of the piece, in its colour.
		size := max(b.Half[0], b.Half[1], b.Half[2]) * 1.4
		life := float32(0.55)
		if b.Collapsed {
			life = 0.9
		}
		m.addBurst(burst{at: b.At, size: size, life: life, grow: true, lit: true, colour: withAlpha(dustColor[b.Mat], 0.7)})
		if heard < breakSounds {
			m.playAt(m.sfx.breaks[b.Mat], b.At, 0.8)
			heard++
		}
	}
	eye := me.Eye(1)
	for _, s := range ev.Smashes {
		if s.By == me {
			m.shake = max(m.shake, 0.35)
		}
		m.addBurst(burst{at: s.At.Add(s.Normal.Scale(0.05)), size: 0.35, life: 0.15, grow: true, colour: flashColor})
		m.playAt(m.sfx.thud, s.At, 1)
	}
	for _, x := range ev.Explosions {
		at := x.At
		dist := at.Sub(eye).Len()
		m.shake = max(m.shake, 1.2*clampf(1-dist/25, 0.15, 1))
		m.addBurst(burst{at: at, size: arena.BlastRadius * 0.9, life: 0.35, grow: true, colour: explosionColor})
		m.addBurst(burst{at: at, size: arena.BlastRadius * 0.5, life: 0.2, grow: true, colour: tracerColor})
		m.addBurst(burst{at: at.Add(mathx.Vec3{0, 0.8, 0}), size: arena.BlastRadius * 1.1, life: 1.6, grow: true, lit: true,
			colour: withAlpha(smokeColor, 0.55)})
		m.playAt(m.sfx.boom, at, 1)
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
	yaw, pitch = p.Yaw, p.ViewPitch()
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
// the right, swaying as you walk, dropping out of view while switching, and
// animated by its own action (recoil, reload dip, hammer swing).
func (m *Arena) weaponModel() mathx.Mat4 {
	w := &m.me().Weapons
	offset := mathx.Vec3{0.16, -0.16, -0.4}
	offset[0] += 0.012 * float32(math.Sin(float64(m.bob)))
	offset[1] += 0.01 * float32(math.Abs(math.Cos(float64(m.bob))))
	offset[1] -= 0.35 * w.Switching / arena.SwitchTime // lowered while swapping
	var pitch, roll, yaw float32
	size := float32(0.55) // modelled at real scale; drawn this close they need shrinking

	if t, ok := w.Reloading(); ok {
		dip := float32(math.Sin(math.Pi * float64(t)))
		offset[1] -= 0.14 * dip
		pitch -= 0.35 * dip
		roll = 0.5 * dip
	}
	switch w.Current {
	case arena.WeaponRifle:
		offset[2] += 0.03 * w.Rifle.Kick
		pitch += 0.07 * w.Rifle.Kick
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
func (m *Arena) muzzle() mathx.Vec3 { return m.weaponModel().TransformPoint(rifleMuzzle) }

// Render returns the frame parameters and the draw list (appended to out[:0]).
func (m *Arena) Render(aspect float32, out []render.DrawCmd) (render.FrameParams, []render.DrawCmd) {
	eye := m.eye()
	fov := m.settings.fovRadians() * 1.25 * (1 + 0.18*m.fovKick) // a wider view suits first person; wider still in a launch
	proj := mathx.Perspective(min(fov, 1.9), aspect, arenaNear, arenaFar)
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
	out = append(out, m.glass...) // translucent: after every solid
	out = m.appendEffects(out)
	if !m.me().Dead {
		out = m.appendWeapon(out)
	}
	return params, m.appendHurt(out)
}

// appendStructures draws every standing chunk, darkening as it takes damage.
// Glass goes to m.glass, drawn after the solids.
func (m *Arena) appendStructures(out []render.DrawCmd) []render.DrawCmd {
	m.glass = m.glass[:0]
	for _, s := range m.sim().Structures {
		for _, c := range s.Chunks {
			if !c.Alive {
				continue
			}
			col := materialColor[c.Mat]
			shade := 0.55 + 0.45*c.Health()
			col[0], col[1], col[2] = col[0]*shade, col[1]*shade, col[2]*shade
			d := render.DrawCmd{
				Model:   mathx.Translate(c.Centre[0], c.Centre[1], c.Centre[2]).Mul(mathx.Scale(c.Half[0], c.Half[1], c.Half[2])),
				Color:   col,
				Texture: m.as.materials[c.Mat],
				Mesh:    m.as.cube,
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
		dc := render.DrawCmd{Model: model, Color: materialColor[d.Mat], Texture: m.as.materials[d.Mat], Mesh: m.as.cube}
		if d.Mat == arena.Glass {
			m.glass = append(m.glass, dc)
			continue
		}
		out = append(out, dc)
	}
	for _, g := range m.sim().Grenades {
		if g.Age < 0.06 && g.Owner == m.me() {
			continue // still leaving your barrel: drawn this close it would fill the view
		}
		pos, _ := g.Body.Interpolated(alpha)
		out = append(out,
			render.DrawCmd{Model: bodyMatrix(pos, mathx.QuatIdentity(), 0.07), Color: launcherGreen, Mesh: m.sc.ball},
			render.DrawCmd{Model: bodyMatrix(pos, mathx.QuatIdentity(), 0.035+0.015*float32(math.Sin(float64(m.elapsed)*40))),
				Color: grenadeGlow, Flags: gfx.DrawUnlit, Mesh: m.sc.ball})
	}
	return out
}

// appendEffects draws bullet holes, then the glowing, translucent effects.
func (m *Arena) appendEffects(out []render.DrawCmd) []render.DrawCmd {
	for _, h := range m.holes {
		fade := clampf((holeLife-h.age)/2, 0, 1)
		model := mathx.Translate(h.at[0], h.at[1], h.at[2]).Mul(alignUp(h.normal).Mat4()).Mul(mathx.Scale(0.045, 1, 0.045))
		out = append(out, render.DrawCmd{Model: model, Color: withAlpha(holeColor, 0.85*fade), Flags: gfx.DrawUnlit,
			Mesh: m.sc.shadow})
	}
	for _, t := range m.tracers {
		path := t.to.Sub(t.from)
		total := path.Len()
		head := min(t.age*tracerSpeed, total)
		tail := max(head-tracerLength, 0)
		if head <= tail {
			continue
		}
		dir := path.Scale(1 / total)
		mid := t.from.Add(dir.Scale((head + tail) / 2))
		model := mathx.Translate(mid[0], mid[1], mid[2]).Mul(mathx.LookRotation(dir).Mat4()).
			Mul(mathx.Scale(0.012, 0.012, (head-tail)/2))
		out = append(out, render.DrawCmd{Model: model, Color: withAlpha(tracerColor, 0.8), Flags: gfx.DrawUnlit,
			Mesh: m.as.cube})
	}
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

// appendWeapon draws the first-person weapon and, for the rifle, its muzzle flash.
func (m *Arena) appendWeapon(out []render.DrawCmd) []render.DrawCmd {
	model := m.weaponModel()
	parts := rifleParts
	switch m.me().Current {
	case arena.WeaponHammer:
		parts = hammerParts
	case arena.WeaponLauncher:
		parts = launcherParts
	}
	for _, part := range parts {
		c, h := part.centre, part.half
		pm := model.Mul(mathx.Translate(c[0], c[1], c[2])).Mul(mathx.Scale(h[0], h[1], h[2]))
		out = append(out, render.DrawCmd{Model: pm, Color: part.color, Flags: part.flags, Mesh: m.as.cube})
	}
	if m.flash > 0 && m.me().Current == arena.WeaponRifle {
		size := 0.014 + 0.012*m.rng.Float32() // it's only ~40 cm from the eye
		spin := mathx.AxisAngle(mathx.Vec3{0, 0, 1}, m.rng.Float32()*math.Pi)
		at := model.TransformPoint(rifleMuzzle.Add(mathx.Vec3{0, 0, -0.03}))
		out = append(out, render.DrawCmd{Model: bodyMatrix(at, spin, size), Color: withAlpha(flashColor, 0.9),
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
	if low := 1 - me.Health/arena.MaxHealth; low > 0.6 && !me.Dead {
		a = max(a, 0.2*(low-0.6)/0.4+0.05*float32(math.Sin(float64(m.elapsed)*5)))
	}
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
// with a pulse rising through it. The launch bays' pads stay dim until the
// round goes live.
func (m *Arena) appendPads(out []render.DrawCmd) []render.DrawCmd {
	s := m.sim()
	for _, p := range s.Pads {
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
		// Pulses rise off the pad and fade.
		for k := range 2 {
			t := float32(math.Mod(float64(m.elapsed)*0.9+float64(k)*0.5, 1))
			disc(0.05+t*1.6, p.Radius*(0.95-0.4*t), 0.45*glow*(1-t))
		}
	}
	return out
}
