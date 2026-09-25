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
	arenaNear      = 0.05 // close enough that the gun model isn't clipped
	arenaFar       = 400
	arenaFogDens   = 0.011

	tracerSpeed  = 260 // m/s: how fast a tracer streak travels down the bullet's path
	tracerLength = 5   // m
	flashTime    = 0.04
	holeLife     = 10 // s
	maxHoles     = 80
	feedLife     = 1.6 // s a kill-feed line stays up
	hitMarkTime  = 0.12
)

// tracer is the visible streak of one bullet.
type tracer struct {
	from, to mathx.Vec3
	age      float32
}

// burst is a short-lived glowing sphere: impact sparks and explosions.
type burst struct {
	at     mathx.Vec3
	size   float32
	life   float32 // total
	age    float32
	grow   bool // expands (explosions) rather than shrinking (sparks)
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
}

// Arena is the FPS arena mode: first-person movement and a rifle against
// hovering drones. The simulation lives in package arena; this draws it,
// maps the player's input and adds effects and sounds.
type Arena struct {
	sc       *scenery
	sound    *audio.Mixer
	settings *Settings
	as       *arenaAssets
	sfx      arenaSounds
	rng      *rand.Rand

	sim *arena.Arena
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
	bob      float32 // walk-cycle phase for the gun sway
	prevPull bool    // trigger state last frame (for the pad's "pressed")
	elapsed  float32
}

func newArena(sc *scenery, mixer *audio.Mixer, settings *Settings, seed uint64) (*Arena, error) {
	m := &Arena{
		fixedSeed: seed,
		sc:        sc,
		sound:     mixer,
		settings:  settings,
		rng:       rand.New(rand.NewPCG(11, 13)),
		sfx: arenaSounds{
			shot:   audio.Blip(70*time.Millisecond, 1500, 170, 0.35),
			hit:    audio.Blip(35*time.Millisecond, 1900, 1700, 0.3),
			kill:   audio.Blip(420*time.Millisecond, 380, 35, 1),
			reload: audio.Blip(60*time.Millisecond, 900, 650, 0.35),
			empty:  audio.Blip(25*time.Millisecond, 2300, 2300, 0.2),
			jump:   audio.Blip(90*time.Millisecond, 300, 520, 0.25),
			land:   audio.Blip(70*time.Millisecond, 160, 80, 0.7),
		},
	}
	as, err := newArenaAssets(arena.Level())
	if err != nil {
		return nil, err
	}
	m.as = as
	m.start()
	return m, nil
}

// start begins a new match.
func (m *Arena) start() {
	seed := m.fixedSeed
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	m.sim = arena.New(seed)
	m.tracers, m.bursts, m.holes, m.feed = m.tracers[:0], m.bursts[:0], m.holes[:0], m.feed[:0]
	m.flash, m.hitMark, m.elapsed = 0, 0, 0
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
		ctl = m.input(in, dt)
	}
	ev := m.sim.Step(dt, ctl)
	m.effects(dt, ev)
}

// input maps keyboard, mouse and gamepad to the player's intent.
func (m *Arena) input(in *input.State, dt float32) arena.Input {
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
	return c
}

