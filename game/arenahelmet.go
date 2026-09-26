package game

import (
	"embed"
	"fmt"
	"math"

	"CliffCrack/engine/geom"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/render"
	"CliffCrack/engine/svgicon"
	"CliffCrack/game/arena"
)

// The helmet HUD is drawn in the world, a hand's width in front of the eye,
// so it can use shapes and icons the text UI can't: a segmented armour bar,
// chevrons pointing at whoever shot you, hit markers, and SVG icons for the
// weapons, grenades and the rounds left in the magazine.

//go:embed icons/*.svg
var iconFiles embed.FS

// icon is an SVG rasterized to a texture.
type icon struct {
	tex    render.Texture
	aspect float32 // width / height
}

// hudIcons are every icon the HUD draws.
type hudIcons struct {
	weapons                    [len(arena.WeaponNames)]icon
	frag, sticky               icon
	ball, shell, round, bullet icon // magazine rounds: paintball, shell, sniper round, launcher grenade
	chevron                    icon
	gadgets                    [arena.GadgetKinds]icon
	elbow                      icon
	// The kill feed's: a headshot, a fall, a knock off the edge, rubble.
	headshot, fall, knockoff, rubble icon
}

// loadIcon rasterizes icons/<name>.svg, height pixels tall, into a texture.
func loadIcon(name string, height int) (icon, error) {
	f, err := iconFiles.Open("icons/" + name + ".svg")
	if err != nil {
		return icon{}, err
	}
	defer f.Close()
	ic, err := svgicon.Parse(f)
	if err != nil {
		return icon{}, fmt.Errorf("%s: %w", name, err)
	}
	tex, err := render.CreateTexture(ic.Rasterize(height), true)
	return icon{tex: tex, aspect: float32(ic.Aspect())}, err
}

// loadIcons rasterizes the embedded SVGs.
func loadIcons() (hudIcons, error) {
	load := loadIcon
	var h hudIcons
	var err error
	for k, name := range []string{"hammer", "rifle", "pistol", "shotgun", "sniper", "launcher", "smg"} {
		if h.weapons[k], err = load(name, 96); err != nil {
			return h, err
		}
	}
	for _, x := range []struct {
		dst    *icon
		name   string
		height int
	}{
		{&h.frag, "frag", 96}, {&h.sticky, "sticky", 96}, {&h.ball, "ball", 32}, {&h.shell, "shell", 48},
		{&h.round, "round", 64}, {&h.bullet, "grenade", 40}, {&h.chevron, "chevron", 64},
		{&h.gadgets[arena.GadgetGrapple], "grapple", 96}, {&h.elbow, "elbow", 96},
		{&h.headshot, "headshot", 64}, {&h.fall, "fall", 64}, {&h.knockoff, "knockoff", 64}, {&h.rubble, "rubble", 64},
	} {
		if *x.dst, err = load(x.name, x.height); err != nil {
			return h, err
		}
	}
	h.gadgets[arena.GadgetHammer] = h.weapons[arena.WeaponHammer]
	return h, nil
}

// hudQuad is a unit quad (-1..1) facing +Z, textured top to bottom.
func hudQuad() geom.MeshData {
	var m geom.MeshData
	for _, c := range [][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		m.Vertices = append(m.Vertices, geom.Vertex{Position: mathx.Vec3{c[0], c[1], 0}, Normal: mathx.Vec3{0, 0, 1},
			UV: [2]float32{(c[0] + 1) / 2, (1 - c[1]) / 2}})
	}
	m.Indices = []uint32{0, 1, 2, 0, 2, 3}
	return m
}

// helmet draws the HUD layer: positions are screen fractions (0,0 top
// left), sizes fractions of the screen's height.
type helmet struct {
	m             *Arena
	cam           mathx.Mat4
	d             float32 // m in front of the eye
	halfH, aspect float32
	out           []render.DrawCmd
}

const helmetDepth = 0.07

