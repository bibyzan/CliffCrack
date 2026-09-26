package game

import (
	"fmt"

	"CliffCrack/engine/gfx"
	"CliffCrack/engine/input"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/render"
	"CliffCrack/engine/ui"
	"CliffCrack/game/arena"
)

// Gadgets: the hammer and the grapple, chosen before each round.

// choosingGadget reports whether the countdown's gadget choice is up: its
// first seconds, before the last CountdownCall are counted down.
func (m *Arena) choosingGadget() bool {
	mt := m.match
	return !mt.Practice && mt.Phase == arena.PhaseCountdown && mt.Timer > arena.CountdownCall
}

// noteGadgetChoice keeps a gadget chosen in the countdown (c, this frame's
// input) for the next match: saved at once. It follows the choice as made
// here rather than the player's gadget, which online only reaches a guest a
// moment later.
func (m *Arena) noteGadgetChoice(c arena.Input) {
	if m.match.Practice || m.match.Phase != arena.PhaseCountdown {
		return
	}
	g, n := m.settings.Gadget, int(arena.GadgetKinds)
	switch {
	case c.Select >= 1 && c.Select <= n:
		g = c.Select - 1
	case c.Cycle != 0 || c.Gadget:
		g = (g + 1) % n
	default:
		return
	}
	m.rememberGadget(g)
}

// rememberGadget saves g as the gadget to take into the next match.
func (m *Arena) rememberGadget(g int) {
	if g == m.settings.Gadget {
		return
	}
	m.settings.Gadget = g
	if m.saveSettings != nil {
		m.saveSettings()
	}
}

// gadgetUnderMouse is the gadget card the mouse (or a finger) is over while
// choosing.
func (m *Arena) gadgetUnderMouse(in *input.State) (arena.GadgetKind, bool) {
	w, h := render.DisplaySize()
	if w <= 0 || h <= 0 {
		return 0, false
	}
	mx, my := in.MousePos()
	fx, fy := float32(mx)/float32(w), float32(my)/float32(h)
	aspect := float32(w) / float32(h)
	for k := range arena.GadgetKinds {
		cx := 0.5 + gadgetCardX(k)/aspect
		if abs32(fx-cx) < gadgetCardW/2/aspect && abs32(fy-gadgetCardY) < gadgetCardH/2 {
			return k, true
		}
	}
	return 0, false
}

// gadgetBlurbs say what each gadget does, under its name in the choice.
var gadgetBlurbs = [arena.GadgetKinds][2]string{
	{"smashes walls · two blows down a player", "no shooting while it's out"},
	{"hook on and reel yourself in, fast", "recharges after each pull"},
}

// gadgetChoiceUI is the countdown's text: what to do, the round and the
// time left, and under each gadget's icon (drawn by the helmet) its name,
// key and what it does, the chosen one lit.
func (m *Arena) gadgetChoiceUI(b *ui.Builder, in *input.State, secs int) {
	mt := m.match
	b.Panel("##choose", 0.5, 0.2, hudText, 2.6)
	b.ColorText(uiWhite, "CHOOSE YOUR GADGET")
	b.End()
	round := "ROUND %d"
	if mt.Round == 3 {
		round = "FINAL ROUND"
	}
	b.Panel("##choosesub", 0.5, 0.275, hudText, 1.3)
	if mt.Round == 3 {
		b.ColorText(uiMuted, round+"  ·  LAUNCHING IN %d", secs)
	} else {
		b.ColorText(uiMuted, round+"  ·  LAUNCHING IN %d", mt.Round, secs)
	}
	b.End()
	me := m.me()
	aspect := max(m.viewAspect, 0.5)
	for k := range arena.GadgetKinds {
		x := 0.5 + gadgetCardX(k)/aspect
		col, sub := uiWhite, withAlpha(uiWhite, 0.6)
		if arena.GadgetKind(k) == me.Gadget {
			col, sub = uiAccent, uiWhite
		}
		b.Panel("##gadgetname"+arena.GadgetNames[k], x, 0.56, hudText, 1.5)
		b.ColorText(col, "%d  %s", k+1, arena.GadgetNames[k])
		b.End()
		for i, line := range gadgetBlurbs[k] {
			b.Panel(fmt.Sprintf("##gadgetblurb%d%d", k, i), x, 0.615+float32(i)*0.035, hudText, 0.9)
			b.ColorText(sub, "%s", line)
			b.End()
		}
	}
	b.Panel("##choosekeys", 0.5, 0.76, hudText, 1.1)
	b.ColorText(uiMuted, "%s", prompt(in, "click,  1 / 2  or  Q  to choose", "RB / d-pad  choose", "tap one to choose"))
	b.End()
}

// appendGrapples draws every grapple line: from the launcher on the wrist
// (yours: low at the left of the view) out to the hook, flying out when
// fired, taut while it pulls, and snapping back after a miss.
func (m *Arena) appendGrapples(out []render.DrawCmd) []render.DrawCmd {
	for _, p := range m.sim().Players {
		g := p.Grapple
		if p.Dead || (!g.On && !(g.Miss && g.Shot < arena.GrappleFly+0.25)) {
			continue
		}
		var from mathx.Vec3
		if p == m.me() {
			from = m.camWorld().TransformPoint(mathx.Vec3{-0.2, -0.16, -0.35})
		} else {
			right, _ := flatRight(p.Yaw)
			from = p.Eye(1).Add(right.Scale(-0.25)).Add(mathx.Vec3{0, -0.35, 0}).Add(p.Forward().Scale(0.3))
		}
		reach := clampf(g.Shot/arena.GrappleFly, 0, 1) // flying out
		if g.Miss && g.Shot > arena.GrappleFly {
			reach = 1 - clampf((g.Shot-arena.GrappleFly)/0.25, 0, 1) // and back
		}
		to := lerp3(from, g.To, smooth(reach))
		out = m.segment(out, from, to, 0.012, 0.012, gunBlack)
		out = m.joint(out, to, 0.05, uiAccent) // the hook
		if g.On {
			out = append(out, render.DrawCmd{Model: bodyMatrix(to, mathx.QuatIdentity(), 0.12),
				Color: withAlpha(uiAccent, 0.35), Flags: gfx.DrawUnlit, Mesh: m.sc.chip})
		}
	}
	return out
}
