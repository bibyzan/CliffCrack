package arena

import (
	"testing"

	"CliffCrack/engine/mathx"
)

func TestElbowIsALightBlow(t *testing.T) {
	a, attacker, target := duel(1.6)
	attacker.Pitch = -0.1
	ev := run(a, frame, Input{Melee: true})
	ev.Merge(run(a, ElbowTime))
	if !ev.Did(attacker, ActElbow) {
		t.Fatal("melee should throw an elbow")
	}
	if len(ev.Smashes) != 1 || ev.Smashes[0].Victim != target || !ev.Smashes[0].Light {
		t.Fatalf("the elbow should land on the player: %+v", ev.Smashes)
	}
	took := MaxShield + MaxHealth - target.Durability()
	if took != ElbowDamage || took >= HammerPlayerDamage {
		t.Errorf("an elbow took %v; want %v, well under the hammer's %v", took, float32(ElbowDamage), float32(HammerPlayerDamage))
	}
	if attacker.HammerOut || attacker.Holding() != WeaponRifle {
		t.Error("an elbow shouldn't bring out the hammer")
	}
}

func TestElbowIsShortRanged(t *testing.T) {
	a, _, target := duel(3)
	ev := run(a, frame, Input{Melee: true})
	ev.Merge(run(a, ElbowTime))
	if target.Durability() != MaxShield+MaxHealth {
		t.Errorf("an elbow reached a player 3 m off: %+v", ev.Smashes)
	}
}

func TestHammerGadgetTradesTheGun(t *testing.T) {
	a, p, _ := duel(10)
	p.Gadget = GadgetHammer
	run(a, frame, Input{Gadget: true})
	run(a, hammerDraw)
	if !p.HammerOut || p.Holding() != WeaponHammer {
		t.Fatal("the gadget button should bring out the hammer")
	}
	if ev := run(a, frame, Input{Fire: true, FirePressed: true, Aim: true}); len(ev.Shots) != 0 || !ev.Did(p, ActSwing) {
		t.Errorf("with the hammer out the trigger swings it: shots %d, swung %v", len(ev.Shots), ev.Did(p, ActSwing))
	}
	run(a, HammerSwing)
	if p.ADS != 0 {
		t.Error("no sights with the hammer out")
	}
	run(a, frame, Input{Gadget: true})
	run(a, SwitchTime)
	if p.HammerOut {
		t.Fatal("the gadget button again should put it away")
	}
	if ev := run(a, frame, Input{Fire: true, FirePressed: true}); len(ev.Shots) == 0 {
		t.Error("the gun should fire again once the hammer's away")
	}
}

func TestGrapplePullsYouIn(t *testing.T) {
	a, p, _ := duel(30)
	p.Gadget = GadgetGrapple
	p.Pitch = -0.09 // at the ground well off
	start := p.Body.Position
	ev := run(a, frame, Input{Gadget: true})
	if !p.Grapple.On || !ev.Did(p, ActGrapple) {
		t.Fatalf("the hook should catch the ground ahead: %+v", p.Grapple)
	}
	anchor := p.Grapple.To
	far := anchor.Sub(start).Len()
	var top float32
	for i := 0; i < int(GrappleTime/frame)+10 && p.Grapple.On; i++ {
		run(a, frame, Input{})
		top = max(top, p.Body.Velocity.Len())
	}
	near := anchor.Sub(p.Body.Position).Len()
	t.Logf("hooked %.1f m away; pulled to %.1f m, top speed %.1f m/s", far, near, top)
	if far < 12 {
		t.Fatalf("the hook caught only %.1f m off: aim further", far)
	}
	if near > far*0.5 {
		t.Errorf("the grapple should pull you most of the way: %.1f m of %.1f left", near, far)
	}
	if top < 12 {
		t.Errorf("a grapple should be fast: top speed %.1f m/s", top)
	}
	if p.Grapple.On || p.Grapple.Cooldown <= 0 {
		t.Error("it should let go on arrival and recharge")
	}
	if ev := run(a, frame, Input{Gadget: true}); ev.Did(p, ActGrapple) {
		t.Error("it fired again while recharging")
	}
}

