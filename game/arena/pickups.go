package arena

import (
	"math"

	"CliffCrack/engine/mathx"
)

// Pickup is something lying about to take: a weapon (with the ammo it holds)
// or a crate of grenades. Weapons are taken with Interact, swapping for the
// one in hand; walking over one you already carry takes its ammo; grenades
// are taken by walking over them.
type Pickup struct {
	Weapon  WeaponKind  // NoWeapon for a crate of grenades
	Grenade GrenadeKind // ... of this kind
	Count   int         // ... this many
	Ammo    int         // the weapon's magazine
	Reserve int         // ... and the rounds with it
	At      mathx.Vec3  // where it rests (its underside)
	Yaw     float32     // which way it lies
	Table   bool        // laid out on the range's table: never runs out
	Age     float32     // s since it was dropped (dropped ones are cleared)
	spot    int         // the spot it spawned at, -1 if dropped
	fall    float32     // m/s, falling when what it lay on is gone
}

// PickupSpot is where a pickup spawns, and comes back after it's taken.
type PickupSpot struct {
	Pickup  Pickup
	Respawn float32 // s after it's taken
	due     float32 // arena time it's next due, while it's out
	out     bool    // taken, and not back yet
}

const (
	pickupReach  = 1.7 // m (flat) from a pickup you can take it
	pickupHeight = 1.8 // m of height difference you can reach across
	dropLife     = 30  // s a dropped weapon lies there
	powerRespawn = 45  // s for the sniper and the launcher to come back
	gunRespawn   = 25
	crateRespawn = 20
)

// weaponPickup is a fresh weapon of kind k, loaded.
func weaponPickup(k WeaponKind, at mathx.Vec3) Pickup {
	p := Pickup{Weapon: k, At: at, spot: -1}
	if g := gunFor(k); g != nil {
		p.Ammo, p.Reserve = g.Mag, g.Reserve
	} else if k == WeaponLauncher {
		p.Ammo, p.Reserve = LauncherMag, LauncherReserve
	}
	return p
}

// grenadePickup is a crate of n grenades of kind k.
func grenadePickup(k GrenadeKind, n int, at mathx.Vec3) Pickup {
	return Pickup{Weapon: NoWeapon, Grenade: k, Count: n, At: at, spot: -1}
}

// respawnFor is how long a spot's pickup takes to come back.
func respawnFor(p Pickup) float32 {
	switch p.Weapon {
	case WeaponSniper, WeaponLauncher:
		return powerRespawn
	case NoWeapon:
		return crateRespawn
	}
	return gunRespawn
}

// Name is how the HUD refers to a pickup.
func (p *Pickup) Name() string {
	if p.Weapon == NoWeapon {
		return GrenadeNames[p.Grenade] + " GRENADES"
	}
	return WeaponNames[p.Weapon]
}

// updatePickups brings spots' pickups back, lets pickups fall when what
// they lay on is gone, clears old dropped weapons, and gives players what
// they walk over: grenades, and ammo for guns they carry.
func (a *Arena) updatePickups(dt float32) {
	for i := range a.Spots {
		s := &a.Spots[i]
		if s.out && a.Time >= s.due {
			s.out = false
			p := s.Pickup
			p.spot = i
			a.Pickups = append(a.Pickups, &p)
		}
	}
	live := a.Pickups[:0]
	for _, p := range a.Pickups {
		gone := false
		if !p.Table {
			p.Age += dt
			a.settlePickup(p, dt)
			gone = p.At[1] < fallDeath || (p.spot < 0 && p.Age > dropLife)
		}
		for _, pl := range a.Players {
			if !gone && !pl.Dead && a.within(pl, p) && a.walkOver(pl, p) && !p.Table {
				gone = true
			}
		}
		if gone {
			a.taken(p)
			continue
		}
		live = append(live, p)
	}
	clear(a.Pickups[len(live):])
	a.Pickups = live
}

// settle drops a pickup onto whatever's under it, falling if that's gone.
func (a *Arena) settlePickup(p *Pickup, dt float32) {
	from := p.At.Add(mathx.Vec3{0, 0.3, 0})
	hit, ok := a.Phys.Raycast(from, mathx.Vec3{0, -1, 0}, 0.3+max(p.fall*dt, 0.05), a.ignoreForAim)
	if ok {
		p.At[1], p.fall = hit.Point[1], 0
		return
	}
	p.fall += gravity * dt
	p.At[1] -= p.fall * dt
}

// taken starts a spot's pickup on its way back.
func (a *Arena) taken(p *Pickup) {
	if p.spot >= 0 {
		s := &a.Spots[p.spot]
		s.out, s.due = true, a.Time+s.Respawn
	}
}

