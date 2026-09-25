package game

import (
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/game/arena"
)

// The markers' models, in weapon space: metres, barrel down -Z, +Y up, the
// hand at the grip. Each is built from boxes, balls (hoppers, tanks, bells)
// and rings (sight reticles), with lit plastics and glowing sights: a holo
// sight on the rifle, tritium irons on the pistol, a ghost ring and a fibre
// bead on the shotgun, a scope on the sniper, a ladder on the launcher.
var (
	sightRed   = mathx.SRGB(1.00, 0.16, 0.10, 1)
	tritium    = mathx.SRGB(0.35, 1.00, 0.35, 1)
	fiberRed   = mathx.SRGB(1.00, 0.20, 0.25, 1)
	tealDark   = mathx.SRGB(0.06, 0.40, 0.40, 1)
	rubber     = mathx.SRGB(0.10, 0.10, 0.11, 1)
	paintBalls = mathx.SRGB(0.10, 0.78, 1.00, 1)
)

// b, r and o are shorthands for a box, a ball and a ring part.
func b(cx, cy, cz, hx, hy, hz float32, col [4]float32) gunPart {
	return gunPart{centre: mathx.Vec3{cx, cy, cz}, half: mathx.Vec3{hx, hy, hz}, color: col}
}

func r(cx, cy, cz, hx, hy, hz float32, col [4]float32) gunPart {
	p := b(cx, cy, cz, hx, hy, hz, col)
	p.round = true
	return p
}

// o is a ring of radius rad in the XY plane (facing the eye).
func o(cx, cy, cz, rad float32, col [4]float32) gunPart {
	p := b(cx, cy, cz, rad, rad, 1, col)
	p.ring = true
	return p
}

// glow makes a part unlit: it shines.
func glow(p gunPart) gunPart {
	p.flags |= gfx.DrawUnlit
	return p
}

// row repeats a part n times, stepping by d.
func row(p gunPart, n int, d mathx.Vec3) []gunPart {
	out := make([]gunPart, n)
	for i := range out {
		out[i] = p
		out[i].centre = p.centre.Add(d.Scale(float32(i)))
	}
	return out
}

