package game

import (
	"math"

	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/render"
	"CliffCrack/game/arena"
)

// heldKind is what's in the hand to draw: the hammer mid-swing, else the
// weapon in hand.
func heldKind(w *arena.Weapons) arena.WeaponKind {
	if w.Swinging() {
		return arena.WeaponHammer
	}
	return w.Current
}

// Grenade looks.
var (
	fragOlive  = mathx.SRGB(0.30, 0.36, 0.20, 1)
	fragSteel  = mathx.SRGB(0.62, 0.64, 0.66, 1)
	stickyCore = mathx.SRGB(1.00, 1.00, 1.00, 1)
	pickupGlow = mathx.SRGB(1.00, 0.86, 0.45, 1) // the ring under a weapon waiting to be taken
)

// appendGrenade draws a grenade: a frag's olive body, spoon and pin; a
// sticky's glowing ball of paint, pulsing faster as it's about to go; a
// launcher round with its blinking light.
func (m *Arena) appendGrenade(out []render.DrawCmd, g *arena.Grenade, alpha float32) []render.DrawCmd {
	pos, rot := g.Body.Interpolated(alpha)
	if g.Stuck {
		pos, rot = g.Position(), mathx.QuatIdentity()
	}
	frame := mathx.Translate(pos[0], pos[1], pos[2]).Mul(rot.Mat4())
	part := func(c, h mathx.Vec3, col [4]float32, flags gfx.DrawFlags, mesh render.Mesh) {
		out = append(out, render.DrawCmd{Model: frame.Mul(mathx.Translate(c[0], c[1], c[2])).Mul(mathx.Scale(h[0], h[1], h[2])),
			Color: col, Flags: flags, Mesh: mesh})
	}
	switch g.Kind {
	case arena.Frag:
		part(mathx.Vec3{}, mathx.Vec3{0.055, 0.065, 0.055}, fragOlive, gfx.DrawFlat, m.as.gem)      // body
		part(mathx.Vec3{}, mathx.Vec3{0.058, 0.008, 0.058}, gunBlack, gfx.DrawFlat, m.as.gem)       // band
		part(mathx.Vec3{0, 0.07, 0}, mathx.Vec3{0.02, 0.018, 0.02}, fragSteel, 0, m.as.cube)        // fuse head
		part(mathx.Vec3{0.028, 0.045, 0}, mathx.Vec3{0.006, 0.045, 0.012}, fragSteel, 0, m.as.cube) // spoon
		part(mathx.Vec3{-0.024, 0.085, 0}, mathx.Vec3{0.012, 0.012, 0.003}, fragSteel, 0, m.as.gem) // pin ring
	case arena.Sticky:
		rate := 6.0
		if g.Stuck {
			rate = 10 + 30*float64(clampf(1-g.Fuse()/1.6, 0, 1))
		}
		pulse := 0.5 + 0.5*float32(math.Sin(float64(m.elapsed)*rate))
		col := paintColor[team(g.Owner)]
		part(mathx.Vec3{}, mathx.Vec3{0.07, 0.07, 0.07}, withAlpha(col, 0.6), gfx.DrawUnlit, m.as.gem)
		part(mathx.Vec3{}, mathx.Vec3{0.035, 0.035, 0.035}.Scale(0.8+0.5*pulse), stickyCore, gfx.DrawUnlit, m.as.gem)
		if g.Stuck {
			out = append(out, render.DrawCmd{Model: bodyMatrix(pos, mathx.QuatIdentity(), 0.18+0.1*pulse),
				Color: withAlpha(col, 0.25*pulse), Flags: gfx.DrawUnlit, Mesh: m.sc.chip})
		}
	default:
		out = append(out,
			render.DrawCmd{Model: bodyMatrix(pos, mathx.QuatIdentity(), 0.07), Color: hopperYellow, Mesh: m.as.gem},
			render.DrawCmd{Model: bodyMatrix(pos, mathx.QuatIdentity(), 0.035+0.015*float32(math.Sin(float64(m.elapsed)*40))),
				Color: grenadeGlow, Flags: gfx.DrawUnlit, Mesh: m.as.gem})
	}
	return out
}