func (m *Arena) newHelmet(out []render.DrawCmd, fovY, aspect float32) *helmet {
	return &helmet{m: m, cam: m.camWorld(), d: helmetDepth, halfH: helmetDepth * float32(math.Tan(float64(fovY)/2)),
		aspect: aspect * m.settings.hudBox(), out: out} // laid out in the HUD box
}

// at is screen position (sx, sy) in camera space.
func (h *helmet) at(sx, sy float32) (x, y float32) {
	return (sx*2 - 1) * h.halfH * h.aspect, (1 - sy*2) * h.halfH
}

// size is a fraction of the screen's height in metres at the HUD's depth.
func (h *helmet) size(f float32) float32 { return f * 2 * h.halfH }

// rect is a filled box centred at (sx, sy), w by ht, turned by rot radians.
func (h *helmet) rect(sx, sy, w, ht, rot float32, col [4]float32) {
	x, y := h.at(sx, sy)
	model := h.cam.Mul(mathx.Translate(x, y, -h.d)).Mul(mathx.RotateZ(rot)).
		Mul(mathx.Scale(h.size(w)/2, h.size(ht)/2, 0.00001))
	h.out = append(h.out, render.DrawCmd{Model: model, Color: col, Flags: gfx.DrawUnlit, Mesh: h.m.as.cube})
}

// icon draws ic centred at (sx, sy), ht tall, turned by rot, tinted col.
func (h *helmet) icon(ic icon, sx, sy, ht, rot float32, col [4]float32) {
	x, y := h.at(sx, sy)
	half := h.size(ht) / 2
	model := h.cam.Mul(mathx.Translate(x, y, -h.d)).Mul(mathx.RotateZ(rot)).Mul(mathx.Scale(half*ic.aspect, half, 1))
	h.out = append(h.out, render.DrawCmd{Model: model, Color: col, Texture: ic.tex, Flags: gfx.DrawUnlit, Mesh: h.m.as.quad})
}

// hitDir is where a hit you took came from, for its chevron.
type hitDir struct {
	from mathx.Vec3
	age  float32
}

const (
	hitDirLife  = 1.5  // s a damage chevron shows
	killMarkLen = 0.45 // s the kill marker shows
)

// appendHelmet draws the HUD layer.
func (m *Arena) appendHelmet(out []render.DrawCmd, fovY, aspect float32) []render.DrawCmd {
	me := m.me()
	h := m.newHelmet(out, fovY, aspect)
	h.visor()
	if m.choosingGadget() {
		h.gadgetChoice(me)
		return h.out
	}
	if !me.Dead {
		h.armourBar(me)
		h.hitMarkers()
		h.damageChevrons()
		h.loadout(me)
	}
	return h.out
}

// visor frames the view like the inside of a helmet: faint brackets out
// in the corners of the HUD box, clear of everything in it.
func (h *helmet) visor() {
	col := withAlpha(teamGlow[0], 0.2)
	const margin, arm, thick = 0.008, 0.04, 0.0025 // in screen heights
	mx := margin / h.aspect                        // ... as a fraction of the box's width
	for _, c := range [][2]float32{{mx, margin}, {1 - mx, margin}, {mx, 1 - margin}, {1 - mx, 1 - margin}} {
		sx, sy := c[0], c[1]
		dx, dy := float32(1), float32(1)
		if sx > 0.5 {
			dx = -1
		}
		if sy > 0.5 {
			dy = -1
		}
		h.rect(sx+dx*arm/2/h.aspect, sy, arm, thick, 0, col)
		h.rect(sx, sy+dy*arm/2, thick, arm, 0, col)
	}
}

