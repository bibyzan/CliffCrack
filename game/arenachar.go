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

// charPart is one piece of a character, in the frame of the bone it hangs
// off. A zero colour is the suit's (lit) or the visor's (unlit).
type charPart = gunPart

var suitTint = [4]float32{} // (a zero colour: the suit, or the visor)

// A character: rounded armour over a suit, all facets. Built from the
// same parts as the guns: b (a chamfered box) and r (a gem).
var (
	charTorso = []charPart{
		b(0, 1.1, 0, 0.21, 0.3, 0.14, suitTint),      // body
		b(0, 1.22, -0.01, 0.235, 0.17, 0.16, armour), // chest plate
		r(0, 1.3, -0.14, 0.14, 0.09, 0.04, armour),   // chest bulge
		b(0, 0.86, 0, 0.225, 0.055, 0.15, armour),    // belt
		r(0.12, 0.86, -0.15, 0.035, 0.035, 0.02, gunMetal),
		r(-0.12, 0.86, -0.15, 0.035, 0.035, 0.02, gunMetal),
		b(0, 1.2, 0.17, 0.16, 0.15, 0.05, suitTint),   // pack
		r(0, 1.06, 0.2, 0.09, 0.06, 0.04, gunMetal),   // pack canister
		r(0.27, 1.37, 0, 0.1, 0.085, 0.11, suitTint),  // right shoulder dome
		r(-0.27, 1.37, 0, 0.1, 0.085, 0.11, suitTint), // left shoulder dome
		r(0, 1.43, 0, 0.08, 0.05, 0.08, gunBlack),     // neck
	}
	// Legs hang from the hip.
	charLeg = []charPart{
		b(0, -0.22, 0, 0.095, 0.23, 0.1, suitTint),     // thigh
		r(0, -0.45, -0.05, 0.075, 0.07, 0.06, armour),  // knee pad
		b(0, -0.63, 0.005, 0.08, 0.17, 0.09, armour),   // shin
		b(0, -0.81, -0.04, 0.09, 0.05, 0.14, skinTone), // boot
		r(0, -0.78, -0.13, 0.07, 0.05, 0.06, skinTone), // toe
	}
	// The head turns on the neck.
	charHead = []charPart{
		r(0, 0.16, 0.01, 0.15, 0.165, 0.16, armour),           // helmet
		glow(r(0, 0.15, -0.075, 0.125, 0.065, 0.1, suitTint)), // visor, wrapping round (visor colour)
		b(0, 0.31, 0.03, 0.028, 0.035, 0.12, suitTint),        // crest
		r(0.14, 0.13, 0.02, 0.035, 0.06, 0.06, gunMetal),      // ear pieces
		r(-0.14, 0.13, 0.02, 0.035, 0.06, 0.06, gunMetal),
	}
	// Arms reach from the shoulders (in the aim frame) to the weapon.
	charArms = []charPart{
		b(0.2, -0.1, -0.12, 0.055, 0.055, 0.16, suitTint), // right upper arm
		r(0.19, -0.11, -0.28, 0.06, 0.055, 0.05, armour),  // right elbow pad
		b(0.17, -0.12, -0.36, 0.045, 0.045, 0.06, armour), // right forearm
		r(0.16, -0.12, -0.43, 0.045, 0.045, 0.045, skinTone),
		b(-0.04, -0.13, -0.26, 0.05, 0.05, 0.2, suitTint), // left arm
		r(-0.03, -0.13, -0.46, 0.045, 0.045, 0.045, skinTone),
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
	alpha := m.alpha()
	if p.Dead {
		alpha = 1 // (see eye)
	}
	pos, _ := p.Body.Interpolated(alpha)
	feet := pos.Sub(mathx.Vec3{0, arena.PlayerRadius, 0})
	// Running: the body bounces with each step and leans into a sprint.
	run, sprint := runFactors(p)
	feet[1] += 0.05 * run * (1 + 0.5*sprint) * float32(math.Abs(math.Sin(float64(stride))))
	base := mathx.Translate(feet[0], feet[1], feet[2]).Mul(mathx.RotateY(-p.Yaw)).Mul(mathx.RotateX(-0.18 * sprint))
	if p.Dead {
		t := clampf((m.sim().Time-p.DiedAt)/0.5, 0, 1)
		base = base.Mul(mathx.RotateX(smooth(t) * math.Pi / 2 * 0.97))
	}
	i := team(p)
	// Paint builds up on the suit as the armour goes, in the shooter's colour.
	cover := 1 - p.Shield/arena.MaxShield
	suit := lerpColor(suitColor[i], paintColor[1-i], 0.6*cover)
	suit = lerpColor(suit, [4]float32{1, 1, 1, 1}, p.Flash*0.8)
	if p.Dead {
		suit = lerpColor(suit, armour, 0.5)
	}
	draw := func(frame mathx.Mat4, parts []charPart) {
		out = drawBaked(out, frame, bake(parts), suit, visorGlow[i])
	}

	// Legs: a walk cycle scaled by speed on the ground.
	v := p.Body.Velocity
	speed := float32(math.Hypot(float64(v[0]), float64(v[2])))
	swing := float32(0)
	if !p.Dead {
		swing = (0.6 + 0.3*clampf((speed-arena.WalkSpeed)/(arena.SprintSpeed-arena.WalkSpeed), 0, 1)) *
			float32(math.Sin(float64(stride))) * clampf(speed/arena.WalkSpeed, 0, 1)
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
	parts := partsFor(heldKind(&p.Weapons))
	switch heldKind(&p.Weapons) {
	case arena.WeaponLauncher:
		weapon = weapon.Mul(mathx.Translate(0, 0, 0.1))
	case arena.WeaponHammer:
		parts = hammerParts
		s := hammerPose(p.Hammer.Progress())
		weapon = aim.Mul(mathx.Translate(0.2, -0.2, -0.3)).Mul(mathx.RotateX(-0.2 - 1.3*s))
	}
	if p.Switching > 0 {
		weapon = weapon.Mul(mathx.RotateX(-1.2 * p.Switching / arena.SwitchTime))
	}
	if e := elbowPose(p.Elbow.Progress()); e > 0 {
		weapon = weapon.Mul(mathx.Translate(-0.2*e, 0, -0.1*e)).Mul(mathx.RotateY(0.9 * e))
	}
	out = m.drawParts(out, weapon, parts)
	// A scoped sniper's lens catches the light: a glint you can spot them by.
	if mk, ok := m.markerFor(p.Current); ok && mk.scope && p.ADS > 0.5 && !p.Dead {
		lens := weapon.TransformPoint(mathx.Vec3{0, 0.085, -0.17})
		flicker := 0.7 + 0.3*float32(math.Sin(float64(m.elapsed)*9))
		m.glass = append(m.glass, render.DrawCmd{Model: bodyMatrix(lens, mathx.QuatIdentity(), 0.12*flicker),
			Color: withAlpha(uiWhite, 0.8), Flags: gfx.DrawUnlit, Mesh: m.sc.chip})
	}

	// Armour: a faint shell round them that flares when hit, gone once
	// it's popped. It's translucent, so it's drawn with the glass.
	if !p.Dead && p.Shield > 0 {
		glow := 0.05 + 0.4*p.Flash
		shell := base.Mul(mathx.Translate(0, 0.98, 0)).Mul(mathx.Scale(0.36, 0.98, 0.28))
		m.glass = append(m.glass, render.DrawCmd{Model: shell, Color: withAlpha(teamGlow[i], glow), Flags: gfx.DrawUnlit, Mesh: m.as.gem})
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

// elbowPose is how far an elbow strike has got as a pose: 0 at rest, 1 at
// the moment it lands; p is its progress (0..1, negative when idle).
func elbowPose(p float32) float32 {
	if p < 0 {
		return 0
	}
	strike := float32(arena.ElbowHitAt / arena.ElbowTime)
	if p < strike {
		return smooth(p / strike)
	}
	return 1 - smooth((p-strike)/(1-strike))
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

// teamGlow is each player's colour as a glowing trim, by player index.
var teamGlow = [...][4]float32{mathx.SRGB(0.30, 0.66, 1.00, 1), mathx.SRGB(1.00, 0.32, 0.24, 1)}
