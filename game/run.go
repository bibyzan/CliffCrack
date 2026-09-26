package game

import (
	"math"
	"time"

	"CliffCrack/engine/audio"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/input"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
	"CliffCrack/engine/render"
	"CliffCrack/engine/ui"
	"CliffCrack/game/course"
)

const (
	overDelay     = 0.9 // seconds from a crash to the game-over card
	attractReplay = 3.0 // seconds the menu backdrop lingers on a crash before a new run
	hintTime      = 7.0 // seconds the controls hint stays up
)

// runChunk is a streamed piece of the course: its terrain mesh and obstacles.
type runChunk struct {
	mesh      render.Mesh
	obstacles []course.Obstacle
	draws     []render.DrawCmd // obstacle draws; they never move
}

// Run is the arcade mode: dropped off a cliff, ride the ball down an endless
// generated mountain as far as you can before you hit something.
//
// In attract mode it plays itself (on the autopilot, restarting after each
// crash) as the backdrop of the main menu.
type Run struct {
	sc      *scenery
	sound   *audio.Mixer
	sfx     runSounds
	attract bool
	// Autopilot steers the player's runs too (for demos and scripted tests).
	Autopilot bool
	fixedSeed uint64  // non-zero: every run uses this seed
	startAt   float32 // non-zero: the player's runs start this far down (a dev shortcut)

	seed   uint64
	course *course.Course
	ride   *ride
	chunks map[int]*runChunk
	cam    chaseCam

	best       float32
	newBest    bool
	overTime   float32 // seconds since the crash
	choice     int     // game-over card: 0 retry, 1 main menu
	wantsMenu  bool    // the player picked "Main menu"
	retry      bool    // "Ride again" was clicked; restart on the next update
	sun        float32 // debug: sun intensity
	zone       int     // current zone (zoneLength metres each), -1 before the start line
	zoneTime   float32 // seconds since entering it
	picked     string  // the last power-up collected, for its banner
	pickedKind course.PowerKind
	pickedAt   float32 // ride time it was collected

	debugOpen bool // the F1 window is up: the mouse is for the UI unless the right button is held
	locked    bool // the mouse is captured for looking around

	touchScreen bool      // the device has one: show on-screen controls while it's in use
	touch       *runTouch // the stick, jump and pause buttons
	touchOn     bool      // the on-screen controls are up this frame
	wantsPause  bool      // the on-screen pause button was tapped

	settings *Settings // the player's preferences (field of view, look sensitivity)
}

type runSounds struct {
	jump, land, crash, move, pick *audio.Sound
	boost, shield, smash          *audio.Sound
}

func newRun(sc *scenery, mixer *audio.Mixer, seed uint64, settings *Settings) *Run {
	return &Run{
		sc:        sc,
		sound:     mixer,
		settings:  settings,
		fixedSeed: seed,
		chunks:    map[int]*runChunk{},
		sun:       1,
		sfx:       newRunSounds(),
		touch:     newRunTouch(),
	}
}

// start begins a new run on a fresh course (or the fixed seed).
func (r *Run) start(attract bool) {
	r.attract = attract
	r.seed = r.fixedSeed
	if r.seed == 0 {
		r.seed = uint64(time.Now().UnixNano())
	}
	for index, c := range r.chunks {
		render.DestroyMesh(c.mesh) // deferred by the renderer: no stall
		delete(r.chunks, index)
	}
	r.course = course.New(r.seed)
	r.ride = newRide(r.course, r.obstacles)
	if r.startAt > 0 && !attract {
		r.ride.placeAt(r.startAt, 0, r.ride.cruise())
	}
	r.overTime, r.choice, r.newBest, r.wantsMenu, r.retry = 0, 0, false, false, false
	r.zone, r.zoneTime = -1, 0
	r.cam = newChaseCam(r.ride)
	r.stream(true)
}

// obstacles returns a chunk's obstacles, from the streamed chunk when loaded.
func (r *Run) obstacles(index int) []course.Obstacle {
	if c, ok := r.chunks[index]; ok {
		return c.obstacles
	}
	return r.course.Obstacles(index)
}