// armourBar is your own armour across the top: segments that drain as it's
// hit, flash as they're hit, and sweep back as it recharges; amber when
// low, and red and flashing once it's gone, with your health showing under
// it.
func (h *helmet) armourBar(me *arena.Player) {
	m := h.m
	const segs = 24
	const y, width float32 = 0.042, 0.42 // width in screen heights
	frac := me.Shield / arena.MaxShield
	colour := teamGlow[0]
	switch {
	case frac <= 0:
		colour = hurtColor
	case frac < 0.3:
		colour = uiAccent
	}
	flash := clampf(m.hurt*2, 0, 1)
	segW := width / float32(segs)
	x0 := 0.5 - width/2/h.aspect
	for i := range segs {
		sx := x0 + (float32(i)+0.5)*segW/h.aspect
		fill := clampf(frac*segs-float32(i), 0, 1)
		col := withAlpha(colour, 0.18)
		if fill > 0 {
			col = lerpColor(withAlpha(colour, 0.35+0.55*fill), withAlpha(uiWhite, 0.95), flash*0.6)
		}
		if m.charging && fill > 0 && fill < 1 {
			col = withAlpha(uiWhite, 0.95) // the leading edge as it fills
		}
		h.rect(sx, y, segW*0.8, 0.016, 0, col)
	}
	// The frame: rails and angled ends; pulsing red once it's gone.
	frame := withAlpha(uiWhite, 0.35)
	if frac <= 0 {
		frame = withAlpha(hurtColor, 0.5+0.4*float32(math.Sin(float64(m.elapsed)*8)))
	}
	h.rect(0.5, y-0.014, width+0.01, 0.0025, 0, frame)
	h.rect(0.5, y+0.014, width+0.01, 0.0025, 0, frame)
	for _, s := range []float32{-1, 1} {
		h.rect(0.5+s*(width/2+0.012)/h.aspect, y, 0.004, 0.034, s*0.45, frame)
	}
	// Health: pips under the bar, shown once it's taken a knock or the
	// armour's gone.
	if me.Health < arena.MaxHealth || frac <= 0 {
		const pips, pw = 8, 0.022
		hf := me.Health / arena.MaxHealth
		for i := range pips {
			sx := 0.5 + (float32(i)-float32(pips-1)/2)*pw/h.aspect
			col := withAlpha(hurtColor, 0.2)
			if hf*pips > float32(i) {
				col = withAlpha(hurtColor, 0.9)
			}
			h.rect(sx, y+0.032, pw*0.75, 0.007, 0, col)
		}
	}
}

// hitMarkers are four diagonal ticks round the crosshair whenever you land
// a hit: white on health, ice-white on armour, red for the head, bigger for
// a kill. Each is outlined in dark, to show against the snow and the sky.
func (h *helmet) hitMarkers() {
	m := h.m
	kill := m.killMark > 0
	if m.hitMark <= 0 && !kill {
		return
	}
	col, t := withAlpha(uiWhite, 0.95), m.hitMark/hitMarkTime
	switch {
	case kill:
		col, t = hurtColor, m.killMark/killMarkLen
	case m.headMark:
		col = hurtColor
	case m.armourMark:
		col = withAlpha(mathx.SRGB(0.8, 0.95, 1, 1), 0.95)
	}
	gap, long := float32(0.018), float32(0.016)
	if kill {
		gap, long = 0.022, 0.026
	}
	gap += 0.01 * (1 - t) // they spring out
	fade := min(t*2, 1)
	for _, outline := range []bool{true, false} {
		for _, s := range [][2]float32{{1, 1}, {-1, 1}, {1, -1}, {-1, -1}} {
			d := gap + long/2
			x, y, a := 0.5+s[0]*d*0.707/h.aspect, 0.5+s[1]*d*0.707, -s[0]*s[1]*math.Pi/4
			if outline {
				h.rect(x, y, long+0.004, 0.0035+0.003, a, [4]float32{0.02, 0.03, 0.06, 0.6 * fade})
			} else {
				h.rect(x, y, long, 0.0035, a, withAlpha(col, col[3]*fade))
			}
		}
	}
}