// effects turns the step's events into tracers, sparks, holes, sounds and HUD
// feedback, and ages the ones already running.
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

	// Gun sway follows walking speed on the ground.
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
			m.bursts = append(m.bursts, burst{at: s.To, size: 0.12, life: 0.08, colour: tracerColor})
			m.playAt(m.sfx.hit, s.To, 0.8)
		case s.Normal != (mathx.Vec3{}):
			m.bursts = append(m.bursts, burst{at: s.To.Add(s.Normal.Scale(0.03)), size: 0.07, life: 0.07, colour: flashColor})
			if len(m.holes) == maxHoles {
				m.holes = m.holes[1:]
			}
			m.holes = append(m.holes, hole{at: s.To.Add(s.Normal.Scale(0.004)), normal: s.Normal})
		}
	}
	for _, d := range ev.Kills {
		at := d.Body.Position
		m.bursts = append(m.bursts,
			burst{at: at, size: 1.3, life: 0.28, grow: true, colour: explosionColor},
			burst{at: at, size: 0.7, life: 0.16, grow: true, colour: tracerColor})
		m.feed = append(m.feed, feedLine{text: "+100  DRONE DOWN"})
		m.playAt(m.sfx.kill, at, 1)
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

// camWorld is the camera's world transform (camera looks down its -Z).
func (m *Arena) camWorld() mathx.Mat4 {
	p := &m.sim.Player
	eye := p.Eye(m.sim.Phys.Alpha())
	return mathx.Translate(eye[0], eye[1], eye[2]).
		Mul(mathx.RotateY(-p.Yaw)).
		Mul(mathx.RotateX(p.ViewPitch()))
}

func (m *Arena) view() mathx.Mat4 {
	p := &m.sim.Player
	eye := p.Eye(m.sim.Phys.Alpha())
	return mathx.LookAt(eye, eye.Add(camera.Direction(p.Yaw, p.ViewPitch())), mathx.Vec3{0, 1, 0})
}

// gunModel places the rifle in front of the camera: down and to the right,
// kicked back by recoil, swaying as you walk and dipping during a reload.
func (m *Arena) gunModel() mathx.Mat4 {
	w := &m.sim.Weapon
	offset := mathx.Vec3{0.16, -0.16, -0.4}
	offset[2] += 0.03 * w.Kick
	offset[0] += 0.012 * float32(math.Sin(float64(m.bob)))
	offset[1] += 0.01 * float32(math.Abs(math.Cos(float64(m.bob))))
	pitch := 0.07 * w.Kick
	var roll float32
	if w.Reloading > 0 {
		t := 1 - w.Reloading/arena.ReloadTime
		dip := float32(math.Sin(math.Pi * float64(t)))
		offset[1] -= 0.14 * dip
		pitch -= 0.35 * dip
		roll = 0.5 * dip
	}
	const size = 0.55 // the parts are modelled at real scale; drawn this close they need shrinking
	return m.camWorld().
		Mul(mathx.Translate(offset[0], offset[1], offset[2])).
		Mul(mathx.RotateX(pitch)).
		Mul(mathx.RotateZ(roll)).
		Mul(mathx.Scale(size, size, size))
}

// muzzle is the barrel tip in world space.
func (m *Arena) muzzle() mathx.Vec3 { return m.gunModel().TransformPoint(gunMuzzle) }

// Render returns the frame parameters and the draw list (appended to out[:0]).
func (m *Arena) Render(aspect float32, out []render.DrawCmd) (render.FrameParams, []render.DrawCmd) {
	eye := m.sim.Player.Eye(m.sim.Phys.Alpha())
	fov := m.settings.fovRadians() * 1.25 // a wider view suits first person
	proj := mathx.Perspective(min(fov, 1.9), aspect, arenaNear, arenaFar)
	params := render.FrameParams{
		ViewProj:     proj.Mul(m.view()),
		CameraPos:    eye,
		SunDirection: arenaSunDir,
		SunColor:     arenaSun.Scale(1.05),
		Ambient:      arenaShade.Scale(0.5),
		FogColor:     arenaHaze,
		FogDensity:   arenaFogDens,
		Clear:        [4]float32{arenaHaze[0], arenaHaze[1], arenaHaze[2], 1},
	}

	out = append(out[:0], render.DrawCmd{
		Model: mathx.Translate(eye[0], eye[1], eye[2]).Mul(mathx.Scale(skyRadius, skyRadius, skyRadius)),
		Color: arenaSkyZenith, Flags: gfx.DrawSky, Mesh: m.sc.sky,
	})
	out = append(out, m.as.blocks...)
	out = m.appendDrones(out, eye)
	out = m.appendEffects(out)
	return params, m.appendGun(out)
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
	for _, deb := range m.sim.Debris {
		pos, rot := deb.Body.Interpolated(alpha)
		out = append(out, render.DrawCmd{Model: bodyMatrix(pos, rot, deb.Body.Radius), Color: droneColor,
			Flags: gfx.DrawFlat, Mesh: m.sc.chip})
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
		out = append(out, render.DrawCmd{Model: bodyMatrix(b.at, mathx.QuatIdentity(), size),
			Color: withAlpha(b.colour, 1-t*t), Flags: gfx.DrawUnlit, Mesh: m.sc.chip})
	}
	return out
}

// appendGun draws the first-person rifle and its muzzle flash.
func (m *Arena) appendGun(out []render.DrawCmd) []render.DrawCmd {
	gun := m.gunModel()
	for _, part := range gunParts {
		c, h := part.centre, part.half
		model := gun.Mul(mathx.Translate(c[0], c[1], c[2])).Mul(mathx.Scale(h[0], h[1], h[2]))
		out = append(out, render.DrawCmd{Model: model, Color: part.color, Flags: part.flags, Mesh: m.as.cube})
	}
	if m.flash > 0 {
		size := 0.014 + 0.012*m.rng.Float32() // it's only ~40 cm from the eye
		spin := mathx.AxisAngle(mathx.Vec3{0, 0, 1}, m.rng.Float32()*math.Pi)
		at := gun.TransformPoint(gunMuzzle.Add(mathx.Vec3{0, 0, -0.03}))
		out = append(out, render.DrawCmd{Model: bodyMatrix(at, spin, size), Color: withAlpha(flashColor, 0.9),
			Flags: gfx.DrawUnlit, Mesh: m.sc.chip})
	}
	return out
}