// stream keeps the chunks from just behind the ball to chunksAhead in front
// loaded. It builds at most one new chunk per frame (a few hundred
// microseconds) unless all is set, so streaming never causes a hitch.
func (r *Run) stream(all bool) {
	cur := course.ChunkAt(max(r.ride.s(), 0))
	for index, c := range r.chunks {
		if index < cur-2 || index > cur+r.settings.view().chunks {
			render.DestroyMesh(c.mesh)
			delete(r.chunks, index)
		}
	}
	for index := cur - 1; index <= cur+r.settings.view().chunks; index++ {
		if _, ok := r.chunks[index]; ok {
			continue
		}
		data := r.course.Chunk(index)
		mesh, err := render.CreateMesh(data.Mesh)
		if err != nil {
			logf("run: chunk %d: %v", index, err)
			continue
		}
		c := &runChunk{mesh: mesh, obstacles: data.Obstacles}
		r.chunks[index] = c
		r.drawObstacles(index)
		if !all {
			return
		}
	}
}

// drawObstacles (re)builds chunk index's obstacle draws, leaving out any
// smashed with the shield up.
func (r *Run) drawObstacles(index int) {
	c, ok := r.chunks[index]
	if !ok {
		return
	}
	c.draws = c.draws[:0]
	for i, o := range c.obstacles {
		if !r.ride.smashed[obstacleID{index, i}] {
			c.draws = r.sc.appendObstacle(c.draws, o)
		}
	}
}

// Update advances the run. mouseFree is false while the UI has the mouse.
// Esc is handled by the App.
func (r *Run) Update(dt float32, in *input.State, mouseFree bool) {
	if r.retry {
		r.play(r.sfx.pick, 1)
		r.start(false)
	}
	riding := !r.attract && (!r.ride.crashed || r.overTime < overDelay)
	r.touchOn = r.touchScreen && riding && !in.UsingPad()
	var (
		touchRide rideInput
		touchLook lookInput
	)
	if r.touchOn {
		w, h := render.DisplaySize()
		r.touch.floating = r.settings.FloatingStick
		touchRide, touchLook, r.wantsPause = r.touch.read(in, float32(w), float32(h))
	} else {
		r.touch.release()
	}
	var ctl rideInput
	switch {
	case r.attract || r.Autopilot:
		ctl = autopilot(r.ride)
	default:
		// Analog on a pad: the left stick steers, the triggers tuck and brake.
		ctl = rideInput{
			steer: r.settings.axis(in, ActLeft, ActRight) + in.Axis(input.KeyLeft, input.KeyRight) +
				in.PadAxis(input.PadLeftX) + padDirX(in),
			throttle: r.settings.axis(in, ActBack, ActForward) + in.Axis(input.KeyDown, input.KeyUp) +
				in.PadAxis(input.PadRightTrigger) - in.PadAxis(input.PadLeftTrigger),
			jump: r.settings.pressed(in, ActJump) || in.PadPressed(input.PadA),
		}
		ctl.steer += touchRide.steer
		ctl.throttle += touchRide.throttle
		ctl.jump = ctl.jump || touchRide.jump
	}
	wasCrashed := r.ride.crashed
	ev := r.ride.step(dt, ctl)
	r.stream(false)
	look := r.look(in, mouseFree, dt)
	scale := r.settings.LookSensitivity
	look.yaw += touchLook.yaw * scale
	look.elev += touchLook.elev * scale * r.settings.lookSign()
	r.cam.update(dt, r.ride, r.course, look)

	r.zoneTime += dt
	if s := r.ride.s(); s >= 0 && !r.ride.crashed {
		if zone := int(s / zoneLength); zone > r.zone {
			r.zone, r.zoneTime = zone, 0
			if !r.attract {
				r.play(r.sfx.pick, 0.7)
			}
		}
	}
	for _, index := range ev.smashed {
		r.drawObstacles(index)
		if !r.attract {
			r.play(r.sfx.smash, 1)
		}
	}
	for _, k := range ev.picked {
		r.pickedAt = r.ride.time
		r.picked, r.pickedKind = "BOOST", k
		sound := r.sfx.boost
		if k == course.Shield {
			r.picked, sound = "SHIELD", r.sfx.shield
		}
		if !r.attract {
			r.play(sound, 1)
		}
	}
	if !r.attract {
		switch {
		case ev.jumped:
			r.play(r.sfx.jump, 0.6)
		case ev.landed > 0:
			r.play(r.sfx.land, min(1, ev.landed/10))
		}
	}
	if !r.ride.crashed {
		return
	}
	if !wasCrashed {
		if !r.attract {
			r.play(r.sfx.crash, 1)
		}
		if r.ride.distance > r.best && !r.attract {
			r.best, r.newBest = r.ride.distance, true
		}
	}
	r.overTime += dt
	if r.attract {
		if r.overTime > attractReplay {
			r.start(true)
		}
		return
	}
	if r.overTime < overDelay {
		return
	}
	// Game-over card: up/down choose, Enter/A confirm, R/Y ride again, B menu.
	if navY(in) != 0 {
		r.choice = 1 - r.choice
		r.play(r.sfx.move, 1)
	}
	if in.Pressed(input.KeyR) || in.PadPressed(input.PadY) || (confirmPressed(in) && r.choice == 0) {
		r.play(r.sfx.pick, 1)
		r.start(false)
		return
	}
	if in.PadPressed(input.PadB) || (confirmPressed(in) && r.choice == 1) {
		r.play(r.sfx.pick, 1)
		r.wantsMenu = true
	}
}

