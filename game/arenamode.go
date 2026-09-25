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
	feedLife     = 1.6 // s a kill-feed line stays up
	hitMarkTime  = 0.12
	maxBursts    = 160 // effect puffs alive at once
	breakSounds  = 3   // per frame, so a collapse doesn't deafen
)

// arenaLight is a mode's sky and lighting.
type arenaLight struct {
	zenith     [4]float32
	haze       mathx.Vec3
	sun, shade mathx.Vec3
	sunDir     mathx.Vec3
	fog        float32
}

var (
	// Arena: dusk, violet haze.
	duskLight = arenaLight{
		zenith: mathx.SRGB(0.10, 0.14, 0.32, 1), haze: srgb3(0.52, 0.44, 0.58),
		sun: srgb3(1.0, 0.74, 0.52).Scale(1.05), shade: srgb3(0.42, 0.50, 0.72).Scale(0.5),
		sunDir: mathx.Vec3{-0.55, 0.42, -0.62}, fog: 0.011,
	}
	// Demolition: a clear afternoon, so the materials read.
	dayLight = arenaLight{
		zenith: mathx.SRGB(0.30, 0.52, 0.86, 1), haze: srgb3(0.78, 0.84, 0.92),
		sun: srgb3(1.0, 0.95, 0.86).Scale(1.1), shade: srgb3(0.55, 0.65, 0.85).Scale(0.55),
		sunDir: mathx.Vec3{0.45, 0.8, 0.35}, fog: 0.006,
	}
)

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
	age  float32
}

type arenaSounds struct {
	shot, hit, kill, reload, empty, jump, land *audio.Sound
	swing, thud, launch, boom, swap            *audio.Sound
	breaks                                     [6]*audio.Sound // by material
}

// Arena is the first-person mode, in two flavours: Arena (a fixed level at
// dusk with drones to shoot) and Demolition (a random site of destructible
// buildings to take apart). The simulation lives in package arena; this
// draws it, maps the player's input and adds effects and sounds.
type Arena struct {
	sc       *scenery
	sound    *audio.Mixer
	settings *Settings
	as       *arenaAssets
	sfx      arenaSounds
	rng      *rand.Rand
	site     bool // Demolition: generate a random site each match
	light    arenaLight

	sim   *arena.Arena
	level []render.DrawCmd // the indestructible blocks
	owned []render.Mesh    // the level's meshes, freed when a new site replaces it
	// Autopilot plays the match (for demos and scripted tests).
	Autopilot bool
	fixedSeed uint64 // non-zero: every match uses this seed (repeatable captures)

	debugOpen bool // the F1 window is up: the mouse is for the UI unless the right button is held
	locked    bool

	tracers  []tracer
	bursts   []burst
	holes    []hole
	feed     []feedLine
	flash    float32 // seconds of muzzle flash left
	hitMark  float32
	shake    float32 // camera shake strength, decays
	bob      float32 // walk-cycle phase for the weapon sway
	prevPull bool    // trigger state last frame (for the pad's "pressed")
	elapsed  float32
	glass    []render.DrawCmd // scratch: translucent draws, drawn after the solids
}

