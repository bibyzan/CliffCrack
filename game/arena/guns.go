package arena

// GunSpec is one paintball gun's handling, after Halo: Combat Evolved's
// loadout: a spraying assault rifle, a pistol that rewards precision, a
// pump shotgun and a sniper rifle. Spreads are cone half-angles in radians.
type GunSpec struct {
	Mag      int
	Reserve  int     // rounds carried beyond the magazine (you can carry twice this)
	Interval float32 // s between shots
	Auto     bool    // fires while the trigger is held (else once per pull)
	Reload   float32 // s (per shell, for one loaded a shell at a time)
	PerShell bool    // loaded a shell at a time; firing interrupts the reload
	Pellets  int     // paintballs per shot

	Damage      float32 // per paintball to a player
	HeadMult    float32 // ... times this in the head
	Precision   bool    // a headshot on a popped player (no armour left) kills
	HeadKills   bool    // a headshot kills even through armour
	ChunkDamage float32 // per paintball to a structure
	Range       float32 // m
	Falloff     float32 // m beyond which damage falls away (0: none)
	Push        float32 // m/s a hit shoves a player

	HipSpread, ADSSpread float32
	MoveSpread           float32 // extra at the hip while moving, and in the air
	Bloom, MaxBloom      float32 // extra per shot, and its cap
	BloomDecay           float32 // 1/s
	Recoil               float32 // radians of pitch per shot

	Zoom    float32 // view magnification with the sights up
	ADSTime float32 // s to raise the sights
	ADSMove float32 // movement speed with the sights up, as a fraction

	BallSpeed float32 // m/s the paintballs fly (drawn; the hit is instant)
	BoltTime  float32 // s the bolt's worked after a shot, the sights down meanwhile (0: no bolt)
}

// Guns are the specs by slot; nil for the hammer and the launcher.
var Guns = [weaponCount]*GunSpec{
	// The assault rifle: a hopper-fed marker hosing paint, and the yardstick
	// the others are tuned around. Weak per ball (24 to the body, 19 to the
	// head: most of a magazine as you really hit), wild from the hip, and it
	// blooms fast; aim down the sights to tame it.
	WeaponRifle: {
		Mag: 36, Reserve: 108, Interval: 0.075, Auto: true, Reload: 2.0, Pellets: 1,
		Damage: 7, HeadMult: 1.25, ChunkDamage: 9, Range: 120, Push: 0.3,
		HipSpread: 0.012, ADSSpread: 0.004, MoveSpread: 0.012, Bloom: 0.008, MaxBloom: 0.055, BloomDecay: 3.5, Recoil: 0.009,
		Zoom: 1.35, ADSTime: 0.16, ADSMove: 0.7, BallSpeed: 140,
	},
	// The pistol: three body shots pop armour, then a headshot kills (five to
	// the body). Measured, not spammed: quicker than the rifle for a steady
	// hand, and the thing to draw when the rifle's empty.
	WeaponPistol: {
		Mag: 12, Reserve: 48, Interval: 0.28, Reload: 1.4, Pellets: 1,
		Damage: 34, HeadMult: 1.5, Precision: true, ChunkDamage: 22, Range: 150, Push: 0.8,
		HipSpread: 0.008, ADSSpread: 0.0008, MoveSpread: 0.01, Bloom: 0.018, MaxBloom: 0.04, BloomDecay: 7, Recoil: 0.018,
		Zoom: 2, ADSTime: 0.14, ADSMove: 0.8, BallSpeed: 190,
	},
	// The pump shotgun, after the SPAS-12: a cone of paint that kills in one
	// up close (seven pellets pop armour, five more finish) and falls away
	// fast, and a long pump between shots. Its pellets tear through walls.
	WeaponShotgun: {
		Mag: 8, Reserve: 16, Interval: 1.0, Reload: 0.42, PerShell: true, Pellets: 12,
		Damage: 16, HeadMult: 1.25, ChunkDamage: 22, Range: 40, Falloff: 7, Push: 0.9,
		HipSpread: 0.075, ADSSpread: 0.055, MoveSpread: 0.01, Recoil: 0.07,
		Zoom: 1.15, ADSTime: 0.18, ADSMove: 0.8, BallSpeed: 110,
	},
	// The sniper: a body shot pops armour, a second finishes; a headshot
	// always kills. Wild from the hip, dead on through the scope, and the
	// bolt's worked between shots.
	WeaponSniper: {
		Mag: 4, Reserve: 12, Interval: 1.25, Reload: 2.5, Pellets: 1,
		Damage: 110, HeadMult: 2, Precision: true, HeadKills: true, ChunkDamage: 90, Range: 300, Push: 3,
		HipSpread: 0.035, ADSSpread: 0, MoveSpread: 0.02, Bloom: 0.03, MaxBloom: 0.05, BloomDecay: 3, Recoil: 0.07,
		Zoom: 5, ADSTime: 0.24, ADSMove: 0.5, BallSpeed: 420, BoltTime: 1.0,
	},
}