func TestGrappleLetsGoOnJumpAndMissesTheSky(t *testing.T) {
	a, p, _ := duel(30)
	p.Gadget = GadgetGrapple
	p.Pitch = -0.35
	run(a, frame, Input{Gadget: true})
	run(a, 0.2)
	run(a, frame, Input{Jump: true})
	if p.Grapple.On {
		t.Error("jumping should let go")
	}
	v := p.Body.Velocity
	if v.Len() < 5 {
		t.Errorf("letting go keeps the momentum: %.1f m/s", v.Len())
	}

	a, p, _ = duel(30)
	p.Gadget = GadgetGrapple
	p.Pitch = 1.2 // at the sky
	ev := run(a, frame, Input{Gadget: true})
	if p.Grapple.On || !p.Grapple.Miss || !ev.Did(p, ActGrapple) {
		t.Errorf("into the sky it should miss: %+v", p.Grapple)
	}
	if p.Grapple.Cooldown <= 0 || p.Grapple.Cooldown > GrappleCooldown {
		t.Errorf("a miss waits a moment: %v", p.Grapple.Cooldown)
	}
}

func TestGadgetIsChosenInTheCountdown(t *testing.T) {
	m := NewMatch(3, 2)
	if m.Phase != PhaseCountdown || m.Timer != CountdownTime {
		t.Fatalf("a round opens with the countdown: %v %v", m.Phase, m.Timer)
	}
	m.Step(frame, []Input{{Select: 2}, {}})
	if got := m.Arena.Players[0].Gadget; got != GadgetGrapple {
		t.Fatalf("2 in the countdown should pick the grapple, got %v", got)
	}
	if p := m.Arena.Players[0]; p.Current != WeaponRifle {
		t.Error("in the countdown the number keys pick a gadget, not a weapon")
	}
	m.Step(frame, []Input{{Gadget: true}, {}})
	if got := m.Arena.Players[0].Gadget; got != GadgetHammer {
		t.Errorf("the gadget button should step to the next: %v", got)
	}
	m.Step(frame, []Input{{Select: 2}, {}})
	// Through the countdown, a round and into the next: the choice sticks.
	for m.Round == 1 {
		m.Arena.Players[1].Health, m.Arena.Players[1].Shield = 0, 0
		if m.Phase == PhaseFight {
			m.Arena.hurtPlayer(m.Arena.Players[1], nil, 1, false, WeaponDrop, mathx.Vec3{}, mathx.Vec3{}, &Events{})
		}
		m.Step(0.1, []Input{{}, {}})
	}
	if got := m.Arena.Players[0].Gadget; got != GadgetGrapple {
		t.Errorf("the next round should keep the grapple, got %v", got)
	}
	if m.Phase == PhaseCountdown {
		m.Step(frame, []Input{{Select: 1}, {}})
	}
	if m.Arena.Players[0].Gadget != GadgetHammer {
		t.Error("it can be changed again in the next countdown")
	}
}

// On the range, the gadgets lie on the table: take one with Interact.
func TestGadgetsOnTheRangeTable(t *testing.T) {
	m := NewRange()
	a, p := m.Arena, m.Arena.Players[0]
	var grapple *Pickup
	for _, pk := range a.Pickups {
		if pk.IsGadget && pk.Gadget == GadgetGrapple {
			grapple = pk
		}
	}
	if grapple == nil {
		t.Fatal("no grapple on the range table")
	}
	p.Body.Position = grapple.At.Add(mathx.Vec3{0, PlayerRadius + 0.05, 0.6})
	p.Body.Teleported()
	if got := a.NearestPickup(p); got != grapple {
		t.Fatalf("standing by the grapple, the nearest pickup is %+v", got)
	}
	m.Step(frame, []Input{{Interact: true}})
	if p.Gadget != GadgetGrapple {
		t.Errorf("took the grapple: gadget %v", p.Gadget)
	}
	if p.Current != WeaponRifle || !p.Holds(WeaponPistol) {
		t.Error("taking a gadget shouldn't cost a weapon")
	}
}