// gadget is your gadget, over the grenades: its icon, lit when it's ready,
// and under it the grapple recharging.
func (h *helmet) gadget(me *arena.Player, x, grenadeY float32) {
	ic := h.m.as.icons.gadgets[me.Gadget]
	const gh = 0.045
	y := grenadeY - 0.13
	if h.m.touchOn {
		y = grenadeY + 0.16
	}
	charge, ready := float32(1), true
	col := withAlpha(uiWhite, 0.95)
	switch me.Gadget {
	case arena.GadgetGrapple:
		charge, ready = me.Grapple.Ready()
		if me.Grapple.On {
			col = uiAccent
		} else if !ready {
			col = withAlpha(uiWhite, 0.35)
		}
	case arena.GadgetHammer:
		if me.HammerOut {
			col = uiAccent
		}
	}
	w := gh * ic.aspect
	cx := (x + w/2) / h.aspect
	h.icon(ic, cx, y, gh, 0, col)
	if !ready {
		bw := w
		h.rect(cx, y+gh*0.8, bw, 0.006, 0, withAlpha(uiWhite, 0.2))
		h.rect(cx-(bw*(1-charge)/2)/h.aspect, y+gh*0.8, bw*charge, 0.006, 0, withAlpha(uiAccent, 0.9))
	}
}

// gadgetChoice is the countdown's choice: a card for each gadget with its
// icon, the chosen one lit and framed (the words are in the UI layer: see
// gadgetChoiceUI, which lines up with gadgetCardX).
func (h *helmet) gadgetChoice(me *arena.Player) {
	for k := range arena.GadgetKinds {
		ic := h.m.as.icons.gadgets[k]
		cx := 0.5 + gadgetCardX(k)/h.aspect
		const gh, cw, ch, cy = 0.1, gadgetCardW, gadgetCardH, gadgetCardY
		chosen := arena.GadgetKind(k) == me.Gadget
		card, col := withAlpha(mathx.SRGB(0.03, 0.05, 0.1, 1), 0.55), withAlpha(uiWhite, 0.55)
		if arena.GadgetKind(k) == h.m.gadgetHover && !chosen {
			card, col = withAlpha(mathx.SRGB(0.08, 0.11, 0.2, 1), 0.7), withAlpha(uiWhite, 0.85)
		}
		if chosen {
			card, col = withAlpha(mathx.SRGB(0.03, 0.05, 0.1, 1), 0.75), uiAccent
		}
		h.rect(cx, cy, cw, ch, 0, card)
		h.d *= 0.995 // what's on the card, just in front of it
		if chosen {
			for _, e := range [][4]float32{{0, -ch / 2, cw, 0.005}, {0, ch / 2, cw, 0.005}, {-cw / 2, 0, 0.005, ch}, {cw / 2, 0, 0.005, ch}} {
				h.rect(cx+e[0]/h.aspect, cy+e[1], e[2], e[3], 0, withAlpha(uiAccent, 0.9))
			}
		}
		h.icon(ic, cx, cy-0.08, gh, 0, col)
		h.d = helmetDepth
	}
}

// The gadget choice's cards: their size and height on the screen, in
// screen heights (and see gadgetCardX).
const gadgetCardW, gadgetCardH, gadgetCardY = 0.56, 0.36, 0.52

// gadgetCardX is gadget k's card's centre across the screen, from the
// middle, in screen heights.
func gadgetCardX(k arena.GadgetKind) float32 { return (float32(k) - 0.5) * 0.66 }

// damageChevrons point round the crosshair at where recent hits came from.
func (h *helmet) damageChevrons() {
	m := h.m
	view := m.view()
	for _, d := range m.hitDirs {
		v := view.TransformPoint(d.from) // camera space: -Z ahead, +X right
		a := float32(math.Atan2(float64(v[0]), float64(-v[2])))
		fade := clampf(1-d.age/hitDirLife, 0, 1)
		const r = 0.14
		sx, sy := 0.5+r*float32(math.Sin(float64(a)))/h.aspect, 0.5-r*float32(math.Cos(float64(a)))
		h.icon(m.as.icons.chevron, sx, sy, 0.03, -a, withAlpha(hurtColor, 0.85*fade))
	}
}

