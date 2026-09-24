package game

import (
	"fmt"
	"strings"

	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/ui"
)

// The interface palette (linear RGBA), matching the renderer's UI theme.
var (
	uiAccent = mathx.Hex(0xf5842a) // the ball's orange
	uiHot    = mathx.Hex(0xff4d2e)
	uiMuted  = mathx.SRGB(0.86, 0.88, 0.96, 0.85)
	uiWhite  = [4]float32{1, 1, 1, 1}
)

// Transparent, centred, pinned text: headline and HUD elements.
const hudText = gfx.UIOverlay | gfx.UIAnchored | gfx.UINoBackground | gfx.UICentered

// Card is a pinned, centred panel with a background.
const card = gfx.UIOverlay | gfx.UIAnchored | gfx.UICentered

// zoneLength is how often the run announces a new zone (metres).
const zoneLength = 500

var zoneNames = []string{
	"The Drop", "Pine Line", "Boulder Field", "The Narrows",
	"Crevasse Run", "Avalanche Alley", "Knife Edge", "The Abyss",
}

// zoneName names zone i (0-based); past the named ones they just count on.
func zoneName(i int) string {
	if i < len(zoneNames) {
		return zoneNames[i]
	}
	return fmt.Sprintf("Deep Descent %d", i-len(zoneNames)+1)
}

// withAlpha scales a colour's alpha (for fading text).
func withAlpha(c [4]float32, a float32) [4]float32 {
	c[3] *= clampf(a, 0, 1)
	return c
}

// speedColor runs from white through orange to red as the ride gets fast.
func speedColor(kmh float32) [4]float32 {
	lerp := func(a, b [4]float32, t float32) [4]float32 {
		t = clampf(t, 0, 1)
		return [4]float32{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t, 1}
	}
	if kmh < 110 {
		return lerp(uiWhite, uiAccent, (kmh-70)/40)
	}
	return lerp(uiAccent, uiHot, (kmh-110)/60)
}

// UI draws the HUD, zone banners and the game-over card.
func (r *Run) UI(b *ui.Builder) {
	if r.attract {
		return
	}
	b.Panel("##distance", 0.5, 0.025, hudText, 3.2)
	b.Text("%.0f m", r.ride.distance)
	b.End()

	if !r.ride.crashed {
		kmh := r.ride.speed() * 3.6
		b.Panel("##speed", 0.5, 0.125, hudText, 1.25)
		b.ColorText(speedColor(kmh), "%.0f km/h", kmh)
		b.Progress("", kmh/180, 200, 6)
		b.End()
	}
	if r.best > 0 {
		b.Panel("##best", 0.985, 0.025, hudText, 1.2)
		b.ColorText(uiMuted, "BEST")
		b.Text("%.0f m", r.best)
		b.End()
	}
	if t := r.zoneTime; t < 3 && r.zone >= 0 {
		fade := min(t*4, (3-t)*2) // in quickly, out slowly
		b.Panel("##zone", 0.5, 0.26, hudText, 1.4)
		b.ColorText(withAlpha(uiAccent, fade), "ZONE %d", r.zone+1)
		b.End()
		b.Panel("##zonename", 0.5, 0.31, hudText, 3)
		b.ColorText(withAlpha(uiWhite, fade), "%s", strings.ToUpper(zoneName(r.zone)))
		b.End()
	}
	if r.ride.time < hintTime && !r.ride.crashed {
		b.Panel("##hint", 0.5, 0.975, hudText, 1.1)
		b.ColorText(uiMuted, "A / D  steer      W  tuck      S  brake      Space  jump      Esc  menu")
		b.End()
	}
	if !r.ride.crashed || r.overTime < overDelay {
		return
	}

	b.Panel("##wipeout", 0.5, 0.24, hudText, 5)
	b.ColorText(uiAccent, "WIPEOUT")
	b.End()
	b.Panel("##over", 0.5, 0.6, card, 1.35)
	b.ColorText(uiMuted, "%s", strings.ToUpper("you "+r.ride.cause))
	b.Text("%.0f m  ·  %s", r.ride.distance, zoneName(max(r.zone, 0)))
	if r.newBest {
		b.ColorText(uiAccent, "NEW BEST!")
	} else {
		b.ColorText(uiMuted, "best  %.0f m", r.best)
	}
	b.Separator()
	if b.MenuButton("Ride again", 300, 46, r.choice == 0) {
		r.retry = true
	}
	if b.MenuButton("Main menu", 300, 46, r.choice == 1) {
		r.wantsMenu = true
	}
	b.ColorText(withAlpha(uiMuted, 0.8), "Enter  choose      R  ride again")
	b.End()
}

// ui draws the title screen and reports a clicked item.
func (m *menu) ui(b *ui.Builder, best float32) (Mode, bool) {
	// One line: the anchored window centres itself (per-item centring would
	// centre "CLIFF" alone and hang "CRACK" off its end).
	b.Panel("##title", 0.5, 0.2, hudText&^gfx.UICentered, 5.5)
	b.Text("CLIFF")
	b.SameLine(24)
	b.ColorText(uiAccent, "CRACK")
	b.End()
	b.Panel("##tagline", 0.5, 0.335, hudText, 1.5)
	b.ColorText(uiMuted, "drop in  ·  ride down  ·  don't hit anything")
	b.End()

	chosen, clicked := Mode(0), false
	b.Panel("##menu", 0.5, 0.76, card, 1.5)
	for i, item := range menuItems {
		if b.MenuButton(item.label, 320, 50, i == m.choice) {
			chosen, clicked = item.mode, true
		}
	}
	if best > 0 {
		b.Separator()
		b.ColorText(uiMuted, "best run  %.0f m", best)
	}
	b.End()

	b.Panel("##keys", 0.5, 0.975, hudText, 1.05)
	b.ColorText(uiMuted, "W / S  choose      Enter  select      Esc  quit")
	b.End()
	return chosen, clicked
}