// Spread is the cone the next shot (or each pellet) can go anywhere in:
// the gun's hip or ADS spread, its bloom from firing, and at the hip more
// for moving and more still in the air.
func (p *Player) Spread() float32 {
	g, s := p.Gun()
	if g == nil {
		return 0
	}
	ads := smoothstep(p.ADS)
	spread := g.HipSpread + (g.ADSSpread-g.HipSpread)*ads + s.bloom*(1-0.6*ads)
	moving := min(flat(p.Body.Velocity).Len()/walkSpeed, 1)
	spread += g.MoveSpread * moving * (1 - 0.8*ads)
	if !p.onGround {
		spread += g.MoveSpread*(1-0.5*ads) + 0.01
	}
	return spread
}

// updateGun fires the current gun: automatic ones while the trigger is
// held, the rest once per pull, each paintball (or pellet) a hitscan trace
// with the current spread.
func (a *Arena) updateGun(p *Player, dt float32, in Input, ev *Events) {
	g, w := p.Gun()
	// The cooldown may go negative while the trigger is held, so leftover time
	// carries into the next shot and the fire rate doesn't depend on frame rate.
	w.cooldown -= dt
	if !in.Fire || w.Reloading > 0 {
		w.cooldown = max(w.cooldown, 0)
	}
	if w.Reloading > 0 {
		if !(g.PerShell && in.FirePressed && w.Ammo > 0) {
			return
		}
		w.Reloading = 0 // a shell at a time: firing breaks off the reload
	}
	canReload := w.Reserve > 0 || a.FreeAmmo
	if in.Reload && w.Ammo < g.Mag && canReload {
		w.Reloading = g.Reload
		ev.act(p, ActReload, 0)
		return
	}
	trigger := in.FirePressed
	if g.Auto {
		trigger = in.Fire
	}
	if !trigger || w.cooldown > 0 {
		return
	}
	if w.Ammo == 0 {
		if in.FirePressed {
			ev.act(p, ActEmpty, 0)
			if canReload {
				w.Reloading = g.Reload
				ev.act(p, ActReload, 0)
			}
		}
		return
	}

	if g.Auto {
		w.cooldown += g.Interval
	} else {
		w.cooldown = g.Interval
	}
	if !a.InfiniteAmmo {
		w.Ammo--
	}
	p.ShotsFired++

	spread := p.Spread()
	eye, fwd := p.Eye(1), p.Forward()
	hit := false
	for range g.Pellets {
		dir := a.jitter(fwd, spread)
		shot := a.trace(p, eye, dir, g.Range)
		shot.Weapon = p.Current
		ev.Shots = append(ev.Shots, shot)
		dist := shot.To.Sub(eye).Len()
		scale := float32(1)
		if g.Falloff > 0 && dist > g.Falloff {
			scale = clamp(1-(dist-g.Falloff)/(g.Range-g.Falloff)*1.6, 0.1, 1)
		}
		switch {
		case shot.Victim != nil:
			hit = true
			if shot.Head {
				p.Headshots++
			}
			a.hurtPlayer(shot.Victim, p, g.Damage*scale, shot.Head, p.Current, shot.From, dir.Scale(g.Push), ev)
		case shot.Chunk != nil:
			a.damageChunk(shot.Chunk, g.ChunkDamage*scale, dir.Scale(3), p, ev)
		}
	}
	if hit {
		p.ShotsHit++
	}

	ads := smoothstep(p.ADS)
	w.bloom = min(w.bloom+g.Bloom*(1-0.5*ads), g.MaxBloom)
	w.Kick, w.SinceShot = 1, 0
	p.recoil += g.Recoil * (1 - 0.5*ads)
	if w.Ammo == 0 && !a.InfiniteAmmo && canReload {
		w.Reloading = g.Reload // auto-reload after the last round
		ev.act(p, ActReload, 0)
	}
}

// gunFor is the spec of the gun a weapon kind is, if it's a gun.
func gunFor(k WeaponKind) *GunSpec {
	if k >= 0 && k < weaponCount {
		return Guns[k]
	}
	return nil
}
