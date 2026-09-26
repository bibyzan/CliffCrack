package game

import (
	"fmt"
	"math"

	"CliffCrack/engine/gfx"
	"CliffCrack/engine/input"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/ui"
	"CliffCrack/game/arena"
)

const arenaHintTime = 8.0 // seconds the controls hint stays up

// UI draws the HUD: the round score and clock, crosshair and hit marker,
// health, the bot's name tag when it's in sight, the kill feed, the weapon
// bar with ammo and reload, and the countdown and result banners.
func (m *Arena) UI(b *ui.Builder, in *input.State) {
	mt, me := m.match, m.me()
	anchored := hudText &^ gfx.UICentered
	if m.touchOn {
		m.touch.ui(b)
	}

	if mt.Practice {
		m.rangeHUD(b)
	} else {
		// Score, top left under the visor's corner (the armour bar has the
		// top middle): YOU 1 : 0 BOT, the round and its clock.
		b.Panel("##score", m.leftX(), 0.075, anchored, 1.5)
		// You first; then everyone else, by name online.
		b.ColorText(suitColor[0], "YOU %d", mt.Wins[local])
		for _, p := range m.sim().Players {
			if p.ID == local {
				continue
			}
			b.SameLine(18)
			b.ColorText(suitColor[1], "%d %s", mt.Wins[p.ID], m.playerName(p))
		}
		b.End()
		b.Panel("##round", m.leftX(), 0.135, anchored, 1.0)
		clock := ""
		if mt.Phase == arena.PhaseFight {
			t := int(math.Ceil(float64(mt.Timer)))
			clock = fmt.Sprintf("   ·   %d:%02d", t/60, t%60)
		}
		b.ColorText(uiMuted, "ROUND %d  ·  FIRST TO %d%s", mt.Round, arena.RoundsToWin, clock)
		b.End()
	}

	m.nameTags(b)

	// Kill feed, top right.
	for i, f := range m.feed {
		fade := clampf((feedLife-f.age)*3, 0, 1)
		b.Panel("##feed"+string(rune('0'+i%10)), m.hudX(0.985), 0.03+0.04*float32(i), anchored, 1.15)
		c := uiMuted
		if f.good {
			c = uiAccent
		}
		b.ColorText(withAlpha(c, fade), "%s", f.text)
		b.End()
	}

	if !me.Dead {
		m.weaponHUD(b, in, me)
	}
	m.banner(b, in)

	if m.elapsed < arenaHintTime && !m.Autopilot && mt.Round == 1 {
		fade := clampf((arenaHintTime-m.elapsed)*2, 0, 1)
		hintY := float32(0.975)
		if m.touchOn {
			hintY = 0.2
		}
		b.Panel("##arenahint", 0.5, hintY, hudText, 1.05)
		b.ColorText(withAlpha(uiMuted, fade), "%s", prompt(in,
			"LMB  fire    RMB  aim    1 2 / wheel  swap    R  reload    E  pick up    F  elbow    Q  gadget    G  grenade    C  frag / sticky    Ctrl  crouch (sprinting: slide)    Space  jump, vault, climb",
			"RT  fire    LT  aim    Y  swap    X  reload (hold: pick up)    R3  elbow    RB  gadget    LB  grenade    D-pad down  frag / sticky    B  crouch (sprinting: slide)",
			"Stick  move (push all the way to sprint)  ·  drag right side  look  ·  CROUCH while sprinting to slide  ·  JUMP at a ledge to climb"))
		b.End()
	}
}

// weaponHUD is the words that go with the helmet's icons (see
// appendHelmet): the magazine and reserve, and a prompt when a weapon's in
// reach. (Reloads show in the hands, not as a bar.)
func (m *Arena) weaponHUD(b *ui.Builder, in *input.State, me *arena.Player) {
	anchored := hudText &^ gfx.UICentered
	if m.touchOn {
		b.Panel("##ammo", 0.5, 0.975, hudText, 2.8) // the buttons have the corner
	} else {
		b.Panel("##ammo", m.hudX(0.985), 0.975, anchored, 2.8)
	}
	switch g, s := me.Gun(); {
	case g != nil:
		ammoText(b, s.Ammo, s.Reserve, s.Reloading > 0)
	case me.Current == arena.WeaponLauncher:
		ammoText(b, me.Launcher.Ammo, me.Launcher.Reserve, me.Launcher.Reloading > 0)
	default:
		b.ColorText(uiMuted, "--")
	}
	b.End()

	if p := m.sim().NearestPickup(me); p != nil {
		b.Panel("##pickup", 0.5, 0.6, hudText, 1.15)
		verb := "pick up"
		switch {
		case p.IsGadget:
			verb = "swap " + arena.GadgetNames[me.Gadget] + " for"
		case me.Other() != arena.NoWeapon:
			verb = "swap " + arena.WeaponNames[me.Current] + " for"
		}
		b.ColorText(uiWhite, "%s", prompt(in, "E  "+verb+" "+p.Name(), "hold X  "+verb+" "+p.Name(),
			"PICK UP  "+verb+" "+p.Name()))
		b.End()
	}
}

