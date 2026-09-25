package game

import (
	"math"

	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/render"
	"CliffCrack/game/arena"
)

// Player suits by player index: blue for player 0 (you), red for player 1.
var (
	suitColor = [...][4]float32{mathx.SRGB(0.20, 0.42, 0.85, 1), mathx.SRGB(0.85, 0.24, 0.20, 1)}
	visorGlow = [...][4]float32{mathx.SRGB(0.35, 0.95, 1.00, 1), mathx.SRGB(1.00, 0.72, 0.25, 1)}
	armour    = mathx.SRGB(0.16, 0.17, 0.20, 1)
	skinTone  = mathx.SRGB(0.23, 0.24, 0.27, 1) // gloves and boots
)

// charPart is one box of a character, in the frame of the bone it hangs off.
type charPart struct {
	centre, half mathx.Vec3
	color        [4]float32 // zero: the suit colour
	flags        gfx.DrawFlags
}

var (
	charTorso = []charPart{
		{mathx.Vec3{0, 1.12, 0}, mathx.Vec3{0.24, 0.3, 0.15}, [4]float32{}, 0},     // body
		{mathx.Vec3{0, 1.2, 0}, mathx.Vec3{0.255, 0.18, 0.165}, armour, 0},         // vest
		{mathx.Vec3{0, 0.86, 0}, mathx.Vec3{0.25, 0.05, 0.16}, armour, 0},          // belt
		{mathx.Vec3{0, 1.24, 0.17}, mathx.Vec3{0.17, 0.14, 0.04}, [4]float32{}, 0}, // pack
		{mathx.Vec3{0.27, 1.36, 0}, mathx.Vec3{0.07, 0.07, 0.1}, [4]float32{}, 0},  // right shoulder
		{mathx.Vec3{-0.27, 1.36, 0}, mathx.Vec3{0.07, 0.07, 0.1}, [4]float32{}, 0}, // left shoulder
	}
	// Legs hang from the hip.
	charLeg = []charPart{
		{mathx.Vec3{0, -0.24, 0}, mathx.Vec3{0.1, 0.24, 0.11}, [4]float32{}, 0},  // thigh
		{mathx.Vec3{0, -0.62, 0}, mathx.Vec3{0.09, 0.18, 0.1}, armour, 0},        // shin
		{mathx.Vec3{0, -0.8, -0.04}, mathx.Vec3{0.095, 0.05, 0.14}, skinTone, 0}, // boot
	}
	// The head turns on the neck.
	charHead = []charPart{
		{mathx.Vec3{0, 0.16, 0}, mathx.Vec3{0.14, 0.16, 0.15}, armour, 0},                         // helmet
		{mathx.Vec3{0, 0.17, -0.14}, mathx.Vec3{0.115, 0.05, 0.025}, [4]float32{}, gfx.DrawUnlit}, // visor (visor colour)
		{mathx.Vec3{0, 0.31, 0.02}, mathx.Vec3{0.03, 0.03, 0.1}, [4]float32{}, 0},                 // crest
	}
	// Arms reach from the shoulders (in the aim frame) to the weapon.
	charArms = []charPart{
		{mathx.Vec3{0.2, -0.1, -0.16}, mathx.Vec3{0.06, 0.06, 0.2}, [4]float32{}, 0},
		{mathx.Vec3{0.17, -0.12, -0.36}, mathx.Vec3{0.05, 0.05, 0.05}, skinTone, 0},
		{mathx.Vec3{-0.04, -0.13, -0.3}, mathx.Vec3{0.055, 0.055, 0.22}, [4]float32{}, 0},
	}
)

const (
	hipHeight      = 0.85
	neckHeight     = 1.44
	shoulderHeight = 1.34
)