// appendPickups draws what's lying about: weapons hovering and turning over
// a glowing ring where they spawn (a column of light over the power ones),
// lying flat on the range's table or where they were dropped, and crates of
// grenades.
func (m *Arena) appendPickups(out []render.DrawCmd) []render.DrawCmd {
	s := m.sim()
	t := float64(m.elapsed)
	for i, p := range s.Pickups {
		at := p.At
		var frame mathx.Mat4
		if !p.Table && s.SpawnedHere(p) {
			hover := 0.45 + 0.06*float32(math.Sin(t*2+float64(i)))
			frame = mathx.Translate(at[0], at[1]+hover, at[2]).Mul(mathx.RotateY(p.Yaw + float32(t)*0.8))
			disc := func(y, r, a float32) {
				model := mathx.Translate(at[0], at[1]+y, at[2]).Mul(mathx.Scale(r, 1, r))
				out = append(out, render.DrawCmd{Model: model, Color: withAlpha(pickupGlow, a), Flags: gfx.DrawUnlit, Mesh: m.sc.shadow})
			}
			disc(0.015, 0.55, 0.25)
			disc(0.02, 0.35, 0.35+0.15*float32(math.Sin(t*3+float64(i))))
			if p.Weapon == arena.WeaponSniper || p.Weapon == arena.WeaponLauncher {
				beam := mathx.Translate(at[0], at[1]+1.6, at[2]).Mul(mathx.Scale(0.05, 1.6, 0.05))
				out = append(out, render.DrawCmd{Model: beam, Color: withAlpha(pickupGlow, 0.18), Flags: gfx.DrawUnlit, Mesh: m.as.cube})
			}
		} else {
			frame = mathx.Translate(at[0], at[1]+0.05, at[2]).Mul(mathx.RotateY(p.Yaw)).Mul(mathx.RotateZ(math.Pi / 2)) // on its side
		}
		if p.Weapon == arena.NoWeapon {
			out = m.appendCrate(out, p, mathx.Translate(at[0], at[1], at[2]).Mul(mathx.RotateY(p.Yaw)))
			continue
		}
		out = m.drawParts(out, frame, partsFor(p.Weapon))
	}
	return out
}

var wholeGuns [len(markers)][]gunPart

// partsFor is weapon k's model.
func partsFor(k arena.WeaponKind) []gunPart {
	switch {
	case k == arena.WeaponHammer:
		return hammerParts
	case int(k) >= 0 && int(k) < len(markers):
		if wholeGuns[k] == nil { // all of it, at rest (joined once, so it bakes once)
			mk := &markers[k]
			wholeGuns[k] = join(mk.parts, mk.mag, mk.pump)
		}
		return wholeGuns[k]
	}
	return nil
}

// appendCrate draws a crate of grenades: a small case with a couple on top.
func (m *Arena) appendCrate(out []render.DrawCmd, p *arena.Pickup, frame mathx.Mat4) []render.DrawCmd {
	trim := fragOlive
	if p.Grenade == arena.Sticky {
		trim = paintColor[0]
	}
	part := func(c, h mathx.Vec3, col [4]float32, flags gfx.DrawFlags, mesh render.Mesh) {
		out = append(out, render.DrawCmd{Model: frame.Mul(mathx.Translate(c[0], c[1], c[2])).Mul(mathx.Scale(h[0], h[1], h[2])),
			Color: col, Flags: flags, Mesh: mesh})
	}
	part(mathx.Vec3{0, 0.09, 0}, mathx.Vec3{0.22, 0.09, 0.14}, gunMetal, 0, m.as.cube)
	part(mathx.Vec3{0, 0.12, 0.141}, mathx.Vec3{0.2, 0.02, 0.003}, trim, gfx.DrawUnlit, m.as.cube)
	part(mathx.Vec3{0, 0.12, -0.141}, mathx.Vec3{0.2, 0.02, 0.003}, trim, gfx.DrawUnlit, m.as.cube)
	for _, x := range []float32{-0.09, 0.09} {
		if p.Grenade == arena.Frag {
			part(mathx.Vec3{x, 0.24, 0}, mathx.Vec3{0.055, 0.065, 0.055}, fragOlive, gfx.DrawFlat, m.as.gem)
		} else {
			part(mathx.Vec3{x, 0.24, 0}, mathx.Vec3{0.065, 0.065, 0.065}, withAlpha(paintColor[0], 0.8), gfx.DrawUnlit, m.as.gem)
		}
	}
	return out
}