// look reads this frame's camera input. While riding, the mouse is captured
// and turns the camera; with the debug window open it's free for the UI and
// the right button must be held to look. A drag (or a finger on a touch
// screen) outside the UI looks too, and so does the right stick.
func (r *Run) look(in *input.State, mouseFree bool, dt float32) lookInput {
	if r.attract {
		r.locked = false
		return lookInput{}
	}
	riding := !r.ride.crashed || r.overTime < overDelay
	held := in.MouseDown(input.MouseRight) && (mouseFree || r.locked)
	r.locked = riding && (!r.debugOpen || held) && !r.touchScreen

	var l lookInput
	if x, y := in.PadStick(true); x != 0 || y != 0 {
		l.yaw += x * camStickYaw * dt
		l.elev += y * camStickPitch * dt
	}
	// On a touch screen the first finger also acts as the mouse; the
	// on-screen controls sort out which fingers look.
	dragging := (in.MouseDown(input.MouseLeft) || in.MouseDown(input.MouseRight)) && mouseFree && !r.touchOn
	if r.locked || dragging {
		dx, dy := in.MouseDelta()
		l.yaw += float32(dx) * camMouseSens
		l.elev += float32(dy) * camMouseSens
	}
	scale := r.settings.LookSensitivity
	l.yaw *= scale
	l.elev *= scale * r.settings.lookSign()
	return l
}

// CursorLocked reports whether the mouse should be captured for looking around.
func (r *Run) CursorLocked() bool { return r.locked }

func (r *Run) play(s *audio.Sound, volume float32) {
	if r.sound != nil {
		r.sound.Play(s, volume, 0)
	}
}

// Render returns the frame parameters and the draw list (appended to out[:0]).
func (r *Run) Render(aspect float32, out []render.DrawCmd) (render.FrameParams, []render.DrawCmd) {
	view, eye := r.cam.view()
	v := r.settings.view()
	proj := mathx.Perspective(r.cam.fov(r.settings.fovRadians()), aspect, 0.3, v.far)
	params := runFrameParams(proj.Mul(view), eye, r.sun)
	params.FogDensity = v.fog

	out = r.sc.appendBackdrop(out[:0], eye, v)
	for _, c := range r.chunks {
		out = append(out, render.DrawCmd{Model: mathx.Identity(), Color: snowColor,
			Flags: gfx.DrawFlat | gfx.DrawSnow, Mesh: c.mesh})
		out = append(out, c.draws...)
	}
	out = r.appendPowerUps(out)
	return params, r.appendBall(out)
}

// Power-up colours: boost is fire, shield is ice.
var (
	boostColor  = mathx.SRGB(1.0, 0.55, 0.12, 1)
	shieldColor = mathx.SRGB(0.35, 0.85, 1.0, 1)
)

