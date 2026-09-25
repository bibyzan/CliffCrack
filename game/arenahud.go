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

	if mt.Practice {
		m.rangeHUD(b)
	} else {
		// Score: YOU 1 : 0 BOT, the round and its clock.
		b.Panel("##score", 0.5, 0.02, anchored, 1.7)
		b.ColorText(suitColor[0], "YOU")
		b.SameLine(14)
		b.Text("%d : %d", mt.Wins[0], mt.Wins[1])
		b.SameLine(14)
		b.ColorText(suitColor[1], "BOT")
		b.End()
		b.Panel("##round", 0.5, 0.085, hudText, 1.1)
		clock := ""
		if mt.Phase == arena.PhaseFight {
			t := int(math.Ceil(float64(mt.Timer)))
			clock = fmt.Sprintf("   ·   %d:%02d", t/60, t%60)
		}
		b.ColorText(uiMuted, "ROUND %d  ·  FIRST TO %d%s", mt.Round, arena.RoundsToWin, clock)
		b.End()
	}

	m.nameTags(b)

	// Armour, bottom left: a word, not a bar. With it gone, the word dims
	// red and your health shows under it.
	b.Panel("##armour", 0.02, 0.975, anchored, 2)
	switch {
	case me.Popped():
		pulse := 0.45 + 0.25*float32(math.Sin(float64(m.elapsed)*6))
		b.ColorText(withAlpha(hurtColor, pulse), "ARMOUR")
		b.Progress("", me.Health/arena.MaxHealth, 150, 6)
	case m.charging:
		b.ColorText(uiMuted, "ARMOUR")
	case me.Shield < arena.MaxShield*0.5:
		b.ColorText(uiAccent, "ARMOUR CRACKED")
	default:
		b.ColorText(uiWhite, "ARMOUR")
	}
	b.End()

	// Kill feed, top right.
	for i, f := range m.feed {
		fade := clampf((feedLife-f.age)*3, 0, 1)
		b.Panel("##feed"+string(rune('0'+i%10)), 0.985, 0.03+0.04*float32(i), anchored, 1.15)
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
		b.Panel("##arenahint", 0.5, 0.975, hudText, 1.05)
		b.ColorText(withAlpha(uiMuted, fade), "%s", prompt(in,
			"LMB  fire    RMB  aim    1 2 / wheel  swap    R  reload    E  pick up    F  hammer    G  grenade    Q  frag / sticky",
			"RT  fire    LT  aim    Y  swap    X  reload (hold: pick up)    RB  hammer    LB  grenade    B  frag / sticky"))
		b.End()
	}
}

// weaponHUD is the loadout, bottom right: the weapon in hand over the other,
// its magazine and reserve, and the grenades (the kind G throws lit). A
// weapon in reach gets a prompt to pick it up.
func (m *Arena) weaponHUD(b *ui.Builder, in *input.State, me *arena.Player) {
	anchored := hudText &^ gfx.UICentered
	b.Panel("##ammo", 0.985, 0.975, anchored, 2.8)
	switch g, s := me.Gun(); {
	case g != nil:
		ammoText(b, s.Ammo, s.Reserve, s.Reloading > 0)
	case me.Current == arena.WeaponLauncher:
		ammoText(b, me.Launcher.Ammo, me.Launcher.Reserve, me.Launcher.Reloading > 0)
	default:
		b.ColorText(uiMuted, "--")
	}
	b.End()

	b.Panel("##loadout", 0.985, 0.86, anchored, 1.2)
	b.ColorText(uiAccent, "%s", arena.WeaponNames[me.Current])
	if other := me.Other(); other != arena.NoWeapon {
		b.SameLine(16)
		b.ColorText(withAlpha(uiMuted, 0.75), "%s", arena.WeaponNames[other])
	}
	b.End()
	b.Panel("##grenades", 0.985, 0.815, anchored, 1.05)
	for k := range arena.GrenadeKinds {
		if k > 0 {
			b.SameLine(14)
		}
		col := withAlpha(uiMuted, 0.7)
		if k == me.GrenadeKind {
			col = uiWhite
		}
		if me.Grenades[k] == 0 {
			col = withAlpha(col, 0.35)
		}
		b.ColorText(col, "%s x%d", arena.GrenadeNames[k], me.Grenades[k])
	}
	b.End()

	if t, ok := me.Reloading(); ok {
		b.Panel("##reload", 0.5, 0.64, hudText, 1.2)
		b.ColorText(uiAccent, "RELOADING")
		b.Progress("", t, 220, 6)
		b.End()
	}
	if p := m.sim().NearestPickup(me); p != nil {
		b.Panel("##pickup", 0.5, 0.6, hudText, 1.15)
		verb := "pick up"
		if me.Other() != arena.NoWeapon {
			verb = "swap " + arena.WeaponNames[me.Current] + " for"
		}
		b.ColorText(uiWhite, "%s", prompt(in, "E  "+verb+" "+p.Name(), "hold X  "+verb+" "+p.Name()))
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
	switch mt.Phase {
	case arena.PhaseCountdown:
		title = fmt.Sprintf("%d", int(math.Ceil(float64(mt.Timer))))
		sub = fmt.Sprintf("ROUND %d  ·  LAUNCHING IN", mt.Round)
		if mt.Round == 3 {
			sub = "FINAL ROUND  ·  LAUNCHING IN"
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
		b.ColorText(uiWhite, "%s", prompt(in, "Enter / click  rematch     Esc  menu", "A  rematch     Start  menu"))
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
	b.Panel("##range", 0.5, 0.02, hudText&^gfx.UICentered, 1.5)
	b.ColorText(paintColor[0], "FIRING RANGE")
	b.End()
	b.Panel("##rangeinfo", 0.5, 0.08, hudText, 1.05)
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
