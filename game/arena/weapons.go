package arena

import (
	"math"

	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// WeaponKind is a weapon. A player carries two (see Weapons.Slots), swings
// the hammer as their melee, and throws grenades.
type WeaponKind int

const (
	WeaponHammer WeaponKind = iota // the melee: always to hand, never in a slot
	WeaponRifle                    // the assault rifle
	WeaponPistol
	WeaponShotgun
	WeaponSniper
	WeaponLauncher
	weaponCount

	// Not carried, but what else can take a player down.
	WeaponRubble = weaponCount     // crushed by falling debris
	WeaponDrop   = weaponCount + 1 // fell into the pit
	WeaponFrag   = weaponCount + 2 // a frag grenade
	WeaponSticky = weaponCount + 3 // a sticky grenade

	NoWeapon WeaponKind = -1 // an empty slot
)

// WeaponNames are the HUD labels.
var WeaponNames = [weaponCount]string{"HAMMER", "RIFLE", "PISTOL", "SHOTGUN", "SNIPER", "LAUNCHER"}

// Cause is how the kill feed names what took a player down.
func (k WeaponKind) Cause() string {
	switch k {
	case WeaponRubble:
		return "RUBBLE"
	case WeaponDrop:
		return "THE DROP"
	case WeaponFrag:
		return "FRAG"
	case WeaponSticky:
		return "STICKY"
	}
	if k >= 0 && k < weaponCount {
		return WeaponNames[k]
	}
	return "?"
}

// Weapon tuning that isn't per gun (see Guns for those).
const (
	SwitchTime = 0.3 // s to put one weapon away and bring the other up

	HammerSwing        = 0.7  // s per swing
	HammerHitAt        = 0.22 // s into the swing when the head lands
	hammerReach        = 2.8  // m
	hammerDamage       = 120  // to structures, at the point of impact
	hammerRadius       = 0.75 // m: the crater it smashes (about a panel or two of wood)
	hammerPush         = 7    // m/s given to rubble
	HammerPlayerDamage = 80   // gets through armour: two blows down a player

	LauncherMag       = 6
	LauncherReserve   = 12
	launcherInterval  = 0.7 // s
	LauncherReload    = 2.2 // s
	grenadeRadius     = 0.1
	grenadeSpeed      = 26 // m/s
	grenadeFuse       = 2.5
	BlastRadius       = 4.2
	blastDamage       = 330 // to structures
	BlastPlayerDamage = 120 // to a player at the centre (through armour)
	blastPush         = 13  // m/s at the centre

	recoilReturn = 4.0 // 1/s: how quickly recoil's climb settles
	descopeTime  = 0.5 // s: hit while zoomed in, your sights are knocked down this long
)

// GunState is one gun's magazine, reserve, timers, bloom and kick.
type GunState struct {
	Ammo      int
	Reserve   int     // rounds carried beyond the magazine
	Reloading float32 // seconds left, 0 when ready
	Kick      float32 // 0..1 visual recoil for the gun model, decays fast
	cooldown  float32
	bloom     float32 // extra spread from firing, radians; settles back
}

// HammerState is the sledgehammer's swing.
type HammerState struct {
	Swing  float32 // seconds into the current swing, -1 when idle
	struck bool
}

// Progress is how far through the swing the hammer is (0..1), or -1 idle.
func (h HammerState) Progress() float32 {
	if h.Swing < 0 {
		return -1
	}
	return h.Swing / HammerSwing
}

// LauncherState is the grenade launcher's magazine and timers.
type LauncherState struct {
	Ammo      int
	Reserve   int
	Reloading float32
	cooldown  float32
	Kick      float32
}

// Weapons is a player's loadout: two weapons in slots, the one in hand, the
// hammer to swing, and grenades.
type Weapons struct {
	Slots     [2]WeaponKind
	Active    int        // which slot is in hand
	Current   WeaponKind // the weapon in hand: Slots[Active]
	Switching float32    // seconds until the weapon coming up is ready
	States    [weaponCount]GunState
	Hammer    HammerState
	Launcher  LauncherState
	// ADS is how far the sights are raised, 0 (hip) to 1 (aiming down them).
	ADS     float32
	descope float32

	Grenades    [GrenadeKinds]int // carried, by kind
	GrenadeKind GrenadeKind       // which kind G throws
	throwWait   float32           // s before another can be thrown
}

// newWeapons is the loadout everyone starts with: the rifle in hand, the
// pistol on the hip, two frags and a sticky. Every gun's state starts full,
// so one picked up later comes loaded.
func newWeapons() Weapons {
	w := Weapons{Slots: [2]WeaponKind{WeaponRifle, WeaponPistol}, Current: WeaponRifle,
		Hammer: HammerState{Swing: -1}, Launcher: LauncherState{Ammo: LauncherMag, Reserve: LauncherReserve}}
	for k, g := range Guns {
		if g != nil {
			w.States[k].Ammo, w.States[k].Reserve = g.Mag, g.Reserve
		}
	}
	w.Grenades = [GrenadeKinds]int{Frag: 2, Sticky: 1}
	return w
}

// Holds reports whether k is in one of the slots.
func (w *Weapons) Holds(k WeaponKind) bool { return w.Slots[0] == k || w.Slots[1] == k }

// Other is the weapon in the slot not in hand.
func (w *Weapons) Other() WeaponKind { return w.Slots[1-w.Active] }

// setActive brings slot i up.
func (w *Weapons) setActive(i int) {
	w.Active, w.Current = i, w.Slots[i]
	w.Switching, w.ADS = SwitchTime, 0
}

// Gun is the current weapon's spec and state, or nil for the launcher (or
// an empty hand).
func (w *Weapons) Gun() (*GunSpec, *GunState) {
	if g := gunFor(w.Current); g != nil {
		return g, &w.States[w.Current]
	}
	return nil, nil
}

// Reloading reports the current weapon's reload progress (0..1) and whether it's reloading.
func (w *Weapons) Reloading() (float32, bool) {
	if g, s := w.Gun(); g != nil {
		if s.Reloading > 0 {
			return 1 - s.Reloading/g.Reload, true
		}
		return 0, false
	}
	if w.Current == WeaponLauncher && w.Launcher.Reloading > 0 {
		return 1 - w.Launcher.Reloading/LauncherReload, true
	}
	return 0, false
}

// Swinging reports whether the hammer is mid-swing (the gun is put aside).
func (w *Weapons) Swinging() bool { return w.Hammer.Swing >= 0 }

// Zoom is how much the view is magnified right now: 1 at the hip, up to the
// gun's zoom with its sights all the way up.
func (w *Weapons) Zoom() float32 {
	g, _ := w.Gun()
	if g == nil {
		return 1
	}
	return 1 + (g.Zoom-1)*smoothstep(w.ADS)
}

func smoothstep(t float32) float32 {
	t = clamp(t, 0, 1)
	return t * t * (3 - 2*t)
}

func (a *Arena) updateWeapons(p *Player, dt float32, in Input, ev *Events) {
	w := &p.Weapons
	for k, g := range Guns {
		s := &w.States[k]
		s.Kick *= float32(math.Exp(-14 * float64(dt)))
		if g == nil {
			continue
		}
		s.bloom *= float32(math.Exp(-float64(g.BloomDecay * dt)))
		// Reloads carry on in the background, so switching away doesn't lose one.
		if s.Reloading > 0 {
			if s.Reloading = max(s.Reloading-dt, 0); s.Reloading == 0 {
				take := g.Mag - s.Ammo
				if !a.FreeAmmo {
					take = min(take, s.Reserve)
					s.Reserve -= take
				}
				s.Ammo += take
			}
		}
	}
	w.Launcher.Kick *= float32(math.Exp(-10 * float64(dt)))
	if w.Launcher.Reloading > 0 {
		if w.Launcher.Reloading = max(w.Launcher.Reloading-dt, 0); w.Launcher.Reloading == 0 {
			take := LauncherMag - w.Launcher.Ammo
			if !a.FreeAmmo {
				take = min(take, w.Launcher.Reserve)
				w.Launcher.Reserve -= take
			}
			w.Launcher.Ammo += take
		}
	}

	// The other slot: picked by number, or swapped to.
	next := w.Active
	if in.Select >= 1 && in.Select <= 2 {
		next = in.Select - 1
	}
	if in.Cycle != 0 {
		next = 1 - w.Active
	}
	if next != w.Active && w.Slots[next] != NoWeapon && !w.Swinging() {
		w.setActive(next)
		ev.act(p, ActSwitch, 0)
	}
	if in.SwitchGrenade {
		w.GrenadeKind = (w.GrenadeKind + 1) % GrenadeKinds
		ev.act(p, ActSwitch, 0)
	}
	if in.Interact {
		a.pickUp(p, ev)
	}

	// Aiming down the sights: raised over the gun's ADS time while the aim
	// button is held, and dropped while switching, reloading, sprinting,
	// swinging or knocked out of a scope.
	w.descope = max(w.descope-dt, 0)
	g, s := w.Gun()
	// (Sprint held while scoped in holds your breath rather than sprinting.)
	breathing := g != nil && g.Zoom >= 4 && w.ADS > 0.5
	aiming := g != nil && in.Aim && w.Switching == 0 && s.Reloading == 0 && w.descope == 0 && (!sprinting(in) || breathing) && !w.Swinging()
	switch {
	case g == nil:
		w.ADS = 0
	case aiming:
		w.ADS = min(w.ADS+dt/g.ADSTime, 1)
	default:
		w.ADS = max(w.ADS-dt/g.ADSTime, 0)
	}

	// The hammer and grenades are to hand whatever's held; a swing puts the
	// gun aside until it's done.
	w.throwWait = max(w.throwWait-dt, 0)
	if in.Melee && !w.Swinging() {
		w.Hammer = HammerState{Swing: 0}
		w.ADS = 0
		ev.act(p, ActSwing, 0)
	}
	if w.Swinging() {
		a.updateHammer(p, dt, ev)
		return
	}
	if in.Throw {
		a.throwGrenade(p, ev)
	}

	if w.Switching > 0 {
		w.Switching = max(w.Switching-dt, 0)
		for k := range w.States {
			w.States[k].cooldown = max(w.States[k].cooldown-dt, 0)
		}
		w.Launcher.cooldown = max(w.Launcher.cooldown-dt, 0)
		return
	}

	switch w.Current {
	case WeaponLauncher:
		a.updateLauncher(p, dt, in, ev)
	case NoWeapon:
	default:
		a.updateGun(p, dt, in, ev)
	}
}

// sprinting reports whether the input sprints (forward, with sprint held).
func sprinting(in Input) bool { return in.Sprint && in.Move[1] > 0.3 }

// trace follows a ray from by's eye up to reach: to the first solid surface,
// or a player's hitbox in front of it. It ignores the shooter, debris and
// grenades.
func (a *Arena) trace(by *Player, from, dir mathx.Vec3, reach float32) Shot {
	shot := Shot{By: by, From: from, To: from.Add(dir.Scale(reach))}
	dist := reach
	if hit, ok := a.Phys.Raycast(from, dir, reach, a.ignoreForAim); ok {
		shot.To, shot.Normal, dist = hit.Point, hit.Normal, hit.Distance
		if c, ok := hit.Body.UserData.(*Chunk); ok {
			shot.Chunk = c
		}
	}
	for _, p := range a.Players {
		if p == by || p.Dead {
			continue
		}
		if t, head, ok := p.rayHit(from, dir, dist); ok {
			dist = t
			shot.To, shot.Normal = from.Add(dir.Scale(t)), dir.Scale(-1)
			shot.Victim, shot.Head, shot.Chunk = p, head, nil
		}
	}
	return shot
}

// Trace is where a ray from by's eye first hits: a player's hitbox, or a
// solid surface.
func (a *Arena) Trace(by *Player, from, dir mathx.Vec3, reach float32) Shot {
	return a.trace(by, from, dir, reach)
}

// jitter tilts dir by a random angle up to spread radians.
func (a *Arena) jitter(dir mathx.Vec3, spread float32) mathx.Vec3 {
	if spread <= 0 {
		return dir
	}
	right, up := basis(dir)
	r := spread * float32(math.Sqrt(a.rng.Float64())) // uniform over the disc
	th := a.rng.Float64() * 2 * math.Pi
	return dir.Add(right.Scale(r * float32(math.Cos(th)))).Add(up.Scale(r * float32(math.Sin(th)))).Normalize()
}

// basis returns two unit vectors perpendicular to dir and each other.
func basis(dir mathx.Vec3) (right, up mathx.Vec3) {
	up = mathx.Vec3{0, 1, 0}
	if math.Abs(float64(dir.Dot(up))) > 0.99 {
		up = mathx.Vec3{1, 0, 0}
	}
	right = dir.Cross(up).Normalize()
	return right, right.Cross(dir)
}

// updateHammer carries a swing through: the blow lands HammerHitAt in.
func (a *Arena) updateHammer(p *Player, dt float32, ev *Events) {
	h := &p.Hammer
	h.Swing += dt
	if !h.struck && h.Swing >= HammerHitAt {
		h.struck = true
		a.hammerStrike(p, ev)
	}
	if h.Swing >= HammerSwing {
		h.Swing = -1
	}
}

// hammerStrike lands a blow: the nearest thing within reach along the view
// (or a little either side of it) takes it. A player takes a heavy hit and a
// shove; a structure gets a crater.
func (a *Arena) hammerStrike(p *Player, ev *Events) {
	eye, fwd := p.Eye(1), p.Forward()
	right, up := basis(fwd)
	var best Shot
	bestDist := float32(math.MaxFloat32)
	for _, off := range [][2]float32{{0, 0}, {0.14, 0}, {-0.14, 0}, {0, 0.12}, {0, -0.14}} {
		dir := fwd.Add(right.Scale(off[0])).Add(up.Scale(off[1])).Normalize()
		s := a.trace(p, eye, dir, hammerReach)
		if s.Normal == (mathx.Vec3{}) {
			continue // a whiff
		}
		// Prefer a player anywhere in the arc over a wall behind them.
		d := s.To.Sub(eye).Len()
		if s.Victim != nil {
			d -= hammerReach
		}
		if d < bestDist {
			best, bestDist = s, d
		}
	}
	if bestDist == math.MaxFloat32 {
		return
	}
	smash := Smash{By: p, At: best.To, Normal: best.Normal, Mat: -1, Victim: best.Victim}
	switch {
	case best.Victim != nil:
		push := fwd.Scale(6).Add(mathx.Vec3{0, 2.5, 0})
		a.hurtPlayer(best.Victim, p, HammerPlayerDamage, false, WeaponHammer, eye, push, ev)
	default:
		if best.Chunk != nil {
			smash.Mat = best.Chunk.Mat
		}
		a.blast(best.To, hammerRadius, hammerDamage, 0, hammerPush, fwd.Scale(hammerPush*0.5), p, WeaponHammer, ev)
	}
	ev.Smashes = append(ev.Smashes, smash)
	p.recoil += 0.035 // the jolt of the impact
}

func (a *Arena) updateLauncher(p *Player, dt float32, in Input, ev *Events) {
	l := &p.Launcher
	l.cooldown = max(l.cooldown-dt, 0)
	if l.Reloading > 0 {
		return
	}
	canReload := l.Reserve > 0 || a.FreeAmmo
	if in.Reload && l.Ammo < LauncherMag && canReload {
		l.Reloading = LauncherReload
		ev.act(p, ActReload, 0)
		return
	}
	if !in.Fire || l.cooldown > 0 {
		return
	}
	if l.Ammo == 0 {
		if in.FirePressed {
			ev.act(p, ActEmpty, 0)
			if canReload {
				l.Reloading = LauncherReload
				ev.act(p, ActReload, 0)
			}
		}
		return
	}
	l.cooldown = launcherInterval
	if !a.InfiniteAmmo {
		l.Ammo--
	}
	fwd := p.Forward()
	a.launch(p, GrenadeRound, p.Eye(1).Add(fwd.Scale(0.7)), fwd.Scale(grenadeSpeed).Add(mathx.Vec3{0, 1.5, 0}))
	ev.act(p, ActLaunch, 0)
	l.Kick = 1
	p.recoil += 0.05
	if l.Ammo == 0 && !a.InfiniteAmmo && (l.Reserve > 0 || a.FreeAmmo) {
		l.Reloading = LauncherReload
		ev.act(p, ActReload, 0)
	}
}

// launch puts a grenade of kind in flight from at, leaving the owner at
// vel on top of their own velocity.
func (a *Arena) launch(p *Player, kind GrenadeKind, at, vel mathx.Vec3) *Grenade {
	spec := grenadeSpecs[kind]
	b := physics.NewSphere(grenadeRadius, 0.6)
	b.Position = at
	b.Velocity = vel.Add(p.Body.Velocity)
	b.Restitution = spec.Bounce
	b.Ignore = p.Body // it leaves from inside your own collider
	g := &Grenade{Body: b, Owner: p, Kind: kind}
	b.UserData = g
	if err := a.Phys.Add(b); err != nil {
		panic(err)
	}
	a.Grenades = append(a.Grenades, g)
	return g
}