// loadout is bottom right: the weapon in hand's icon with its magazine as
// rounds above it (spent ones dim), the other weapon smaller; and bottom
// left, your grenades. With the touch controls up, those corners are the
// stick's and the buttons': the weapons go bottom centre, over the ammo
// count, and the grenades top left, under the score.
func (h *helmet) loadout(me *arena.Player) {
	m := h.m
	ic := &m.as.icons
	right := 1 - 0.03/h.aspect // the right margin, as a screen fraction
	grenadeX, grenadeY := float32(0.035), float32(0.9)
	if m.touchOn {
		right = 0.5 + 0.2/h.aspect
		grenadeX = 0.5 + (m.leftX()-0.5)/m.settings.hudBox() // the score's left edge, in the box
		grenadeX *= h.aspect
		grenadeY = 0.21
	}
	place := func(width, y float32) float32 { return right - width/2/h.aspect }

	cur := ic.weapons[max(me.Holding(), 0)] // (the hammer, while it's out)
	const curH = 0.06
	h.icon(cur, place(curH*cur.aspect, 0.84), 0.84, curH, 0, withAlpha(uiWhite, 0.95))
	if other := me.Other(); other != arena.NoWeapon {
		o := ic.weapons[other]
		const oH = 0.032
		h.icon(o, place(oH*o.aspect, 0.72), 0.72, oH, 0, withAlpha(uiWhite, 0.4))
	}

	// The magazine: a round per shot, in rows from the right.
	var round icon
	var ammo, mag, perRow int
	var rh float32
	switch me.Holding() {
	case arena.WeaponRifle:
		round, rh, perRow = ic.ball, 0.011, 24
	case arena.WeaponPistol:
		round, rh, perRow = ic.ball, 0.016, 12
	case arena.WeaponSMG:
		round, rh, perRow = ic.ball, 0.009, 20
	case arena.WeaponShotgun:
		round, rh, perRow = ic.shell, 0.028, 8
	case arena.WeaponSniper:
		round, rh, perRow = ic.round, 0.04, 4
	case arena.WeaponLauncher:
		round, rh, perRow = ic.bullet, 0.022, 6
	}
	if g, s := me.Gun(); g != nil {
		ammo, mag = s.Ammo, g.Mag
	} else if me.Current == arena.WeaponLauncher {
		ammo, mag = me.Launcher.Ammo, arena.LauncherMag
	}
	if round.tex != 0 {
		step := rh * round.aspect * 1.35
		for i := range mag {
			row, col := i/perRow, i%perRow
			sx := right - (float32(col)+0.5)*step/h.aspect
			sy := 0.79 - float32(row)*rh*1.3
			c := withAlpha(uiWhite, 0.9)
			if i >= ammo {
				c = withAlpha(uiWhite, 0.15) // spent
			}
			h.icon(round, sx, sy, rh, 0, c)
		}
	}

	h.gadget(me, grenadeX, grenadeY)

	// Grenades, bottom left: an icon each, the kind G throws lit.
	for k, gi := range []icon{ic.frag, ic.sticky} {
		n := me.Grenades[k]
		col := withAlpha(uiWhite, 0.45)
		if arena.GrenadeKind(k) == me.GrenadeKind {
			col = withAlpha(uiWhite, 0.95)
		}
		const gh = 0.04
		y := grenadeY - float32(k)*0.055
		if m.touchOn {
			y = grenadeY + float32(k)*0.055 // frags on top, under the score
		}
		for i := range arena.MaxGrenades {
			c := col
			if i >= n {
				c = withAlpha(col, 0.12)
			}
			h.icon(gi, (grenadeX+float32(i)*gh*gi.aspect*1.2+gh*gi.aspect/2)/h.aspect, y, gh, 0, c)
		}
	}
}
