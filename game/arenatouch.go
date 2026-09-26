package game

import (
	"math"

	"CliffCrack/engine/input"
	"CliffCrack/game/arena"
)

// arenaTouch is the Arena's layout, after phone shooters: the stick moves
// (pushed all the way forward, it sprints), and the right thumb looks by
// dragging anywhere on the right half, FIRE included, so you can aim while
// you shoot. The buttons sit round the bottom-right corner, the ones you
// need most (fire, jump, aim) nearest the thumb:
//
//	HAMMER  NADE   FRAG/STICKY
//	 AIM    SWAP   FIRE  RELOAD
//	 JUMP
//
// PICK UP appears when there's a weapon at your feet, and pause is top left.
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

func newArenaTouch() *arenaTouch {
	t := &arenaTouch{
		fire:   touchButton{label: "FIRE", x: -0.40, y: -0.34, r: 0.115, color: touchRed, look: true},
		jump:   touchButton{label: "JUMP", x: -0.17, y: -0.17, r: 0.085, color: touchOrange},
		aim:    touchButton{label: "AIM", x: -0.17, y: -0.43, r: 0.075, color: touchBack},
		reload: touchButton{label: "RELOAD", x: -0.62, y: -0.16, r: 0.065, color: touchBack},
		swap:   touchButton{label: "SWAP", x: -0.62, y: -0.40, r: 0.065, color: touchBack},
		melee:  touchButton{label: "HAMMER", x: -0.17, y: -0.66, r: 0.065, color: touchBack},
		throw:  touchButton{label: "NADE", x: -0.38, y: -0.60, r: 0.065, color: touchBack},
		kind:   touchButton{label: "FRAG", x: -0.56, y: -0.66, r: 0.05, color: touchBack},
		pickUp: touchButton{label: "PICK UP", x: -0.86, y: -0.30, r: 0.075, color: touchOrange},
		pause:  touchButton{label: "II", x: 0.14, y: 0.14, r: 0.05, color: touchBack},
	}
	t.buttons = []*touchButton{&t.fire, &t.jump, &t.aim, &t.reload, &t.swap,
		&t.melee, &t.throw, &t.kind, &t.pickUp, &t.pause}
	return t
}

// release lets go of everything, the sights included.
func (t *arenaTouch) release() {
	t.touchControls.release()
	t.aiming = false
}

// read puts this frame's presses into c (the caller merges them with the
// keyboard's and pad's, see mergeTouch) and returns the look, in radians
// (yaw right, pitch up), and whether pause was tapped. The caller scales the
// look for sensitivity and zoom.
func (t *arenaTouch) read(in *input.State, w, h float32, me *arena.Player, pickup bool, c *arena.Input) (yaw, pitch float32, pause bool) {
	t.pickUp.hidden = !pickup
	t.kind.label = arena.GrenadeNames[me.GrenadeKind]
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