// appendPowerUps draws the pickups still on the course (spinning, bobbing
// and glowing, each in a faint halo), the shards of anything smashed, and
// what's active on the ball: a trail of fire for a boost, a bubble of ice
// for a shield.
func (r *Run) appendPowerUps(out []render.DrawCmd) []render.DrawCmd {
	t := r.ride.time
	for index := range r.chunks {
		for i, p := range r.ride.powerUps(index) {
			if r.ride.taken[powerID{index, i}] {
				continue
			}
			bob := 0.25 * float32(math.Sin(float64(t)*3+float64(p.Distance)))
			at := p.Pos.Add(mathx.Vec3{0, bob, 0})
			place := mathx.Translate(at[0], at[1], at[2])
			halo := boostColor
			if p.Kind == course.Shield {
				halo = shieldColor
				spin := mathx.AxisAngle(mathx.Vec3{0, 1, 0}, t*2.5)
				out = append(out, render.DrawCmd{Model: place.Mul(spin.Mat4()).Mul(mathx.Scale(0.6, 0.6, 0.6)),
					Color: shieldColor, Flags: gfx.DrawUnlit, Mesh: r.sc.orb})
			} else {
				// Pointing down the run (-Z), spinning about that axis.
				roll := mathx.AxisAngle(mathx.Vec3{0, 0, 1}, t*4)
				tip := mathx.AxisAngle(mathx.Vec3{1, 0, 0}, -math.Pi/2)
				m := place.Mul(roll.Mat4()).Mul(tip.Mat4()).Mul(mathx.Translate(0, -0.65, 0))
				out = append(out, render.DrawCmd{Model: m, Color: boostColor, Flags: gfx.DrawUnlit, Mesh: r.sc.arrow})
			}
			pulse := 1.1 + 0.12*float32(math.Sin(float64(t)*6))
			halo[3] = 0.22
			out = append(out, render.DrawCmd{Model: place.Mul(mathx.Scale(pulse, pulse, pulse)),
				Color: halo, Flags: gfx.DrawUnlit, Mesh: r.sc.orb})
		}
	}
	for _, s := range r.ride.shards {
		c := rockColor
		if s.kind == course.Tree {
			c = crownColor
		}
		pos, rot := r.ride.pose(s.body)
		size := s.body.Radius * min(1, (shardLife-s.age)/0.5)
		out = append(out, render.DrawCmd{Model: bodyMatrix(pos, rot, size), Color: c, Flags: gfx.DrawFlat, Mesh: r.sc.chip})
	}
	if r.ride.crashed {
		return out
	}
	p, _ := r.ride.pose(r.ride.ball)
	if r.ride.boost > 0 {
		back := r.ride.ball.Velocity.Normalize().Scale(-1)
		fade := min(1, r.ride.boost/0.5)
		for i := 1; i <= 6; i++ {
			k := float32(i)
			c := boostColor
			c[3] = 0.5 * fade * (1 - k/7)
			size := rideBallRadius * (1 - k*0.1)
			at := p.Add(back.Scale(k * 0.9))
			out = append(out, render.DrawCmd{Model: mathx.Translate(at[0], at[1], at[2]).Mul(mathx.Scale(size, size, size)),
				Color: c, Flags: gfx.DrawUnlit, Mesh: r.sc.orb})
		}
	}
	if sh := r.ride.shield; sh > 0 {
		c := shieldColor
		c[3] = 0.3
		if sh < 1.5 && int(sh*10)%2 == 0 {
			c[3] = 0.1 // flickering: about to go
		}
		size := float32(rideBallRadius * 1.9)
		spin := mathx.AxisAngle(mathx.Vec3{0, 1, 0}, t)
		out = append(out, render.DrawCmd{Model: mathx.Translate(p[0], p[1], p[2]).Mul(spin.Mat4()).Mul(mathx.Scale(size, size, size)),
			Color: c, Flags: gfx.DrawUnlit, Mesh: r.sc.orb})
	}
	return out
}

