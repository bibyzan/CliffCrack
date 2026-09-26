package game

import (
	"math"

	"CliffCrack/engine/input"
	"CliffCrack/game/arena"
)

// arenaTouch is the Arena's layout, after phone shooters. The left thumb
// has the stick (pushed all the way forward, it sprints). The right thumb
// rests on FIRE, low in the corner, with AIM beside it towards the edge and
// JUMP below that; the rest keeps to the edges, where you don't look: RELOAD
// and SWAP low along the bottom, HAMMER and the grenades up the right side.
// Everything between is for looking around, and so is FIRE itself as it
// drags, so you can aim while you shoot. Pause is top left.
//
//	                   NADE
//	             FRAG
//	                   HAMMER
//	       (PICK UP)
//	            FIRE   AIM
//	SWAP  RELOAD
//	                   JUMP
//
// The buttons show what they'll use: FIRE the gun in hand, SWAP the other
// one, NADE the grenade that'll be thrown, and the small button beside it
// the other kind to switch to. PICK UP appears, showing what's there, when
// there's something at your feet.
type arenaTouch struct {
	touchControls
	fire, jump, aim, reload, swap touchButton
	melee, throw, kind, pickUp    touchButton
	pause                         touchButton

	aiming bool // AIM toggles the sights
}

const (
	arenaStickDead  = 0.12 // radial: the stick moves nothing below this
	arenaSprintPush = 0.92 // pushed this far forward (and not much sideways), it sprints
)

// cornerButton is a button centred x in from the right edge and y up from
// the bottom, r its radius (all in screen heights).
func cornerButton(label string, x, y, r float32, color [4]float32) touchButton {
	return touchButton{label: label, x: -x, y: -y, r: r, color: color}
}

func newArenaTouch() *arenaTouch {
	icons := getTouchIcons()
	t := &arenaTouch{
		fire:   cornerButton("FIRE", 0.32, 0.38, 0.092, touchRed),
		aim:    cornerButton("AIM", 0.12, 0.34, 0.07, touchBack),
		jump:   cornerButton("JUMP", 0.11, 0.13, 0.068, touchOrange),
		reload: cornerButton("RELOAD", 0.62, 0.12, 0.058, touchBack),
		swap:   cornerButton("SWAP", 0.78, 0.10, 0.055, touchBack),
		melee:  cornerButton("HAMMER", 0.11, 0.54, 0.055, touchBack),
		throw:  cornerButton("NADE", 0.11, 0.70, 0.055, touchBack),
		kind:   cornerButton("FRAG", 0.235, 0.74, 0.04, touchBack),
		pickUp: cornerButton("PICK UP", 0.38, 0.59, 0.065, touchOrange),
		pause:  touchButton{label: "II", icon: icons.pause, x: 0.14, y: 0.14, r: 0.05, color: touchBack},
	}
	// FIRE and AIM turn the camera as they drag: tap AIM and keep going to
	// aim in one stroke, and shoot while you track.
	t.fire.look, t.aim.look = true, true
	t.jump.icon, t.aim.icon, t.reload.icon = icons.jump, icons.aim, icons.reload
	t.buttons = []*touchButton{&t.fire, &t.jump, &t.aim, &t.reload, &t.swap,
		&t.melee, &t.throw, &t.kind, &t.pickUp, &t.pause}
	return t
}

// release lets go of everything, the sights included.
func (t *arenaTouch) release() {
	t.touchControls.release()
	t.aiming = false
}

// show sets the buttons' icons from your loadout (ic: the HUD's icons) and
// hides the ones with nothing to do: SWAP with one gun, PICK UP with
// nothing there.
func (t *arenaTouch) show(me *arena.Player, pickup *arena.Pickup, ic *hudIcons) {
	weapon := func(k arena.WeaponKind) icon {
		if k < 0 || int(k) >= len(ic.weapons) {
			return icon{}
		}
		return ic.weapons[k]
	}
	grenade := func(k arena.GrenadeKind) icon {
		if k == arena.Sticky {
			return ic.sticky
		}
		return ic.frag
	}
	t.fire.icon = weapon(me.Current)
	t.swap.icon = weapon(me.Other())
	t.swap.hidden = me.Other() == arena.NoWeapon
	t.melee.icon = weapon(arena.WeaponHammer)
	t.throw.icon = grenade(me.GrenadeKind)
	t.kind.icon = grenade((me.GrenadeKind + 1) % arena.GrenadeKinds)
	t.pickUp.hidden = pickup == nil
	if pickup != nil {
		if pickup.Weapon != arena.NoWeapon {
			t.pickUp.icon = weapon(pickup.Weapon)
		} else {
			t.pickUp.icon = grenade(pickup.Grenade)
		}
	}
}

// read puts this frame's presses into c (the caller merges them with the
// keyboard's and pad's, see mergeTouch) and returns the look, in radians
// (yaw right, pitch up), and whether pause was tapped. The caller scales the
// look for sensitivity and zoom.
func (t *arenaTouch) read(in *input.State, w, h float32, me *arena.Player, c *arena.Input) (yaw, pitch float32, pause bool) {
	f := t.update(in, w, h)

	// Move: a radial deadzone, rescaled so the edge of it is a crawl.
	x, y := f.stickX, -f.stickY
	if l := float32(math.Hypot(float64(x), float64(y))); l > arenaStickDead {
		s := min(1, (l-arenaStickDead)/(1-arenaStickDead)) / l
		c.Move[0] += x * s
		c.Move[1] += y * s
		if y >= arenaSprintPush && math.Abs(float64(x)) < 0.4 {
			c.Sprint = true
		}
	}

	if t.aim.pressed {
		t.aiming = !t.aiming
	}
	if me.Dead || t.swap.pressed || t.reload.pressed {
		t.aiming = false // the sights drop anyway; don't bring them back up after
	}
	t.aim.lit = t.aiming
	c.Aim = c.Aim || t.aiming
	c.Fire = c.Fire || t.fire.down
	c.FirePressed = c.FirePressed || t.fire.pressed
	c.Jump = c.Jump || t.jump.pressed
	c.Reload = c.Reload || t.reload.pressed
	c.Melee = c.Melee || t.melee.pressed
	c.Throw = c.Throw || t.throw.pressed
	c.SwitchGrenade = c.SwitchGrenade || t.kind.pressed
	c.Interact = c.Interact || t.pickUp.pressed
	if t.swap.pressed {
		c.Cycle = 1
	}
	return f.yaw, -f.elev, t.pause.pressed
}
