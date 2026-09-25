package game

import (
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/input"
	"CliffCrack/engine/ui"
	"CliffCrack/game/arena"
)

const arenaHintTime = 8.0 // seconds the controls hint stays up

// UI draws the HUD: crosshair and hit marker, score, the weapon bar with
// ammo and reload, the kill feed, a destruction meter in Demolition and (for
// the first few seconds) the controls.
func (m *Arena) UI(b *ui.Builder, in *input.State) {
	s := m.sim
	b.Panel("##crosshair", 0.5, 0.5, hudText, 1.5)
	if m.hitMark > 0 {
		b.ColorText(uiAccent, "X")
	} else {
		b.ColorText(withAlpha(uiWhite, 0.9), "+")
	}
	b.End()

	b.Panel("##score", 0.5, 0.025, hudText, 2.6)
	b.Text("%d", s.Score)
	b.End()
	b.Panel("##stats", 0.5, 0.1, hudText, 1.1)
	if m.site {
		b.ColorText(uiMuted, "destroyed %d  ·  site %.0f%% standing  ·  drones %d/%d",
			s.Destroyed, s.Standing()*100, s.Alive(), len(s.Drones))
	} else {
		b.ColorText(uiMuted, "kills %d  ·  accuracy %.0f%%  ·  drones %d/%d",
			s.Kills, s.Accuracy()*100, s.Alive(), len(s.Drones))
	}
	b.End()
	if m.site {
		b.Panel("##demolished", 0.5, 0.14, hudText, 1)
		b.Progress("", 1-s.Standing(), 260, 5)
		b.End()
	}

	for i, f := range m.feed {
		fade := clampf((feedLife-f.age)*3, 0, 1)
		b.Panel("##feed"+string(rune('0'+i%10)), 0.5, 0.58+0.04*float32(i), hudText, 1.3)
		b.ColorText(withAlpha(uiAccent, fade), "%s", f.text)
		b.End()
	}

	// Weapon bar, bottom centre: the current slot highlighted.
	b.Panel("##weapons", 0.5, 0.905, hudText&^gfx.UICentered, 1.15)
	for i, name := range arena.WeaponNames {
		if i > 0 {
			b.SameLine(28)
		}
		if arena.WeaponKind(i) == s.Current {
			b.ColorText(uiAccent, "%d %s", i+1, name)
		} else {
			b.ColorText(withAlpha(uiMuted, 0.7), "%d %s", i+1, name)
		}
	}
	b.End()

	// Ammo, bottom right; the anchored window's right edge sits on the point.
	b.Panel("##ammo", 0.985, 0.975, hudText&^gfx.UICentered, 2.8)
	switch s.Current {
	case arena.WeaponHammer:
		b.ColorText(uiMuted, "--")
	case arena.WeaponRifle:
		ammoText(b, s.Rifle.Ammo, arena.MagSize, s.Rifle.Reloading > 0)
	case arena.WeaponLauncher:
		ammoText(b, s.Launcher.Ammo, arena.LauncherMag, s.Launcher.Reloading > 0)
	}
	b.End()
	if t, ok := s.Reloading(); ok {
		b.Panel("##reload", 0.5, 0.64, hudText, 1.2)
		b.ColorText(uiAccent, "RELOADING")
		b.Progress("", t, 220, 6)
		b.End()
	}

	if m.elapsed < arenaHintTime && !m.Autopilot {
		fade := clampf((arenaHintTime-m.elapsed)*2, 0, 1)
		b.Panel("##arenahint", 0.5, 0.975, hudText, 1.05)
		b.ColorText(withAlpha(uiMuted, fade), "%s", prompt(in,
			"WASD  move    Mouse  aim    LMB  fire / swing    1 2 3 / wheel  weapon    R  reload    Space  jump    Shift  sprint",
			"L-stick  move    R-stick  aim    RT  fire / swing    LB RB Y  weapon    X  reload    A  jump    L3  sprint"))
		b.End()
	}
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
	s := m.sim
	p := &s.Player
	b.Window("Arena", 12, 12)
	b.Text("%.0f fps  %.2f ms  %d draws", st.FPS, st.FrameMS, st.Draws)
	b.Text("bodies %d  debris %d  bursts %d", len(s.Phys.Bodies()), len(s.Debris), len(m.bursts))
	chunks := 0
	for _, st := range s.Structures {
		chunks += st.Alive()
	}
	b.Text("structures %d  chunks standing %d", len(s.Structures), chunks)
	b.Text("pos %.1f %.1f %.1f  ground %v", p.Body.Position[0], p.Body.Position[1], p.Body.Position[2], p.OnGround())
	b.Checkbox("infinite ammo", &s.InfiniteAmmo)
	b.Checkbox("autopilot", &m.Autopilot)
	label := "restart match"
	if m.site {
		label = "new site"
	}
	if b.Button(label) {
		m.restart()
	}
	b.End()
}