func join(groups ...[]gunPart) []gunPart {
	var out []gunPart
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// hopper is a side-fed paint hopper at x: its shell, lid, window of paint
// and the neck feeding in.
func hopper(x, y, z, size float32) []gunPart {
	return []gunPart{
		r(x, y, z, size, size*1.05, size*1.25, hopperYellow),
		b(x, y+size, z, size*0.6, size*0.12, size*0.7, rubber),                                    // lid
		b(x-size*0.3, y+size*1.12, z, size*0.3, size*0.06, size*0.2, gunMetal),                    // lid latch
		glow(r(x+size*0.35, y-size*0.1, z-size*0.3, size*0.25, size*0.25, size*0.25, paintBalls)), // paint showing through
		glow(r(x+size*0.4, y+size*0.2, z+size*0.25, size*0.22, size*0.22, size*0.22, paintBalls)),
		b(x/2, y, z, -x/2-0.02, 0.011, 0.011, rubber),  // feed neck
		r(x*0.25, y, z, 0.016, 0.016, 0.016, gunMetal), // elbow
	}
}

// grip is a pistol grip under y at z, with a trigger and guard ahead of it.
func grip(y, z float32) []gunPart {
	return []gunPart{
		b(0, y-0.045, z, 0.017, 0.045, 0.022, rubber),
		b(0, y-0.045, z-0.02, 0.0175, 0.04, 0.003, gunBlack), // finger grooves
		b(0, y-0.094, z, 0.019, 0.005, 0.025, gunMetal),      // cap
		b(0, y-0.028, z-0.058, 0.003, 0.004, 0.03, gunBlack), // guard
		b(0, y-0.014, z-0.088, 0.003, 0.014, 0.003, gunBlack),
		b(0, y-0.012, z-0.045, 0.003, 0.011, 0.003, gunMetal), // trigger
	}
}

var markers = [...]marker{
	arena.WeaponRifle: {
		parts: join(
			[]gunPart{
				b(0, 0.02, 0, 0.028, 0.03, 0.15, markerTeal),             // upper receiver
				b(0, -0.018, 0.02, 0.026, 0.017, 0.12, gunBlack),         // lower
				b(0.0285, 0.025, -0.02, 0.0006, 0.007, 0.08, markerLime), // stripe
				b(0.028, 0.032, 0.07, 0.005, 0.005, 0.014, gunMetal),     // charging handle
				b(0, 0.012, -0.19, 0.031, 0.031, 0.075, tealDark),        // handguard
				b(0, 0.018, -0.34, 0.011, 0.011, 0.17, gunBlack),         // barrel
				b(0, 0.018, -0.515, 0.016, 0.016, 0.022, gunMetal),       // muzzle brake
				b(0, 0.018, -0.515, 0.017, 0.004, 0.012, gunBlack),       // ... its port
				b(0, 0.052, -0.03, 0.012, 0.004, 0.15, gunBlack),         // top rail
				b(0, -0.065, -0.17, 0.013, 0.04, 0.016, rubber),          // fore grip
				b(0, -0.104, -0.17, 0.015, 0.004, 0.018, gunMetal),
				// The stock: a gas tank on a rod, and a butt pad.
				b(0, 0.0, 0.2, 0.008, 0.008, 0.07, gunBlack),
				r(0, -0.05, 0.19, 0.022, 0.022, 0.075, gunMetal),
				b(0, -0.05, 0.108, 0.013, 0.013, 0.012, gasSteel),          // regulator
				glow(b(0.016, -0.04, 0.108, 0.002, 0.004, 0.004, tritium)), // gauge
				b(0, -0.018, 0.275, 0.018, 0.038, 0.008, tealDark),         // butt plate
				// The holo sight, up on a tall riser so the body sits well below
				// the line of sight: a hooded box with a window, and a red ring
				// and dot floating in it.
				b(0, 0.064, 0.03, 0.014, 0.01, 0.03, gunBlack),   // riser
				b(0, 0.064, 0.03, 0.015, 0.004, 0.02, gunMetal),  // riser clamp
				b(0, 0.082, 0.03, 0.017, 0.006, 0.034, gunBlack), // base
				b(-0.018, 0.107, 0.03, 0.003, 0.02, 0.034, gunBlack),
				b(0.018, 0.107, 0.03, 0.003, 0.02, 0.034, gunBlack),
				b(0, 0.129, 0.03, 0.021, 0.003, 0.034, gunBlack),     // hood
				b(0.024, 0.097, 0.045, 0.004, 0.008, 0.01, gunMetal), // brightness knob
				glow(b(0.024, 0.097, 0.056, 0.002, 0.002, 0.001, sightRed)),
				glow(b(0, 0.107, -0.002, 0.015, 0.019, 0.0004, withAlpha(lensBlue, 0.14))), // window
				glow(o(0, 0.107, 0.0, 0.0068, sightRed)),
				glow(b(0, 0.107, 0.0, 0.0009, 0.0009, 0.0004, sightRed)),
			},
			row(b(0, 0.057, -0.16, 0.013, 0.0015, 0.004, gunMetal), 9, mathx.Vec3{0, 0, 0.032}),      // rail notches
			row(b(0.032, 0.012, -0.235, 0.0008, 0.006, 0.014, gunBlack), 3, mathx.Vec3{0, 0, 0.034}), // handguard slots
			row(b(-0.032, 0.012, -0.235, 0.0008, 0.006, 0.014, gunBlack), 3, mathx.Vec3{0, 0, 0.034}),
			row(r(0, 0.018, -0.41, 0.0135, 0.0135, 0.004, gunMetal), 3, mathx.Vec3{0, 0, -0.03}), // barrel rings
			hopper(-0.105, -0.01, -0.01, 0.046),
			grip(-0.035, 0.09),
		),
		muzzle: mathx.Vec3{0, 0.018, -0.54}, sight: mathx.Vec3{0, 0.107, 0.07}, size: 0.55, relief: 0.22,
	},
	arena.WeaponPistol: {
		parts: join(
			[]gunPart{
				b(0, 0.03, -0.03, 0.02, 0.022, 0.1, markerOrange),          // slide
				b(0, 0.052, -0.03, 0.012, 0.0015, 0.09, gunBlack),          // slide top
				b(0.0205, 0.035, -0.07, 0.0006, 0.006, 0.02, gunBlack),     // ejection port
				b(0, 0.0, -0.02, 0.019, 0.012, 0.085, gunBlack),            // frame
				b(0, -0.015, -0.09, 0.01, 0.004, 0.035, gunBlack),          // under rail
				b(0, 0.03, -0.14, 0.009, 0.009, 0.012, gunMetal),           // barrel tip
				r(0, 0.03, -0.152, 0.0095, 0.0095, 0.003, gunBlack),        // muzzle
				b(0, 0.042, 0.075, 0.005, 0.008, 0.006, gunBlack),          // hammer
				b(0, -0.112, 0.04, 0.019, 0.006, 0.026, gunMetal),          // tube base
				b(0.018, -0.055, 0.04, 0.0012, 0.042, 0.018, markerOrange), // grip panels
				b(-0.018, -0.055, 0.04, 0.0012, 0.042, 0.018, markerOrange),
				// Three-dot tritium irons: two on the rear notch, one on the front post.
				b(-0.011, 0.058, 0.06, 0.006, 0.006, 0.005, gunBlack),
				b(0.011, 0.058, 0.06, 0.006, 0.006, 0.005, gunBlack),
				glow(b(-0.011, 0.058, 0.0655, 0.0024, 0.0024, 0.0004, tritium)),
				glow(b(0.011, 0.058, 0.0655, 0.0024, 0.0024, 0.0004, tritium)),
				b(0, 0.057, -0.11, 0.003, 0.006, 0.004, gunBlack),
				glow(b(0, 0.058, -0.1055, 0.0024, 0.0024, 0.0004, tritium)),
			},
			row(b(0.0205, 0.03, 0.045, 0.0006, 0.016, 0.0015, gunBlack), 4, mathx.Vec3{0, 0, 0.007}), // serrations
			row(b(-0.0205, 0.03, 0.045, 0.0006, 0.016, 0.0015, gunBlack), 4, mathx.Vec3{0, 0, 0.007}),
			[]gunPart{
				b(0, -0.055, 0.04, 0.017, 0.055, 0.024, rubber), // grip
				b(0, -0.02, -0.035, 0.003, 0.005, 0.025, gunBlack), b(0, -0.018, -0.028, 0.003, 0.01, 0.003, gunMetal),
			},
		),
		muzzle: mathx.Vec3{0, 0.03, -0.16}, sight: mathx.Vec3{0, 0.058, 0.09}, size: 0.6, relief: 0.3,
	},
	arena.WeaponShotgun: {
		parts: join(
			[]gunPart{
				b(0, 0, 0.02, 0.032, 0.04, 0.13, markerPurple),      // receiver
				b(0.0325, 0.01, 0.0, 0.0006, 0.012, 0.06, gunBlack), // ejection port
				b(0, 0.02, -0.3, 0.018, 0.018, 0.18, gunBlack),      // barrel
				b(0, 0.041, -0.3, 0.006, 0.003, 0.18, gunMetal),     // vent rib
				b(0, 0.02, -0.49, 0.021, 0.021, 0.012, gunMetal),    // muzzle
				r(0, 0.02, -0.5, 0.019, 0.019, 0.002, gunBlack),
				b(0, -0.014, -0.26, 0.014, 0.014, 0.15, gunBlack),   // magazine tube
				b(0, -0.014, -0.415, 0.016, 0.016, 0.006, gunMetal), // tube cap
				b(0, -0.014, -0.2, 0.026, 0.024, 0.07, markerLime),  // pump
				b(0, -0.012, 0.24, 0.028, 0.04, 0.08, markerPurple), // stock
				b(0, -0.012, 0.325, 0.03, 0.045, 0.008, rubber),     // butt pad
				// A big ghost ring rear, on protective ears, that frames the
				// target; and a glowing fibre bead up front.
				b(0, 0.048, 0.07, 0.022, 0.008, 0.014, gunBlack),
				b(-0.026, 0.068, 0.07, 0.004, 0.026, 0.006, gunBlack),
				b(0.026, 0.068, 0.07, 0.004, 0.026, 0.006, gunBlack),
				o(0, 0.07, 0.07, 0.019, gunBlack),
				o(0, 0.07, 0.069, 0.0165, gunMetal),
				b(0, 0.055, -0.46, 0.003, 0.015, 0.004, gunBlack),
				glow(r(0, 0.07, -0.46, 0.004, 0.004, 0.004, fiberRed)),
				glow(r(0, 0.07, -0.455, 0.0022, 0.0022, 0.006, withAlpha(fiberRed, 0.6))), // the fibre's glow
			},
			row(b(0, -0.0145, -0.245, 0.0265, 0.0245, 0.0025, rubber), 5, mathx.Vec3{0, 0, 0.022}),      // pump ridges
			row(b(0.034, -0.006, -0.03, 0.004, 0.006, 0.011, markerOrange), 4, mathx.Vec3{0, 0, 0.026}), // spare paint shells
			hopper(-0.085, 0.04, 0.02, 0.05),
			grip(-0.035, 0.12),
		),
		muzzle: mathx.Vec3{0, 0.02, -0.51}, sight: mathx.Vec3{0, 0.07, 0.1}, size: 0.55, relief: 0.14,
	},
	arena.WeaponSniper: {
		parts: join(
			[]gunPart{
				b(0, 0, 0.03, 0.028, 0.036, 0.16, markerWhite),      // receiver
				b(0.0285, 0.0, 0.03, 0.0006, 0.008, 0.14, gunBlack), // stripe
				b(0.028, 0.02, 0.1, 0.008, 0.006, 0.01, gunMetal),   // bolt
				r(0.036, 0.02, 0.1, 0.007, 0.007, 0.007, gunBlack),  // bolt knob
				b(0, 0.01, -0.25, 0.018, 0.018, 0.1, gunMetal),      // shroud
				b(0, 0.01, -0.47, 0.012, 0.012, 0.14, gunBlack),     // barrel
				b(0, 0.01, -0.625, 0.017, 0.013, 0.025, gunMetal),   // muzzle brake
				b(0, 0.01, -0.625, 0.018, 0.004, 0.014, gunBlack),
				b(0, -0.05, -0.02, 0.016, 0.03, 0.03, gunBlack),     // magazine
				b(-0.012, -0.02, -0.3, 0.003, 0.003, 0.1, gunBlack), // bipod, folded
				b(0.012, -0.02, -0.3, 0.003, 0.003, 0.1, gunBlack),
				b(0, -0.01, 0.25, 0.027, 0.044, 0.09, markerWhite), // stock
				b(0, 0.042, 0.23, 0.02, 0.01, 0.05, markerWhite),   // cheek riser
				b(0, -0.01, 0.345, 0.029, 0.048, 0.008, rubber),    // butt pad
				r(0, -0.06, 0.16, 0.02, 0.02, 0.06, gasSteel),      // gas
				// The scope: tube, bells, turrets, rings and glowing glass.
				b(0, 0.085, 0.0, 0.016, 0.016, 0.11, gunBlack),
				r(0, 0.085, -0.13, 0.025, 0.025, 0.035, gunBlack),
				r(0, 0.085, 0.12, 0.021, 0.021, 0.025, gunBlack),
				b(0, 0.105, -0.01, 0.009, 0.007, 0.009, gunMetal),     // elevation
				b(0.021, 0.085, -0.01, 0.007, 0.009, 0.009, gunMetal), // windage
				b(0, 0.064, -0.06, 0.013, 0.012, 0.008, gunBlack),     // rings
				b(0, 0.064, 0.06, 0.013, 0.012, 0.008, gunBlack),
				glow(b(0, 0.085, -0.163, 0.02, 0.02, 0.0006, lensBlue)), // objective
				glow(b(0, 0.085, 0.144, 0.015, 0.015, 0.0006, withAlpha(lensBlue, 0.5))),
			},
			hopper(-0.07, 0.03, 0.06, 0.04),
			grip(-0.035, 0.13),
		),
		muzzle: mathx.Vec3{0, 0.01, -0.65}, sight: mathx.Vec3{0, 0.085, 0.16}, size: 0.55, relief: 0.2, scope: true,
	},
	arena.WeaponLauncher: {
		parts: join(
			[]gunPart{
				b(0, 0, -0.12, 0.05, 0.05, 0.24, launcherGreen), // tube
				b(0, 0, -0.365, 0.058, 0.058, 0.012, gunBlack),  // muzzle
				r(0, 0, -0.378, 0.045, 0.045, 0.004, gunMetal),
				b(0, 0.052, -0.12, 0.012, 0.003, 0.2, gunBlack),      // top rail
				r(0, -0.03, 0.08, 0.075, 0.075, 0.065, hopperYellow), // drum
				b(0, -0.03, 0.08, 0.02, 0.02, 0.07, gunMetal),        // drum axle
				b(0, -0.055, -0.22, 0.015, 0.045, 0.02, rubber),      // fore grip
				b(0, -0.01, 0.23, 0.03, 0.045, 0.06, launcherGreen),  // stock
				b(0, -0.01, 0.295, 0.032, 0.05, 0.008, rubber),
				// The ladder sight: two uprights and a glowing crossbar.
				b(0, 0.058, -0.03, 0.014, 0.004, 0.012, gunBlack),
				b(-0.013, 0.078, -0.03, 0.0025, 0.02, 0.002, gunBlack),
				b(0.013, 0.078, -0.03, 0.0025, 0.02, 0.002, gunBlack),
				glow(b(0, 0.086, -0.03, 0.011, 0.0012, 0.0012, markerOrange)),
				glow(b(0, 0.074, -0.03, 0.011, 0.0008, 0.0012, markerOrange)),
			},
			row(b(0, 0, -0.33, 0.052, 0.052, 0.004, gunBlack), 4, mathx.Vec3{0, 0, 0.06}),                   // tube bands
			row(glow(r(0.06, -0.03, 0.08, 0.014, 0.014, 0.014, paintBalls)), 3, mathx.Vec3{-0.06, 0.03, 0}), // grenades in the drum
			grip(-0.04, 0.05),
		),
		muzzle: mathx.Vec3{0, 0, -0.39}, sight: mathx.Vec3{0, 0.086, 0.02}, size: 0.45, relief: 0.3,
	},
}
