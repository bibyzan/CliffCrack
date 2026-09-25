package game

import (
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/input"
	"CliffCrack/engine/ui"
	"CliffCrack/game/arena"
)

const arenaHintTime = 8.0 // seconds the controls hint stays up

// UI draws the HUD: crosshair and hit marker, score, ammo and reload, the
// kill feed and (for the first few seconds) the controls.
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
	b.ColorText(uiMuted, "kills %d  ·  accuracy %.0f%%  ·  drones %d/%d",
		s.Kills, s.Accuracy()*100, s.Alive(), len(s.Drones))
	b.End()

	for i, f := range m.feed {
		fade := clampf((feedLife-f.age)*3, 0, 1)
		b.Panel("##feed"+string(rune('0'+i%10)), 0.5, 0.58+0.04*float32(i), hudText, 1.3)
		b.ColorText(withAlpha(uiAccent, fade), "%s", f.text)
		b.End()
	}

	// Ammo, bottom right; the anchored window's right edge sits on the point.
	w := &s.Weapon
	b.Panel("##ammo", 0.985, 0.975, hudText&^gfx.UICentered, 2.8)
	if w.Ammo == 0 && w.Reloading == 0 {
		b.ColorText(uiAccent, "%d", w.Ammo)
	} else {
		b.Text("%d", w.Ammo)
	}
	b.SameLine(10)
	b.ColorText(uiMuted, "/ %d", arena.MagSize)
	b.End()
	if w.Reloading > 0 {
		b.Panel("##reload", 0.5, 0.64, hudText, 1.2)
		b.ColorText(uiAccent, "RELOADING")
		b.Progress("", 1-w.Reloading/arena.ReloadTime, 220, 6)
		b.End()
	}

	if m.elapsed < arenaHintTime && !m.Autopilot {
		fade := clampf((arenaHintTime-m.elapsed)*2, 0, 1)
		b.Panel("##arenahint", 0.5, 0.975, hudText, 1.1)
		b.ColorText(withAlpha(uiMuted, fade), "%s", prompt(in,
			"WASD  move      Mouse  aim      LMB  fire      R  reload      Space  jump      Shift  sprint      Esc  pause",
			"L-stick  move      R-stick  aim      RT  fire      X  reload      A  jump      L3  sprint      Start  pause"))
		b.End()
	}
}

// DebugUI is the F1 window.
func (m *Arena) DebugUI(b *ui.Builder, st Stats) {
	s := m.sim
	p := &s.Player
	b.Window("Arena", 12, 12)
	b.Text("%.0f fps  %.2f ms  %d draws", st.FPS, st.FrameMS, st.Draws)
	b.Text("bodies %d  debris %d  tracers %d", len(s.Phys.Bodies()), len(s.Debris), len(m.tracers))
	b.Text("pos %.1f %.1f %.1f  ground %v", p.Body.Position[0], p.Body.Position[1], p.Body.Position[2], p.OnGround())
	b.Checkbox("infinite ammo", &s.InfiniteAmmo)
	b.Checkbox("autopilot", &m.Autopilot)
	if b.Button("restart match") {
		m.start()
	}
	b.End()
}