// appendBall draws the ball and its shadow, or its pieces after a crash.
func (r *Run) appendBall(out []render.DrawCmd) []render.DrawCmd {
	for _, sb := range r.ride.snowballs {
		pos, rot := r.ride.pose(sb.body)
		out = append(out, render.DrawCmd{Model: bodyMatrix(pos, rot, sb.body.Radius), Color: snowColor,
			Flags: gfx.DrawFlat, Mesh: r.sc.snow})
	}
	if r.ride.crashed {
		for i, p := range r.ride.debris {
			c := [4]float32{1, 1, 1, 1}
			if i%2 == 0 {
				c = mathx.Hex(0xe06a1b)
			}
			pos, rot := r.ride.pose(p)
			out = append(out, render.DrawCmd{Model: bodyMatrix(pos, rot, p.Radius), Color: c,
				Flags: gfx.DrawFlat, Mesh: r.sc.chip})
		}
		return out
	}
	p, rot := r.ride.pose(r.ride.ball)
	// A soft blob shadow on the snow: without shadows, jumps are hard to read.
	ground := r.course.Height(p[0], p[2])
	n := physics.TerrainNormal(r.course.Height, p[0], p[2], 0.5)
	height := max(p[1]-ground-rideBallRadius, 0)
	if alpha := 0.4 * (1 - height/15); alpha > 0 {
		size := rideBallRadius * (1.1 + height*0.06)
		at := mathx.Vec3{p[0], ground, p[2]}.Add(n.Scale(0.08))
		out = append(out, render.DrawCmd{
			Model: mathx.Translate(at[0], at[1], at[2]).Mul(alignUp(n).Mat4()).Mul(mathx.Scale(size, size, size)),
			Color: [4]float32{0.05, 0.07, 0.15, alpha},
			Flags: gfx.DrawUnlit,
			Mesh:  r.sc.shadow,
		})
	}
	return append(out, render.DrawCmd{Model: bodyMatrix(p, rot, rideBallRadius), Color: [4]float32{1, 1, 1, 1},
		Texture: r.sc.ballTex, Mesh: r.sc.ball})
}

// bodyMatrix places a unit mesh at a (drawn) body pose.
func bodyMatrix(p mathx.Vec3, rot mathx.Quat, radius float32) mathx.Mat4 {
	return mathx.Translate(p[0], p[1], p[2]).Mul(rot.Mat4()).Mul(mathx.Scale(radius, radius, radius))
}

// alignUp is the rotation taking +Y to n.
func alignUp(n mathx.Vec3) mathx.Quat {
	up := mathx.Vec3{0, 1, 0}
	axis := up.Cross(n)
	if l := axis.Len(); l > 1e-5 {
		return mathx.AxisAngle(axis.Scale(1/l), float32(math.Acos(float64(clampf(up.Dot(n), -1, 1)))))
	}
	return mathx.QuatIdentity()
}

// appendObstacle adds the draws for one rock or tree.
func (sc *scenery) appendObstacle(out []render.DrawCmd, o course.Obstacle) []render.DrawCmd {
	turn := mathx.AxisAngle(mathx.Vec3{0, 1, 0}, o.Yaw).Mat4()
	switch o.Kind {
	case course.Rock:
		c := o.Centre
		m := mathx.Translate(c[0], c[1], c[2]).Mul(turn).Mul(mathx.Scale(o.Scale, o.Scale*o.Squash, o.Scale))
		return append(out, render.DrawCmd{Model: m, Color: rockColor, Flags: gfx.DrawFlat | gfx.DrawSnow,
			Mesh: sc.rocks[o.Variant%len(sc.rocks)]})
	default:
		b := o.Base
		m := mathx.Translate(b[0], b[1]-0.2, b[2]).Mul(turn).Mul(mathx.Scale(o.Scale, o.Scale, o.Scale))
		return append(out,
			render.DrawCmd{Model: m, Color: trunkColor, Flags: gfx.DrawFlat, Mesh: sc.trunk},
			render.DrawCmd{Model: m, Color: crownColor, Flags: gfx.DrawFlat, Mesh: sc.crowns[o.Variant%len(sc.crowns)]})
	}
}

// DebugUI is the F1 window: numbers and tuning.
func (r *Run) DebugUI(b *ui.Builder, s Stats) {
	b.Window("Run", 12, 12)
	b.Text("%.0f fps  %.2f ms  %d draws", s.FPS, s.FrameMS, s.Draws)
	b.Text("seed %d", r.seed)
	b.Text("chunks %d, bodies %d", len(r.chunks), len(r.ride.phys.Bodies()))
	b.Text("s %.0f m  %.1f m/s  difficulty %.2f", r.ride.s(), r.ride.speed(), course.Difficulty(r.ride.s()))
	b.Slider("sun", &r.sun, 0, 3)
	b.Checkbox("autopilot", &r.Autopilot)
	if b.Button("new run") {
		r.retry = true
	}
	b.End()
}
