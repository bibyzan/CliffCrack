package game

import (
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/game/arena"
)

// The markers' models, in weapon space: metres, barrel down -Z, +Y up, the
// hand at the grip. Each is built from boxes, balls (hoppers, tanks, bells)
// and rings (sight reticles), with lit plastics and glowing sights: a mini
// reflex sight on the rifle, tritium irons on the pistol, a ghost ring and a fibre
// bead on the shotgun, a scope on the sniper, a ladder on the launcher.
var (
	sightRed   = mathx.SRGB(1.00, 0.16, 0.10, 1)
	tritium    = mathx.SRGB(0.35, 1.00, 0.35, 1)
	fiberRed   = mathx.SRGB(1.00, 0.20, 0.25, 1)
	tealDark   = mathx.SRGB(0.06, 0.40, 0.40, 1)
	rubber     = mathx.SRGB(0.10, 0.10, 0.11, 1)
	paintBalls = mathx.SRGB(0.10, 0.78, 1.00, 1)
	opticBlack = mathx.SRGB(0.05, 0.05, 0.06, 1) // anodised: a touch darker than the gun's black
	lensAmber  = mathx.SRGB(1.00, 0.55, 0.22, 1) // a reflex window's coating
	lensSheen  = mathx.SRGB(0.95, 0.45, 0.95, 1) // ... and the magenta at its edge
	chrome     = mathx.SRGB(0.72, 0.74, 0.78, 1) // polished steel
	walnut     = mathx.SRGB(0.42, 0.24, 0.13, 1) // a wooden grip
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

// d is a flat round disc of radius rad in the XY plane (a lens).
func d(cx, cy, cz, rad float32, col [4]float32) gunPart {
	p := b(cx, cy, cz, rad, rad, 1, col)
	p.disc = true
	return p
}

// soft rounds a box part's edges right off: machined, not cut.
func soft(p gunPart) gunPart {
	p.soft = true
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
				// A mini reflex sight, low on the rail: a clamp with a cross
				// bolt, a squat body with the brightness dial and two ears
				// low at the back, +/- buttons on the right; and at the front,
				// the hood round an amber-coated window with the red dot in
				// it. Machined: every edge rounded off.
				soft(b(0, 0.0615, 0.028, 0.0165, 0.0045, 0.044, gunBlack)),          // clamp base
				soft(b(0.0185, 0.0605, 0.032, 0.0025, 0.0065, 0.017, gunBlack)),     // clamp jaw
				r(0.0215, 0.0605, 0.032, 0.0015, 0.0032, 0.0032, gunMetal),          // cross bolt
				soft(b(0, 0.0725, 0.03, 0.0155, 0.0065, 0.04, opticBlack)),          // body
				soft(b(0, 0.0795, 0.057, 0.0135, 0.003, 0.012, opticBlack)),         // rear block, low: the eye looks over it
				r(0, 0.0828, 0.054, 0.0058, 0.0011, 0.0058, gunMetal),               // dial
				soft(b(-0.0105, 0.0835, 0.066, 0.0028, 0.0022, 0.0028, opticBlack)), // ears
				soft(b(0.0105, 0.0835, 0.066, 0.0028, 0.0022, 0.0028, opticBlack)),
				soft(b(0.0158, 0.0735, 0.03, 0.0012, 0.0038, 0.011, rubber)), // button panel
				r(0.0172, 0.0735, 0.025, 0.0009, 0.0026, 0.0026, gunBlack),   // + and -
				r(0.0172, 0.0735, 0.035, 0.0009, 0.0026, 0.0026, gunBlack),
				soft(b(0, 0.0805, 0.022, 0.0035, 0.0015, 0.0035, gunBlack)), // emitter
				// The hood: walls either side of the window, round top corners
				// (rounded rods along the hood), and the top between them.
				soft(b(-0.0175, 0.0875, -0.002, 0.003, 0.0155, 0.016, opticBlack)),
				soft(b(0.0175, 0.0875, -0.002, 0.003, 0.0155, 0.016, opticBlack)),
				soft(b(-0.0158, 0.1025, -0.002, 0.0047, 0.0047, 0.016, opticBlack)),
				soft(b(0.0158, 0.1025, -0.002, 0.0047, 0.0047, 0.016, opticBlack)),
				soft(b(0, 0.1045, -0.002, 0.016, 0.0028, 0.016, opticBlack)),
				soft(b(0, 0.1068, 0.012, 0.013, 0.0009, 0.0018, gunMetal)), // a lit edge along the back of the hood
				// The glass: amber, a magenta sheen at the top, and the dot.
				glow(b(0, 0.089, -0.004, 0.0148, 0.0142, 0.0004, withAlpha(lensAmber, 0.22))),
				glow(b(-0.004, 0.0985, -0.0037, 0.009, 0.0038, 0.0003, withAlpha(lensSheen, 0.2))),
				glow(r(0, 0.089, -0.002, 0.0012, 0.0012, 0.0004, sightRed)),
			},
			row(b(0, 0.057, -0.16, 0.013, 0.0015, 0.004, gunMetal), 9, mathx.Vec3{0, 0, 0.032}),      // rail notches
			row(b(0.032, 0.012, -0.235, 0.0008, 0.006, 0.014, gunBlack), 3, mathx.Vec3{0, 0, 0.034}), // handguard slots
			row(b(-0.032, 0.012, -0.235, 0.0008, 0.006, 0.014, gunBlack), 3, mathx.Vec3{0, 0, 0.034}),
			row(r(0, 0.018, -0.41, 0.0135, 0.0135, 0.004, gunMetal), 3, mathx.Vec3{0, 0, -0.03}), // barrel rings
			grip(-0.035, 0.09),
		),
		// Magazine-fed, as markers built for mag swaps are: a box of paint
		// with a window showing the balls in it.
		mag: []gunPart{
			b(0, -0.07, -0.035, 0.015, 0.042, 0.026, tealDark),
			b(0, -0.1, -0.03, 0.015, 0.014, 0.024, tealDark),
			b(0, -0.118, -0.03, 0.017, 0.004, 0.027, gunBlack), // base plate
			glow(b(0.0152, -0.075, -0.035, 0.0006, 0.028, 0.012, paintBalls)),
			glow(b(-0.0152, -0.075, -0.035, 0.0006, 0.028, 0.012, paintBalls)),
		},
		magHold: mathx.Vec3{0, -0.12, -0.03}, magDrop: mathx.Vec3{0, -1, 0.2},
		charge: mathx.Vec3{0.038, 0.032, 0.07},
		grip:   mathx.Vec3{0, -0.075, 0.09}, fore: mathx.Vec3{0, -0.09, -0.17},
		muzzle: mathx.Vec3{0, 0.018, -0.54}, sight: mathx.Vec3{0, 0.089, 0.08}, size: 0.55, relief: 0.1,
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
		mag: []gunPart{
			b(0, -0.07, 0.04, 0.012, 0.045, 0.016, gunBlack),
			b(0, -0.112, 0.04, 0.019, 0.006, 0.026, markerOrange), // base plate
			glow(b(0, -0.07, 0.0565, 0.006, 0.03, 0.0006, paintBalls)),
		},
		magHold: mathx.Vec3{0, -0.118, 0.04}, magDrop: mathx.Vec3{0, -1, 0},
		charge: mathx.Vec3{-0.025, 0.035, 0.05}, // the slide
		grip:   mathx.Vec3{0, -0.06, 0.045}, fore: mathx.Vec3{-0.02, -0.085, 0.03},
		muzzle: mathx.Vec3{0, 0.03, -0.16}, sight: mathx.Vec3{0, 0.058, 0.09}, size: 0.6, relief: 0.19,
	},
	arena.WeaponShotgun: {
		// After the SPAS-12: a long black receiver, a perforated heat shield
		// over the barrel, the tube magazine under it, a big ribbed pump,
		// and a skeletal folding stock with its hook; the sights a ghost
		// ring on ears and a glowing fibre bead.
		parts: join(
			[]gunPart{
				b(0, 0, 0.02, 0.029, 0.038, 0.13, gunBlack),               // receiver
				b(0.0295, 0.006, 0.03, 0.0006, 0.022, 0.09, markerPurple), // side panels
				b(-0.0295, 0.006, 0.03, 0.0006, 0.022, 0.09, markerPurple),
				b(0.0302, 0.012, -0.005, 0.0006, 0.011, 0.05, gunMetal), // ejection port
				b(0, 0.02, -0.31, 0.013, 0.013, 0.2, gunBlack),          // barrel
				b(0, 0.022, -0.3, 0.02, 0.02, 0.145, gunMetal),          // heat shield
				b(0, 0.02, -0.495, 0.018, 0.018, 0.012, gunBlack),       // muzzle
				r(0, 0.02, -0.508, 0.015, 0.015, 0.002, gunMetal),
				b(0, -0.022, -0.27, 0.014, 0.014, 0.2, gunBlack),    // magazine tube
				b(0, -0.022, -0.475, 0.016, 0.016, 0.008, gunMetal), // tube cap
				b(0, 0.0, -0.46, 0.012, 0.03, 0.01, gunBlack),       // barrel band
				// The folding stock: two struts back from the receiver to the
				// butt plate, and the hook folded down under it.
				b(0, 0.018, 0.22, 0.006, 0.006, 0.1, gunMetal),
				b(0, -0.035, 0.2, 0.006, 0.006, 0.1, gunMetal),
				b(0, -0.008, 0.315, 0.012, 0.052, 0.009, gunBlack), // butt plate
				b(0, -0.068, 0.31, 0.012, 0.012, 0.006, rubber),
				b(0, -0.082, 0.28, 0.005, 0.005, 0.03, gunMetal), // the hook
				b(0, -0.072, 0.252, 0.005, 0.015, 0.005, gunMetal),
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
			// The heat shield's rows of holes, top and sides.
			row(b(0, 0.0425, -0.42, 0.005, 0.0008, 0.006, gunBlack), 7, mathx.Vec3{0, 0, 0.038}),
			row(b(0.0205, 0.022, -0.42, 0.0008, 0.006, 0.006, gunBlack), 7, mathx.Vec3{0, 0, 0.038}),
			row(b(-0.0205, 0.022, -0.42, 0.0008, 0.006, 0.006, gunBlack), 7, mathx.Vec3{0, 0, 0.038}),
			row(b(0.034, -0.006, -0.03, 0.004, 0.006, 0.011, markerOrange), 4, mathx.Vec3{0, 0, 0.026}), // spare paint shells
			grip(-0.035, 0.12),
		),
		pump: join(
			[]gunPart{
				b(0, -0.016, -0.19, 0.03, 0.028, 0.09, markerPurple), // the pump: long and chunky
				b(0, -0.016, -0.285, 0.027, 0.025, 0.006, gunBlack),  // its front lip
			},
			row(b(0, -0.0165, -0.25, 0.0305, 0.0285, 0.003, rubber), 7, mathx.Vec3{0, 0, 0.02}), // grip ribs
		),
		reload:  reloadShells,
		magHold: mathx.Vec3{0, -0.045, 0.0}, // the loading port
		grip:    mathx.Vec3{0, -0.075, 0.12}, fore: mathx.Vec3{0, -0.045, -0.2}, foreFlat: true,
		muzzle: mathx.Vec3{0, 0.02, -0.51}, sight: mathx.Vec3{0, 0.07, 0.1}, size: 0.55, relief: 0.1,
	},
	arena.WeaponSniper: {
		parts: join(
			[]gunPart{
				b(0, 0, 0.03, 0.028, 0.036, 0.16, markerWhite),      // receiver
				b(0.0285, 0.0, 0.03, 0.0006, 0.008, 0.14, gunBlack), // stripe
				b(0, 0.03, 0.1, 0.012, 0.008, 0.04, gunMetal),       // the bolt's shroud
				b(0, 0.01, -0.25, 0.018, 0.018, 0.1, gunMetal),      // shroud
				b(0, 0.01, -0.47, 0.012, 0.012, 0.14, gunBlack),     // barrel
				b(0, 0.01, -0.625, 0.017, 0.013, 0.025, gunMetal),   // muzzle brake
				b(0, 0.01, -0.625, 0.018, 0.004, 0.014, gunBlack),
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
				glow(d(0, 0.085, -0.166, 0.021, lensBlue)), // objective
				glow(d(0, 0.085, 0.146, 0.016, withAlpha(lensBlue, 0.5))),
			},
			hopper(-0.07, 0.03, 0.06, 0.04),
			grip(-0.035, 0.13),
		),
		mag: []gunPart{
			b(0, -0.05, -0.02, 0.016, 0.03, 0.03, gunBlack),
			b(0, -0.081, -0.02, 0.018, 0.003, 0.032, markerWhite), // base plate
		},
		magHold: mathx.Vec3{0, -0.085, -0.02}, magDrop: mathx.Vec3{0, -1, 0.1},
		charge: mathx.Vec3{0.052, 0.02, 0.1}, // the bolt knob
		// The bolt: a handle out to the right with a big knob, turned up
		// and pulled back between shots (about the bore, boltPivot).
		bolt: []gunPart{
			b(0, 0.02, 0.1, 0.009, 0.009, 0.045, gunMetal),        // bolt body
			b(0.028, 0.02, 0.105, 0.02, 0.0045, 0.0045, gunMetal), // handle
			r(0.05, 0.02, 0.105, 0.009, 0.009, 0.009, gunBlack),   // knob
		},
		boltPivot: mathx.Vec3{0, 0.02, 0.1},
		grip:      mathx.Vec3{0, -0.075, 0.13}, fore: mathx.Vec3{0, -0.03, -0.22}, foreFlat: true,
		muzzle: mathx.Vec3{0, 0.01, -0.65}, sight: mathx.Vec3{0, 0.085, 0.16}, size: 0.55, relief: 0.2, scope: true,
	},
	arena.WeaponLauncher: {
		parts: join(
			[]gunPart{
				b(0, 0, -0.12, 0.05, 0.05, 0.24, launcherGreen), // tube
				b(0, 0, -0.365, 0.058, 0.058, 0.012, gunBlack),  // muzzle
				r(0, 0, -0.378, 0.045, 0.045, 0.004, gunMetal),
				b(0, 0.052, -0.12, 0.012, 0.003, 0.2, gunBlack),     // top rail
				b(0, -0.055, -0.22, 0.015, 0.045, 0.02, rubber),     // fore grip
				b(0, -0.01, 0.23, 0.03, 0.045, 0.06, launcherGreen), // stock
				b(0, -0.01, 0.295, 0.032, 0.05, 0.008, rubber),
				// The ladder sight: two uprights and a glowing crossbar.
				b(0, 0.058, -0.03, 0.014, 0.004, 0.012, gunBlack),
				b(-0.013, 0.078, -0.03, 0.0025, 0.02, 0.002, gunBlack),
				b(0.013, 0.078, -0.03, 0.0025, 0.02, 0.002, gunBlack),
				glow(b(0, 0.086, -0.03, 0.011, 0.0012, 0.0012, markerOrange)),
				glow(b(0, 0.074, -0.03, 0.011, 0.0008, 0.0012, markerOrange)),
			},
			row(b(0, 0, -0.33, 0.052, 0.052, 0.004, gunBlack), 4, mathx.Vec3{0, 0, 0.06}), // tube bands
			grip(-0.04, 0.05),
		),
		mag: join(
			[]gunPart{
				r(0, -0.03, 0.08, 0.075, 0.075, 0.065, hopperYellow), // drum
				b(0, -0.03, 0.08, 0.02, 0.02, 0.07, gunMetal),        // axle
			},
			row(glow(r(0.06, -0.03, 0.08, 0.014, 0.014, 0.014, paintBalls)), 3, mathx.Vec3{-0.06, 0.03, 0}),
		),
		reload:  reloadDrum,
		magHold: mathx.Vec3{0.06, -0.09, 0.08}, magDrop: mathx.Vec3{1, -0.3, 0},
		grip: mathx.Vec3{0, -0.085, 0.05}, fore: mathx.Vec3{0, -0.09, -0.22},
		muzzle: mathx.Vec3{0, 0, -0.39}, sight: mathx.Vec3{0, 0.086, 0.02}, size: 0.45, relief: 0.2,
	},
	arena.WeaponSMG: {
		// A compact blaster in lime and black: a short, deep receiver, a
		// stubby shrouded barrel, a vertical fore grip, a long straight
		// magazine ahead of the pistol grip, a wire stock folded along
		// the side, and a small rounded reflex sight.
		parts: join(
			[]gunPart{
				soft(b(0, 0.012, -0.01, 0.024, 0.032, 0.105, markerLime)), // receiver
				b(0, 0.046, -0.02, 0.011, 0.003, 0.08, gunBlack),          // top rail
				b(0.0245, 0.02, -0.05, 0.0006, 0.009, 0.03, gunBlack),     // ejection port
				soft(b(0, 0.012, -0.15, 0.019, 0.019, 0.05, gunBlack)),    // barrel shroud
				b(0, 0.012, -0.215, 0.009, 0.009, 0.02, gunMetal),         // barrel
				r(0, 0.012, -0.236, 0.011, 0.011, 0.003, gunBlack),        // muzzle
				b(0, -0.035, -0.035, 0.018, 0.008, 0.07, gunBlack),        // lower
				b(0, -0.058, -0.15, 0.011, 0.03, 0.012, rubber),           // fore grip
				b(0, -0.09, -0.15, 0.013, 0.004, 0.014, gunMetal),
				// The folded wire stock, along the left side.
				b(-0.029, 0.018, 0.035, 0.003, 0.003, 0.085, gunMetal),
				b(-0.029, -0.012, 0.035, 0.003, 0.003, 0.085, gunMetal),
				b(-0.029, 0.003, -0.05, 0.003, 0.018, 0.004, gunMetal),
				b(0, 0.012, 0.1, 0.015, 0.022, 0.006, gunBlack), // end cap
				// A small reflex sight.
				soft(b(0, 0.056, -0.01, 0.012, 0.006, 0.025, opticBlack)),
				soft(b(-0.011, 0.073, -0.022, 0.0025, 0.011, 0.008, opticBlack)),
				soft(b(0.011, 0.073, -0.022, 0.0025, 0.011, 0.008, opticBlack)),
				soft(b(0, 0.084, -0.022, 0.013, 0.002, 0.008, opticBlack)),
				glow(b(0, 0.071, -0.024, 0.0086, 0.0086, 0.0004, withAlpha(lensAmber, 0.22))),
				glow(r(0, 0.071, -0.022, 0.001, 0.001, 0.0004, sightRed)),
			},
			row(b(0.0245, 0.0, -0.13, 0.0006, 0.007, 0.004, gunBlack), 3, mathx.Vec3{0, 0, 0.012}), // shroud vents
			row(b(-0.0245, 0.0, -0.13, 0.0006, 0.007, 0.004, gunBlack), 3, mathx.Vec3{0, 0, 0.012}),
			grip(-0.04, 0.06),
		),
		mag: []gunPart{
			b(0, -0.085, -0.025, 0.012, 0.055, 0.014, gunBlack),
			b(0, -0.142, -0.025, 0.015, 0.004, 0.017, markerLime), // base plate
			glow(b(0.0125, -0.085, -0.025, 0.0006, 0.04, 0.007, paintBalls)),
		},
		magHold: mathx.Vec3{0, -0.145, -0.025}, magDrop: mathx.Vec3{0, -1, 0.1},
		charge: mathx.Vec3{0.028, 0.03, 0.02},
		grip:   mathx.Vec3{0, -0.08, 0.06}, fore: mathx.Vec3{0, -0.07, -0.15},
		muzzle: mathx.Vec3{0, 0.012, -0.24}, sight: mathx.Vec3{0, 0.071, 0.04}, size: 0.55, relief: 0.12,
	},
	arena.WeaponRevolver: {
		// A big polished hand cannon: a long barrel over a full underlug, a
		// chunky frame and hammer, a rounded wooden grip, a notch rear sight
		// and a glowing red front blade. The fluted cylinder swings out to
		// the left to reload.
		parts: join(
			[]gunPart{
				soft(b(0, 0.022, -0.02, 0.018, 0.028, 0.045, chrome)),  // frame
				soft(b(0, 0.04, -0.15, 0.013, 0.012, 0.09, chrome)),    // barrel
				soft(b(0, 0.018, -0.14, 0.012, 0.013, 0.085, chrome)),  // underlug
				b(0, 0.052, -0.14, 0.004, 0.003, 0.085, gunBlack),      // vent rib
				r(0, 0.04, -0.242, 0.011, 0.011, 0.003, gunBlack),      // muzzle
				soft(b(0, 0.055, 0.035, 0.005, 0.01, 0.012, gunBlack)), // hammer
				b(0, 0.059, 0.02, 0.009, 0.003, 0.006, gunBlack),       // rear sight
				b(0, 0.057, -0.225, 0.0025, 0.007, 0.008, gunBlack),    // front blade
				glow(b(0, 0.063, -0.221, 0.0026, 0.0026, 0.0006, fiberRed)),
				b(0, -0.012, 0.0, 0.003, 0.012, 0.028, gunBlack),      // trigger guard
				b(0, -0.006, 0.005, 0.003, 0.01, 0.003, gunMetal),     // trigger
				soft(b(0, -0.045, 0.05, 0.016, 0.045, 0.022, walnut)), // grip
				soft(b(0, -0.088, 0.063, 0.017, 0.008, 0.024, walnut)),
			},
		),
		// The cylinder: fluted, six chambers of paint showing at the back.
		mag: join(
			[]gunPart{r(0, 0.032, -0.052, 0.024, 0.024, 0.03, chrome)},
			[]gunPart{
				glow(r(0, 0.05, -0.022, 0.0045, 0.0045, 0.002, paintBalls)),
				glow(r(0.0155, 0.041, -0.022, 0.0045, 0.0045, 0.002, paintBalls)),
				glow(r(0.0155, 0.023, -0.022, 0.0045, 0.0045, 0.002, paintBalls)),
				glow(r(0, 0.014, -0.022, 0.0045, 0.0045, 0.002, paintBalls)),
				glow(r(-0.0155, 0.023, -0.022, 0.0045, 0.0045, 0.002, paintBalls)),
				glow(r(-0.0155, 0.041, -0.022, 0.0045, 0.0045, 0.002, paintBalls)),
				b(0.0245, 0.032, -0.052, 0.001, 0.004, 0.026, gunBlack), // flutes
				b(-0.0245, 0.032, -0.052, 0.001, 0.004, 0.026, gunBlack),
				b(0, 0.0565, -0.052, 0.004, 0.001, 0.026, gunBlack),
			},
		),
		reload:  reloadDrum,
		magHold: mathx.Vec3{-0.03, 0.032, -0.052}, magDrop: mathx.Vec3{-1, -0.3, 0},
		kick: 0.3,
		grip: mathx.Vec3{0, -0.05, 0.05}, fore: mathx.Vec3{-0.02, -0.08, 0.04},
		muzzle: mathx.Vec3{0, 0.04, -0.25}, sight: mathx.Vec3{0, 0.062, 0.08}, size: 0.6, relief: 0.2,
	},
}