// nameTags labels the other players you can see.
func (m *Arena) nameTags(b *ui.Builder) {
	s, me := m.sim(), m.me()
	eye := m.eye()
	if mk, ok := m.markerFor(me.Current); ok && mk.scope && me.ADS > 0.85 {
		return // through a scope you see them, not labels
	}
	for _, p := range s.Players {
		if p == me || p.Dead || !s.CanSee(eye, p.Head()) {
			continue
		}
		tag := p.Head()
		tag[1] += 0.5
		x, y, ok := m.project(tag)
		if !ok {
			continue
		}
		dist := tag.Sub(eye).Len()
		scale := clampf(1.3-dist/60, 0.75, 1.3)
		b.Panel(fmt.Sprintf("##tag%d", p.ID), x, y, hudText, scale)
		b.ColorText(suitColor[team(p)], "%s", m.playerName(p))
		b.End()
	}
}

// banner is the big centre text: the countdown, FIGHT, and the round and
// match results.
func (m *Arena) banner(b *ui.Builder, in *input.State) {
	mt := m.match
	var title, sub string
	colour := uiWhite
	if m.net != nil && m.net.over != "" {
		b.Panel("##banner", 0.5, 0.3, hudText, 3)
		b.ColorText(uiAccent, "MATCH OVER")
		b.End()
		b.Panel("##bannersub", 0.5, 0.38, hudText, 1.5)
		b.ColorText(uiMuted, "%s", m.net.over)
		b.End()
		b.Panel("##rematch", 0.5, 0.45, hudText, 1.2)
		b.ColorText(uiWhite, "%s", prompt(in, "Enter / click  main menu", "A  main menu", "Tap  main menu"))
		b.End()
		return
	}
	switch mt.Phase {
	case arena.PhaseCountdown:
		secs := int(math.Ceil(float64(mt.Timer)))
		if m.choosingGadget() {
			m.gadgetChoiceUI(b, in, secs)
			return
		}
		title = fmt.Sprintf("%d", secs)
		sub = fmt.Sprintf("ROUND %d  ·  %s  ·  LAUNCHING IN", mt.Round, arena.GadgetNames[m.me().Gadget])
		if mt.Round == 3 {
			sub = fmt.Sprintf("FINAL ROUND  ·  %s  ·  LAUNCHING IN", arena.GadgetNames[m.me().Gadget])
		}
	case arena.PhaseFight:
		if mt.Timer > arena.RoundTime-0.8 {
			title, colour = "FIGHT", uiAccent
		}
	case arena.PhaseRoundOver:
		title, sub, colour = m.roundResult()
	case arena.PhaseMatchOver:
		if mt.Winner == local {
			title, colour = "VICTORY", uiAccent
		} else {
			title, colour = "DEFEAT", hurtColor
		}
		sub = fmt.Sprintf("%d : %d", mt.Wins[0], mt.Wins[1])
	}
	if title == "" {
		return
	}
	b.Panel("##banner", 0.5, 0.3, hudText, 4)
	b.ColorText(colour, "%s", title)
	b.End()
	if sub != "" {
		b.Panel("##bannersub", 0.5, 0.38, hudText, 1.5)
		b.ColorText(uiMuted, "%s", sub)
		b.End()
	}
	if mt.Phase == arena.PhaseMatchOver && mt.Timer < -1 {
		b.Panel("##rematch", 0.5, 0.45, hudText, 1.2)
		switch {
		case m.net != nil && !m.net.host:
			b.ColorText(uiWhite, "waiting for the host to start a rematch   ·   Esc  menu")
		default:
			b.ColorText(uiWhite, "%s", prompt(in, "Enter / click  rematch     Esc  menu", "A  rematch     Start  menu", "Tap  rematch      II  menu"))
		}
		b.End()
	}
}