func newArena(sc *scenery, mixer *audio.Mixer, settings *Settings, seed uint64, site bool) (*Arena, error) {
	m := &Arena{
		fixedSeed: seed,
		sc:        sc,
		sound:     mixer,
		settings:  settings,
		site:      site,
		light:     duskLight,
		rng:       rand.New(rand.NewPCG(11, 13)),
		sfx: arenaSounds{
			shot:   audio.Blip(70*time.Millisecond, 1500, 170, 0.35),
			hit:    audio.Blip(35*time.Millisecond, 1900, 1700, 0.3),
			kill:   audio.Blip(420*time.Millisecond, 380, 35, 1),
			reload: audio.Blip(60*time.Millisecond, 900, 650, 0.35),
			empty:  audio.Blip(25*time.Millisecond, 2300, 2300, 0.2),
			jump:   audio.Blip(90*time.Millisecond, 300, 520, 0.25),
			land:   audio.Blip(70*time.Millisecond, 160, 80, 0.7),
			swing:  audio.Blip(140*time.Millisecond, 180, 420, 0.18),
			thud:   audio.Blip(120*time.Millisecond, 140, 60, 0.9),
			launch: audio.Blip(110*time.Millisecond, 260, 120, 0.6),
			boom:   audio.Blip(650*time.Millisecond, 170, 28, 1),
			swap:   audio.Blip(35*time.Millisecond, 950, 950, 0.18),
		},
	}
	m.sfx.breaks[arena.Wood] = audio.Blip(110*time.Millisecond, 420, 140, 0.45)
	m.sfx.breaks[arena.Brick] = audio.Blip(160*time.Millisecond, 260, 90, 0.55)
	m.sfx.breaks[arena.Concrete] = audio.Blip(200*time.Millisecond, 180, 60, 0.65)
	m.sfx.breaks[arena.Glass] = audio.Blip(150*time.Millisecond, 3200, 2300, 0.3)
	m.sfx.breaks[arena.Metal] = audio.Blip(260*time.Millisecond, 1200, 1100, 0.35)
	m.sfx.breaks[arena.Scrap] = m.sfx.breaks[arena.Metal]
	if site {
		m.light = dayLight
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

// start begins a new match (on a new site, in Demolition).
func (m *Arena) start() error {
	seed := m.fixedSeed
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	if m.site {
		m.sim = arena.NewSite(seed)
	} else {
		m.sim = arena.New(seed)
	}
	if m.site || m.level == nil {
		for _, mesh := range m.owned {
			render.DestroyMesh(mesh)
		}
		draws, meshes, err := m.as.levelDraws(m.sim.Level)
		m.owned = meshes
		if err != nil {
			return err
		}
		m.level = draws
	}
	m.tracers, m.bursts, m.holes, m.feed = m.tracers[:0], m.bursts[:0], m.holes[:0], m.feed[:0]
	m.flash, m.hitMark, m.shake, m.elapsed = 0, 0, 0, 0
	return nil
}

// restart is start from the pause menu, logging (rather than returning) errors.
func (m *Arena) restart() {
	if err := m.start(); err != nil {
		logf("arena: restart: %v", err)
	}
}

// Update advances the match. mouseFree is false while the UI has the mouse.
// Esc is handled by the App.
func (m *Arena) Update(dt float32, in *input.State, mouseFree bool) {
	m.elapsed += dt
	held := in.MouseDown(input.MouseRight) && (mouseFree || m.locked)
	m.locked = !m.debugOpen || held

	var ctl arena.Input
	if m.Autopilot {
		ctl = m.sim.Autopilot(dt)
	} else {
		ctl = m.input(in, dt, mouseFree)
	}
	ev := m.sim.Step(dt, ctl)
	m.effects(dt, ev)
}

// input maps keyboard, mouse and gamepad to the player's intent.
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

// effects turns the step's events into tracers, bursts, holes, sounds, shake
// and HUD feedback, and ages the ones already running.
func (m *Arena) effects(dt float32, ev arena.Events) {
	p := &m.sim.Player
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
	m.shake *= float32(math.Exp(-6 * float64(dt)))

	// Weapon sway follows walking speed on the ground.
	v := p.Body.Velocity
	if speed := float32(math.Hypot(float64(v[0]), float64(v[2]))); p.OnGround() && speed > 0.5 {
		m.bob += dt * speed * 1.6
	}

	for _, s := range ev.Shots {
		m.flash = flashTime
		m.tracers = append(m.tracers, tracer{from: m.muzzle(), to: s.To})
		m.play(m.sfx.shot, 0.7)
		switch {
		case s.Drone != nil:
			m.hitMark = hitMarkTime
			m.addBurst(burst{at: s.To, size: 0.12, life: 0.08, colour: tracerColor})
			m.playAt(m.sfx.hit, s.To, 0.8)
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
	for _, d := range ev.Kills {
		at := d.Body.Position
		m.addBurst(burst{at: at, size: 1.3, life: 0.28, grow: true, colour: explosionColor})
		m.addBurst(burst{at: at, size: 0.7, life: 0.16, grow: true, colour: tracerColor})
		m.feed = append(m.feed, feedLine{text: "+100  DRONE DOWN"})
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
	for _, s := range ev.Smashes {
		m.shake = max(m.shake, 0.35)
		m.addBurst(burst{at: s.At.Add(s.Normal.Scale(0.05)), size: 0.35, life: 0.15, grow: true, colour: flashColor})
		m.playAt(m.sfx.thud, s.At, 1)
	}
	for _, at := range ev.Explosions {
		dist := at.Sub(m.sim.Player.Eye(1)).Len()
		m.shake = max(m.shake, 1.2*clampf(1-dist/25, 0.15, 1))
		m.addBurst(burst{at: at, size: arena.BlastRadius * 0.9, life: 0.35, grow: true, colour: explosionColor})
		m.addBurst(burst{at: at, size: arena.BlastRadius * 0.5, life: 0.2, grow: true, colour: tracerColor})
		m.addBurst(burst{at: at.Add(mathx.Vec3{0, 0.8, 0}), size: arena.BlastRadius * 1.1, life: 1.6, grow: true, lit: true,
			colour: withAlpha(smokeColor, 0.55)})
		m.playAt(m.sfx.boom, at, 1)
	}
	if ev.Swung {
		m.play(m.sfx.swing, 0.8)
	}
	if ev.Launched {
		m.play(m.sfx.launch, 0.9)
		m.shake = max(m.shake, 0.15)
	}
	if ev.Switched {
		m.play(m.sfx.swap, 1)
	}
	switch {
	case ev.Empty:
		m.play(m.sfx.empty, 1)
	case ev.Reloaded:
		m.play(m.sfx.reload, 0.8)
	}
	if ev.Jumped {
		m.play(m.sfx.jump, 0.6)
	}
	if ev.Landed > 0 {
		m.play(m.sfx.land, min(1, ev.Landed/10))
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

// viewAngles is the player's yaw and pitch plus camera shake.
func (m *Arena) viewAngles() (yaw, pitch float32) {
	p := &m.sim.Player
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
	eye := m.sim.Player.Eye(m.sim.Phys.Alpha())
	yaw, pitch := m.viewAngles()
	return mathx.Translate(eye[0], eye[1], eye[2]).Mul(mathx.RotateY(-yaw)).Mul(mathx.RotateX(pitch))
}

func (m *Arena) view() mathx.Mat4 {
	eye := m.sim.Player.Eye(m.sim.Phys.Alpha())
	yaw, pitch := m.viewAngles()
	return mathx.LookAt(eye, eye.Add(camera.Direction(yaw, pitch)), mathx.Vec3{0, 1, 0})
}

// weaponModel places the current weapon in front of the camera: down and to
// the right, swaying as you walk, dropping out of view while switching, and
// animated by its own action (recoil, reload dip, hammer swing).
func (m *Arena) weaponModel() mathx.Mat4 {
	w := &m.sim.Weapons
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
		if p := w.Hammer.Progress(); p >= 0 {
			strike := float32(arena.HammerHitAt / arena.HammerSwing)
			var s float32 // 0 rest, -0.4 wound up, 1 struck
			switch {
			case p < strike*0.45:
				s = -0.4 * smooth(p/(strike*0.45))
			case p < strike:
				s = -0.4 + 1.4*smooth((p-strike*0.45)/(strike*0.55))
			default:
				s = 1 - smooth((p-strike)/(1-strike))
			}
			pitch -= 1.25 * s
			yaw = 0.35 * max(s, 0)
			offset[0] -= 0.12 * max(s, 0)
			offset[2] -= 0.1 * max(s, 0)
		}
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
	eye := m.sim.Player.Eye(m.sim.Phys.Alpha())
	fov := m.settings.fovRadians() * 1.25 // a wider view suits first person
	proj := mathx.Perspective(min(fov, 1.9), aspect, arenaNear, arenaFar)
	l := &m.light
	params := render.FrameParams{
		ViewProj:     proj.Mul(m.view()),
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
	out = m.appendStructures(out)
	out = m.appendDrones(out, eye)
	out = m.appendDebris(out)
	out = append(out, m.glass...) // translucent: after every solid
	out = m.appendEffects(out)
	return params, m.appendWeapon(out)
}

// appendStructures draws every standing chunk, darkening as it takes damage.
// Glass goes to m.glass, drawn after the solids.
func (m *Arena) appendStructures(out []render.DrawCmd) []render.DrawCmd {
	m.glass = m.glass[:0]
	for _, s := range m.sim.Structures {
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

func (m *Arena) appendDrones(out []render.DrawCmd, eye mathx.Vec3) []render.DrawCmd {
	alpha := m.sim.Phys.Alpha()
	for i, d := range m.sim.Drones {
		if d.Dead {
			continue
		}
		pos, _ := d.Body.Interpolated(alpha)
		r := d.Body.Radius
		body := lerpColor(droneColor, [4]float32{1, 1, 1, 1}, d.Flash)
		spin := mathx.AxisAngle(mathx.Vec3{0, 1, 0}, m.elapsed*2+float32(i))
		out = append(out, render.DrawCmd{Model: bodyMatrix(pos, spin, r), Color: body, Flags: gfx.DrawFlat, Mesh: m.sc.chip})
		// The eye tracks the player.
		look := eye.Sub(pos).Normalize()
		at := pos.Add(look.Scale(r * 0.78))
		out = append(out, render.DrawCmd{Model: bodyMatrix(at, mathx.QuatIdentity(), r*0.3), Color: droneEyeColor,
			Flags: gfx.DrawUnlit, Mesh: m.sc.ball})
		// A spinning rotor disc on top.
		rotor := mathx.Translate(pos[0], pos[1]+r*0.95, pos[2]).
			Mul(mathx.RotateY(m.elapsed * 25)).Mul(mathx.Scale(r*1.1, 1, r*1.1))
		out = append(out, render.DrawCmd{Model: rotor, Color: [4]float32{0.05, 0.05, 0.06, 0.8}, Flags: gfx.DrawUnlit,
			Mesh: m.sc.shadow})
	}
	return out
}

// appendDebris draws rubble as tumbling boxes (glass shards translucent) and
// grenades in flight; pieces shrink away at the end of their life.
func (m *Arena) appendDebris(out []render.DrawCmd) []render.DrawCmd {
	alpha := m.sim.Phys.Alpha()
	for _, d := range m.sim.Debris {
		pos, rot := d.Body.Interpolated(alpha)
		h := d.Half.Scale(clampf((d.Life-d.Age)/0.6, 0, 1))
		model := mathx.Translate(pos[0], pos[1], pos[2]).Mul(rot.Mat4()).Mul(mathx.Scale(h[0], h[1], h[2]))
		dc := render.DrawCmd{Model: model, Color: materialColor[d.Mat], Texture: m.as.materials[d.Mat], Mesh: m.as.cube}
		if d.Mat == arena.Scrap {
			dc.Flags = gfx.DrawFlat
		}
		if d.Mat == arena.Glass {
			m.glass = append(m.glass, dc)
			continue
		}
		out = append(out, dc)
	}
	for _, g := range m.sim.Grenades {
		if g.Age < 0.06 {
			continue // still leaving the barrel: drawn this close it would fill the view
		}
		pos, _ := g.Body.Interpolated(alpha)
		out = append(out,
			render.DrawCmd{Model: bodyMatrix(pos, mathx.QuatIdentity(), 0.07), Color: launcherGreen, Mesh: m.sc.ball},
			render.DrawCmd{Model: bodyMatrix(pos, mathx.QuatIdentity(), 0.035+0.015*float32(math.Sin(float64(m.elapsed)*40))),
				Color: droneEyeColor, Flags: gfx.DrawUnlit, Mesh: m.sc.ball})
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
	switch m.sim.Current {
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
	if m.flash > 0 && m.sim.Current == arena.WeaponRifle {
		size := 0.014 + 0.012*m.rng.Float32() // it's only ~40 cm from the eye
		spin := mathx.AxisAngle(mathx.Vec3{0, 0, 1}, m.rng.Float32()*math.Pi)
		at := model.TransformPoint(rifleMuzzle.Add(mathx.Vec3{0, 0, -0.03}))
		out = append(out, render.DrawCmd{Model: bodyMatrix(at, spin, size), Color: withAlpha(flashColor, 0.9),
			Flags: gfx.DrawUnlit, Mesh: m.sc.chip})
	}
	return out
}