// within reports whether pl is close enough to take p.
func (a *Arena) within(pl *Player, p *Pickup) bool {
	feet := pl.Body.Position.Sub(mathx.Vec3{0, PlayerRadius, 0})
	return flat(feet.Sub(p.At)).Len() < pickupReach && abs(feet[1]-p.At[1]) < pickupHeight
}

// walkOver takes what pl gets just by touching p: its grenades, or its
// ammo if pl carries the same weapon. It reports whether p is used up.
func (a *Arena) walkOver(pl *Player, p *Pickup) bool {
	w := &pl.Weapons
	if p.Weapon == NoWeapon {
		room := MaxGrenades - w.Grenades[p.Grenade]
		if room <= 0 {
			return false
		}
		w.Grenades[p.Grenade] += min(room, p.Count)
		return true
	}
	if !w.Holds(p.Weapon) || p.Ammo+p.Reserve == 0 {
		return false
	}
	if g := gunFor(p.Weapon); g != nil {
		s := &w.States[p.Weapon]
		if s.Reserve >= g.Reserve*2 {
			return false // full up
		}
		s.Reserve = min(s.Reserve+p.Ammo+p.Reserve, g.Reserve*2)
	} else {
		w.Launcher.Reserve = min(w.Launcher.Reserve+p.Ammo+p.Reserve, LauncherReserve*2)
	}
	return true
}

// NearestPickup is the weapon p could take with Interact right now: the
// closest in reach they don't carry.
func (a *Arena) NearestPickup(pl *Player) *Pickup {
	var best *Pickup
	bestDist := float32(math.MaxFloat32)
	for _, p := range a.Pickups {
		if p.Weapon == NoWeapon || pl.Holds(p.Weapon) || !a.within(pl, p) {
			continue
		}
		if d := flat(pl.Body.Position.Sub(p.At)).Len(); d < bestDist {
			best, bestDist = p, d
		}
	}
	return best
}

// pickUp takes the nearest weapon in reach: into an empty slot, or in place
// of the one in hand, which is dropped where it was taken from.
func (a *Arena) pickUp(pl *Player, ev *Events) {
	p := a.NearestPickup(pl)
	if p == nil || pl.Swinging() {
		return
	}
	w := &pl.Weapons
	slot := w.Active
	if w.Slots[1-slot] == NoWeapon {
		slot = 1 - slot
	}
	if old := w.Slots[slot]; old != NoWeapon {
		drop := weaponPickup(old, p.At)
		drop.Yaw = pl.Yaw
		if g := gunFor(old); g != nil {
			drop.Ammo, drop.Reserve = w.States[old].Ammo, w.States[old].Reserve
			w.States[old].Reloading = 0
		} else if old == WeaponLauncher {
			drop.Ammo, drop.Reserve = w.Launcher.Ammo, w.Launcher.Reserve
			w.Launcher.Reloading = 0
		}
		a.Pickups = append(a.Pickups, &drop)
	}
	// Take its ammo with it.
	if g := gunFor(p.Weapon); g != nil {
		s := &w.States[p.Weapon]
		s.Ammo, s.Reserve, s.Reloading, s.bloom = p.Ammo, p.Reserve, 0, 0
		if p.Table {
			s.Ammo, s.Reserve = g.Mag, g.Reserve
		}
	} else if p.Weapon == WeaponLauncher {
		w.Launcher.Ammo, w.Launcher.Reserve, w.Launcher.Reloading = p.Ammo, p.Reserve, 0
		if p.Table {
			w.Launcher.Ammo, w.Launcher.Reserve = LauncherMag, LauncherReserve
		}
	}
	w.Slots[slot] = p.Weapon
	w.setActive(slot)
	ev.act(pl, ActPickup, float32(p.Weapon))
	if !p.Table {
		a.removePickup(p)
		a.taken(p)
	}
}

func (a *Arena) removePickup(p *Pickup) {
	for i, q := range a.Pickups {
		if q == p {
			a.Pickups = append(a.Pickups[:i], a.Pickups[i+1:]...)
			return
		}
	}
}

// addSpots puts the site's pickup spots into the arena, their pickups out.
func (a *Arena) addSpots(spots []PickupSpot) {
	a.Spots = append(a.Spots[:0], spots...)
	for i := range a.Spots {
		p := a.Spots[i].Pickup
		p.spot = i
		a.Pickups = append(a.Pickups, &p)
	}
}

// SpawnedHere reports whether p is at its spawn spot (not dropped).
func (a *Arena) SpawnedHere(p *Pickup) bool { return p.spot >= 0 }
