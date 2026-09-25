package arena

import (
	"math"

	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// WeaponKind is a loadout slot.
type WeaponKind int

const (
	WeaponHammer WeaponKind = iota
	WeaponRifle
	WeaponLauncher
	weaponCount
)

// WeaponNames are the HUD labels, by slot.
var WeaponNames = [weaponCount]string{"HAMMER", "RIFLE", "LAUNCHER"}

// Weapon tuning.
const (
	SwitchTime = 0.3 // s to put one weapon away and bring the next up

	MagSize          = 30
	fireInterval     = 0.1 // s between rifle shots (600 rpm)
	ReloadTime       = 1.5 // s
	maxRange         = 200
	baseSpread       = 0.002 // radians of cone half-angle standing still
	moveSpread       = 0.012 // extra while moving
	bloomPerShot     = 0.004 // extra per shot in a burst, decays quickly
	recoilKick       = 0.012 // radians of pitch per shot
	recoilReturn     = 4.0   // 1/s: sustained fire climbs ~2 degrees, then settles
	rifleChunkDamage = 12
	RifleDamage      = 14 // per round to a player (x headMult in the head)

	HammerSwing        = 0.7  // s per swing
	HammerHitAt        = 0.22 // s into the swing when the head lands
	hammerReach        = 2.8  // m
	hammerDamage       = 120  // to structures, at the point of impact
	hammerRadius       = 0.75 // m: the crater it smashes (about a panel or two of wood)
	hammerPush         = 7    // m/s given to rubble
	HammerPlayerDamage = 80   // two blows down a player

	LauncherMag       = 6
	launcherInterval  = 0.7 // s
	LauncherReload    = 2.2 // s
	grenadeRadius     = 0.1
	grenadeSpeed      = 26 // m/s
	grenadeFuse       = 2.5
	BlastRadius       = 4.2
	blastDamage       = 330 // to structures
	BlastPlayerDamage = 120 // to a player at the centre
	blastPush         = 13  // m/s at the centre
)

// RifleState is the rifle's magazine and timers.
type RifleState struct {
	Ammo      int
	Reloading float32 // seconds left, 0 when ready
	cooldown  float32
	bloom     float32
	Kick      float32 // 0..1 visual recoil for the gun model, decays fast
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
	Reloading float32
	cooldown  float32
	Kick      float32
}

// Grenade is a launched round in flight: it explodes on its first impact,
// when it reaches another player, or when its fuse runs out.
type Grenade struct {
	Body  *physics.Body
	Owner *Player
	Age   float32
}

// Weapons is a player's loadout.
type Weapons struct {
	Current   WeaponKind
	Switching float32 // seconds until the new weapon is ready
	Rifle     RifleState
	Hammer    HammerState
	Launcher  LauncherState
}

func newWeapons() Weapons {
	return Weapons{
		Rifle:    RifleState{Ammo: MagSize},
		Hammer:   HammerState{Swing: -1},
		Launcher: LauncherState{Ammo: LauncherMag},
	}
}

// Reloading reports the current weapon's reload progress (0..1) and whether it's reloading.
func (w *Weapons) Reloading() (float32, bool) {
	switch w.Current {
	case WeaponRifle:
		if w.Rifle.Reloading > 0 {
			return 1 - w.Rifle.Reloading/ReloadTime, true
		}
	case WeaponLauncher:
		if w.Launcher.Reloading > 0 {
			return 1 - w.Launcher.Reloading/LauncherReload, true
		}
	}
	return 0, false
}

func (a *Arena) updateWeapons(p *Player, dt float32, in Input, ev *Events) {
	w := &p.Weapons
	w.Rifle.Kick *= float32(math.Exp(-18 * float64(dt)))
	w.Launcher.Kick *= float32(math.Exp(-10 * float64(dt)))
	w.Rifle.bloom *= float32(math.Exp(-6 * float64(dt)))
	// Reloads carry on in the background, so switching away doesn't lose one.
	if w.Rifle.Reloading > 0 {
		if w.Rifle.Reloading = max(w.Rifle.Reloading-dt, 0); w.Rifle.Reloading == 0 {
			w.Rifle.Ammo = MagSize
		}
	}
	if w.Launcher.Reloading > 0 {
		if w.Launcher.Reloading = max(w.Launcher.Reloading-dt, 0); w.Launcher.Reloading == 0 {
			w.Launcher.Ammo = LauncherMag
		}
	}

	next := w.Current
	if in.Select >= 1 && in.Select <= int(weaponCount) {
		next = WeaponKind(in.Select - 1)
	}
	if in.Cycle != 0 {
		next = WeaponKind((int(w.Current) + in.Cycle + int(weaponCount)) % int(weaponCount))
	}
	if next != w.Current {
		w.Current = next
		w.Switching = SwitchTime
		w.Hammer = HammerState{Swing: -1}
		ev.act(p, ActSwitch, 0)
	}
	if w.Switching > 0 {
		w.Switching = max(w.Switching-dt, 0)
		w.Rifle.cooldown = max(w.Rifle.cooldown-dt, 0)
		w.Launcher.cooldown = max(w.Launcher.cooldown-dt, 0)
		return
	}

	switch w.Current {
	case WeaponHammer:
		a.updateHammer(p, dt, in, ev)
	case WeaponRifle:
		a.updateRifle(p, dt, in, ev)
	case WeaponLauncher:
		a.updateLauncher(p, dt, in, ev)
	}
}

func (a *Arena) updateRifle(p *Player, dt float32, in Input, ev *Events) {
	w := &p.Rifle
	// The cooldown may go negative while the trigger is held, so leftover time
	// carries into the next shot and the fire rate doesn't depend on frame rate.
	w.cooldown -= dt
	if !in.Fire || w.Reloading > 0 {
		w.cooldown = max(w.cooldown, 0)
	}
	if w.Reloading > 0 {
		return
	}
	if in.Reload && w.Ammo < MagSize {
		w.Reloading = ReloadTime
		ev.act(p, ActReload, 0)
		return
	}
	if !in.Fire || w.cooldown > 0 {
		return
	}
	if w.Ammo == 0 {
		if in.FirePressed {
			ev.act(p, ActEmpty, 0)
			w.Reloading = ReloadTime
			ev.act(p, ActReload, 0)
		}
		return
	}

	w.cooldown += fireInterval
	if !a.InfiniteAmmo {
		w.Ammo--
	}
	p.ShotsFired++

	moving := mathx.Vec3{p.Body.Velocity[0], 0, p.Body.Velocity[2]}.Len() / walkSpeed
	spread := baseSpread + moveSpread*min(moving, 1) + w.bloom
	if !p.onGround {
		spread += moveSpread
	}
	dir := a.jitter(p.Forward(), spread)
	shot := a.trace(p, p.Eye(1), dir, maxRange)
	ev.Shots = append(ev.Shots, shot)

	w.bloom = min(w.bloom+bloomPerShot, 0.03)
	w.Kick = 1
	p.recoil += recoilKick

	switch {
	case shot.Victim != nil:
		p.ShotsHit++
		damage := float32(RifleDamage)
		if shot.Head {
			damage *= headMult
			p.Headshots++
		}
		a.hurtPlayer(shot.Victim, p, damage, shot.Head, WeaponRifle, shot.From, dir.Scale(0.4), ev)
	case shot.Chunk != nil:
		a.damageChunk(shot.Chunk, rifleChunkDamage, dir.Scale(3), p, ev)
	}
	if w.Ammo == 0 && !a.InfiniteAmmo {
		w.Reloading = ReloadTime // auto-reload after the last round
		ev.act(p, ActReload, 0)
	}
}

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

func (a *Arena) updateHammer(p *Player, dt float32, in Input, ev *Events) {
	h := &p.Hammer
	if h.Swing >= 0 {
		h.Swing += dt
		if !h.struck && h.Swing >= HammerHitAt {
			h.struck = true
			a.hammerStrike(p, ev)
		}
		if h.Swing >= HammerSwing {
			h.Swing = -1
		}
	}
	if h.Swing < 0 && in.Fire {
		h.Swing, h.struck = 0, false
		ev.act(p, ActSwing, 0)
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
		a.blast(best.To, hammerRadius, hammerDamage, hammerPush, fwd.Scale(hammerPush*0.5), p, false, ev)
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
	if in.Reload && l.Ammo < LauncherMag {
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
			l.Reloading = LauncherReload
			ev.act(p, ActReload, 0)
		}
		return
	}
	l.cooldown = launcherInterval
	if !a.InfiniteAmmo {
		l.Ammo--
	}
	fwd := p.Forward()
	b := physics.NewSphere(grenadeRadius, 0.6)
	b.Position = p.Eye(1).Add(fwd.Scale(0.7))
	b.Velocity = fwd.Scale(grenadeSpeed).Add(mathx.Vec3{0, 1.5, 0}).Add(p.Body.Velocity)
	b.Restitution = 0.3
	b.Ignore = p.Body // it leaves the barrel from inside your own collider
	g := &Grenade{Body: b, Owner: p}
	b.UserData = g
	a.Phys.Add(b)
	a.Grenades = append(a.Grenades, g)
	ev.act(p, ActLaunch, 0)
	l.Kick = 1
	p.recoil += 0.05
	if l.Ammo == 0 && !a.InfiniteAmmo {
		l.Reloading = LauncherReload
		ev.act(p, ActReload, 0)
	}
}

// updateGrenades detonates grenades that hit something, reached another
// player or ran out of fuse.
func (a *Arena) updateGrenades(dt float32, ev *Events) {
	if len(a.Grenades) == 0 {
		return
	}
	hit := map[*physics.Body]bool{}
	for _, im := range a.Phys.Impacts() {
		hit[im.A], hit[im.B] = true, true
	}
	live := a.Grenades[:0]
	for _, g := range a.Grenades {
		g.Age += dt
		if !hit[g.Body] && !a.nearEnemy(g) && g.Age < grenadeFuse && g.Body.Position[1] > fallDeath {
			live = append(live, g)
			continue
		}
		a.Phys.Remove(g.Body)
		a.explode(g.Body.Position, g.Owner, ev)
	}
	clear(a.Grenades[len(live):])
	a.Grenades = live
}

// nearEnemy reports whether a grenade has reached someone else's hitbox.
func (a *Arena) nearEnemy(g *Grenade) bool {
	for _, p := range a.Players {
		if p != g.Owner && !p.Dead && p.hitboxDist(g.Body.Position) <= grenadeRadius {
			return true
		}
	}
	return false
}

// explode is a grenade going off: heavy damage and a shove in a radius,
// including players (rocket jumps work, and cost some health).
func (a *Arena) explode(at mathx.Vec3, by *Player, ev *Events) {
	ev.Explosions = append(ev.Explosions, Explosion{At: at, By: by})
	a.blast(at, BlastRadius, blastDamage, blastPush, mathx.Vec3{}, by, true, ev)
}