// roundResult describes how the last round ended.
func (m *Arena) roundResult() (title, sub string, colour [4]float32) {
	mt, s := m.match, m.sim()
	switch mt.RoundWinner {
	case local:
		title, colour = "ROUND WON", uiAccent
	case -1:
		title, colour = "DRAW", uiWhite
	default:
		title, colour = "ROUND LOST", hurtColor
	}
	if s.Alive() == len(s.Players) {
		return title, "time's up: the healthier player takes it", colour
	}
	for _, p := range s.Players {
		if p.Dead && p.ID == local {
			return title, "you were taken down", colour
		}
	}
	if m.net != nil {
		return title, "everyone else is down", colour
	}
	return title, "the bot is down", colour
}

// ammoText is the magazine, big, and the reserve beside it.
func ammoText(b *ui.Builder, ammo, reserve int, reloading bool) {
	if ammo == 0 && !reloading {
		b.ColorText(uiAccent, "%d", ammo)
	} else {
		b.Text("%d", ammo)
	}
	b.SameLine(10)
	b.ColorText(uiMuted, "| %d", reserve)
}

// rangeHUD is the firing range's panel: what's under the crosshair and how
// far, and how you're shooting.
func (m *Arena) rangeHUD(b *ui.Builder) {
	s, me := m.sim(), m.me()
	b.Panel("##range", m.leftX(), 0.075, hudText&^gfx.UICentered, 1.3)
	b.ColorText(paintColor[0], "FIRING RANGE")
	b.End()
	b.Panel("##rangeinfo", m.leftX(), 0.125, hudText&^gfx.UICentered, 0.95)
	what := "--"
	if !me.Dead {
		shot := s.Trace(me, me.Eye(1), me.Forward(), 300)
		dist := shot.To.Sub(shot.From).Len()
		switch {
		case shot.Victim != nil && shot.Head:
			what = fmt.Sprintf("DUMMY · HEAD · %.0f m", dist)
		case shot.Victim != nil:
			what = fmt.Sprintf("DUMMY · %.0f m", dist)
		case shot.Normal != (mathx.Vec3{}):
			what = fmt.Sprintf("%.0f m", dist)
		}
	}
	b.ColorText(uiMuted, "%s   ·   accuracy %.0f%%   headshots %d   downed %d", what, me.Accuracy()*100, me.Headshots, me.Kills)
	b.End()
}

// DebugUI is the F1 window.
func (m *Arena) DebugUI(b *ui.Builder, st Stats) {
	s, me := m.sim(), m.me()
	b.Window("Arena", 12, 12)
	b.Text("%.0f fps  %.2f ms  %d draws", st.FPS, st.FrameMS, st.Draws)
	b.Text("bodies %d  debris %d  bursts %d", len(s.Phys.Bodies()), len(s.Debris), len(m.bursts))
	b.Text("site %.0f%% standing  ·  you destroyed %d, bot %d", s.Standing()*100, me.Destroyed, s.Players[1].Destroyed)
	b.Text("you: accuracy %.0f%%  damage %.0f  ·  bot: accuracy %.0f%%  damage %.0f",
		me.Accuracy()*100, me.Damage, s.Players[1].Accuracy()*100, s.Players[1].Damage)
	b.Text("pos %.1f %.1f %.1f  ground %v", me.Body.Position[0], me.Body.Position[1], me.Body.Position[2], me.OnGround())
	b.Text("bot skill: %s", botSkills[m.skill].name)
	for i, sk := range botSkills {
		if i > 0 {
			b.SameLine(6)
		}
		if b.Button(sk.name) {
			m.skill = i
			m.bots[1].Skill = sk.skill
		}
	}
	b.Checkbox("bot holds fire", &m.bots[1].Passive)
	b.Checkbox("infinite ammo", &s.InfiniteAmmo)
	b.Checkbox("autopilot", &m.Autopilot)
	if b.Button("new match") {
		m.restart()
	}
	b.End()
}

// hudX maps x across the HUD box (0 its left edge, 1 its right) to the
// screen: the box is centred, as wide as the HUD width setting.
func (m *Arena) hudX(x float32) float32 { return 0.5 + (x-0.5)*m.settings.hudBox() }

// leftX is where the top-left panels start: moved right, clear of the
// on-screen pause button, while the touch controls are up.
func (m *Arena) leftX() float32 {
	if m.touchOn {
		return max(m.hudX(0.035), 0.13)
	}
	return m.hudX(0.035)
}
