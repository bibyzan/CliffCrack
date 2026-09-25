package game

import (
	"fmt"
	"math"

	"CliffCrack/engine/gfx"
	"CliffCrack/engine/input"
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

	if !me.Dead {
		b.Panel("##crosshair", 0.5, 0.5, hudText, 1.5)
		switch {
		case m.hitMark > 0 && m.headMark:
			b.ColorText(hurtColor, "X")
		case m.hitMark > 0:
			b.ColorText(uiAccent, "X")
		default:
			b.ColorText(withAlpha(uiWhite, 0.9), "+")
		}
		b.End()
	}
	m.nameTags(b)

	// Health, bottom left.
	b.Panel("##health", 0.02, 0.975, anchored, 2.4)
	col := uiWhite
	if me.Health < arena.MaxHealth*0.35 {
		col = hurtColor
	}
	b.ColorText(col, "%d", int(math.Ceil(float64(me.Health))))
	b.SameLine(8)
	b.ColorText(uiMuted, "HP")
	b.Progress("", me.Health/arena.MaxHealth, 150, 6)
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
		m.weaponHUD(b, me)
	}
	m.banner(b, in)

	if m.elapsed < arenaHintTime && !m.Autopilot && mt.Round == 1 {
		fade := clampf((arenaHintTime-m.elapsed)*2, 0, 1)
		b.Panel("##arenahint", 0.5, 0.975, hudText, 1.05)
		b.ColorText(withAlpha(uiMuted, fade), "%s", prompt(in,
			"WASD  move    Mouse  aim    LMB  fire / swing    1 2 3 / wheel  weapon    R  reload    Space  jump    Shift  sprint",
			"L-stick  move    R-stick  aim    RT  fire / swing    LB RB Y  weapon    X  reload    A  jump    L3  sprint"))
		b.End()
	}
}

// weaponHUD is the weapon bar (bottom centre), the ammo count (bottom
// right) and the reload bar.
func (m *Arena) weaponHUD(b *ui.Builder, me *arena.Player) {
	anchored := hudText &^ gfx.UICentered
	b.Panel("##weapons", 0.5, 0.905, anchored, 1.15)
	for i, name := range arena.WeaponNames {
		if i > 0 {
			b.SameLine(28)
		}
		if arena.WeaponKind(i) == me.Current {
			b.ColorText(uiAccent, "%d %s", i+1, name)
		} else {
			b.ColorText(withAlpha(uiMuted, 0.7), "%d %s", i+1, name)
		}
	}
	b.End()

	b.Panel("##ammo", 0.985, 0.975, anchored, 2.8)
	switch me.Current {
	case arena.WeaponHammer:
		b.ColorText(uiMuted, "--")
	case arena.WeaponRifle:
		ammoText(b, me.Rifle.Ammo, arena.MagSize, me.Rifle.Reloading > 0)
	case arena.WeaponLauncher:
		ammoText(b, me.Launcher.Ammo, arena.LauncherMag, me.Launcher.Reloading > 0)
	}
	b.End()
	if t, ok := me.Reloading(); ok {
		b.Panel("##reload", 0.5, 0.64, hudText, 1.2)
		b.ColorText(uiAccent, "RELOADING")
		b.Progress("", t, 220, 6)
		b.End()
	}
}

// nameTags labels the other players you can see, with their health.
func (m *Arena) nameTags(b *ui.Builder) {
	s, me := m.sim(), m.me()
	eye := m.eye()
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
		b.ColorText(suitColor[p.ID%len(suitColor)], "%s", playerName(p))
		b.Progress("", p.Health/arena.MaxHealth, 70, 4)
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
		sub = fmt.Sprintf("ROUND %d", mt.Round)
		if mt.Round == 3 {
			sub = "FINAL ROUND"
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

func ammoText(b *ui.Builder, ammo, mag int, reloading bool) {
	if ammo == 0 && !reloading {
		b.ColorText(uiAccent, "%d", ammo)
	} else {
		b.Text("%d", ammo)
	}
	b.SameLine(10)
	b.ColorText(uiMuted, "/ %d", mag)
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