// appendCharacter draws a player's body: legs swinging with their stride,
// head and arms following their aim, the weapon in hand and the suit
// flashing when hit. The dead topple over backwards.
func (m *Arena) appendCharacter(out []render.DrawCmd, p *arena.Player, stride float32) []render.DrawCmd {
	alpha := m.sim().Phys.Alpha()
	pos, _ := p.Body.Interpolated(alpha)
	feet := pos.Sub(mathx.Vec3{0, arena.PlayerRadius, 0})
	base := mathx.Translate(feet[0], feet[1], feet[2]).Mul(mathx.RotateY(-p.Yaw))
	if p.Dead {
		t := clampf((m.sim().Time-p.DiedAt)/0.5, 0, 1)
		base = base.Mul(mathx.RotateX(smooth(t) * math.Pi / 2 * 0.97))
	}
	i := p.ID % len(suitColor)
	suit := lerpColor(suitColor[i], [4]float32{1, 1, 1, 1}, p.Flash*0.8)
	if p.Dead {
		suit = lerpColor(suit, armour, 0.5)
	}
	draw := func(frame mathx.Mat4, parts []charPart) {
		for _, c := range parts {
			col := c.color
			switch {
			case c.flags&gfx.DrawUnlit != 0:
				col = visorGlow[i]
			case col == [4]float32{}:
				col = suit
			}
			model := frame.Mul(mathx.Translate(c.centre[0], c.centre[1], c.centre[2])).Mul(mathx.Scale(c.half[0], c.half[1], c.half[2]))
			out = append(out, render.DrawCmd{Model: model, Color: col, Flags: c.flags | gfx.DrawFlat, Mesh: m.as.cube})
		}
	}

	// Legs: a walk cycle scaled by speed on the ground.
	v := p.Body.Velocity
	speed := float32(math.Hypot(float64(v[0]), float64(v[2])))
	swing := float32(0)
	if !p.Dead {
		swing = 0.55 * float32(math.Sin(float64(stride))) * clampf(speed/6, 0, 1)
		if !p.OnGround() {
			swing = 0.35 // legs tucked mid-jump
		}
	}
	for _, side := range []float32{1, -1} {
		hip := base.Mul(mathx.Translate(0.12*side, hipHeight, 0)).Mul(mathx.RotateX(swing * side))
		if !p.OnGround() && !p.Dead {
			hip = base.Mul(mathx.Translate(0.12*side, hipHeight, 0)).Mul(mathx.RotateX(-swing * (0.5 + 0.5*side)))
		}
		draw(hip, charLeg)
	}
	draw(base, charTorso)

	pitch := p.ViewPitch()
	draw(base.Mul(mathx.Translate(0, neckHeight, 0)).Mul(mathx.RotateX(pitch*0.6)), charHead)

	// Arms and weapon follow the aim.
	aim := base.Mul(mathx.Translate(0, shoulderHeight, 0)).Mul(mathx.RotateX(pitch))
	draw(aim, charArms)
	weapon := aim.Mul(mathx.Translate(0.16, -0.12, -0.42))
	parts := rifleParts
	switch p.Current {
	case arena.WeaponLauncher:
		parts = launcherParts
		weapon = weapon.Mul(mathx.Translate(0, 0, 0.1))
	case arena.WeaponHammer:
		parts = hammerParts
		s := hammerPose(p.Hammer.Progress())
		weapon = aim.Mul(mathx.Translate(0.2, -0.2, -0.3)).Mul(mathx.RotateX(-0.2 - 1.3*s))
	}
	if p.Switching > 0 {
		weapon = weapon.Mul(mathx.RotateX(-1.2 * p.Switching / arena.SwitchTime))
	}
	for _, part := range parts {
		c, h := part.centre, part.half
		pm := weapon.Mul(mathx.Translate(c[0], c[1], c[2])).Mul(mathx.Scale(h[0], h[1], h[2]))
		out = append(out, render.DrawCmd{Model: pm, Color: part.color, Flags: part.flags, Mesh: m.as.cube})
	}
	return out
}

// weaponMuzzle is roughly where another player's shots leave their weapon.
func weaponMuzzle(p *arena.Player) mathx.Vec3 {
	fwd := p.Forward()
	right, _ := flatRight(p.Yaw)
	return p.Eye(1).Add(fwd.Scale(0.85)).Add(right.Scale(0.16)).Add(mathx.Vec3{0, -0.2, 0})
}

// flatRight is the horizontal right vector for a yaw, and forward.
func flatRight(yaw float32) (right, forward mathx.Vec3) {
	s, c := float32(math.Sin(float64(yaw))), float32(math.Cos(float64(yaw)))
	forward = mathx.Vec3{s, 0, -c}
	return mathx.Vec3{c, 0, s}, forward
}

// hammerPose is how far a sledgehammer swing has got as a pose: 0 at rest,
// -0.4 wound up, 1 at the moment of impact; p is the swing's progress (0..1,
// negative when idle).
func hammerPose(p float32) float32 {
	if p < 0 {
		return 0
	}
	strike := float32(arena.HammerHitAt / arena.HammerSwing)
	switch {
	case p < strike*0.45:
		return -0.4 * smooth(p/(strike*0.45))
	case p < strike:
		return -0.4 + 1.4*smooth((p-strike*0.45)/(strike*0.55))
	default:
		return 1 - smooth((p-strike)/(1-strike))
	}
}
