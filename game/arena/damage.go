package arena

import (
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// maxDebris caps loose pieces; beyond it the oldest are cleared first.
const maxDebris = 450

// damageChunk wears a chunk down and breaks it at zero. by (nil for none)
// gets the credit.
func (a *Arena) damageChunk(c *Chunk, damage float32, push mathx.Vec3, by *Player, ev *Events) {
	if !c.Alive {
		return
	}
	c.HP -= damage
	if c.HP <= 0 {
		a.breakChunk(c, push, by, ev)
	}
}

// breakChunk shatters a chunk into a few smaller pieces flung by push.
func (a *Arena) breakChunk(c *Chunk, push mathx.Vec3, by *Player, ev *Events) {
	a.removeChunk(c, by)
	n := int(volume(c.Half) / 0.05)
	n = max(1, min(n, 4))
	if c.Mat == Glass {
		n = 3
	}
	half := c.Half.Scale(0.5)
	for k := range half {
		half[k] = max(half[k], 0.03)
	}
	for range n {
		at := c.Centre.Add(mathx.Vec3{
			(a.rng.Float32() - 0.5) * c.Half[0],
			(a.rng.Float32() - 0.5) * c.Half[1],
			(a.rng.Float32() - 0.5) * c.Half[2],
		})
		spray := mathx.Vec3{a.rng.Float32()*2 - 1, a.rng.Float32() + 0.3, a.rng.Float32()*2 - 1}.Scale(1.5)
		a.addDebris(at, half, c.Mat, push.Add(spray))
	}
	ev.Breaks = append(ev.Breaks, Break{At: c.Centre, Half: c.Half, Mat: c.Mat})
}

// removeChunk takes a chunk out of its structure and the physics world.
func (a *Arena) removeChunk(c *Chunk, by *Player) {
	c.Alive = false
	c.Structure.alive--
	c.Structure.dirty = true
	a.Phys.Remove(c.Body)
	if by != nil {
		by.Destroyed++
	}
}

// settle brings down every chunk left without support: each falls as one
// piece of rubble.
func (a *Arena) settle(ev *Events) {
	for _, s := range a.Structures {
		if !s.dirty {
			continue
		}
		s.dirty = false
		for _, c := range s.unsupported() {
			a.removeChunk(c, nil)
			drift := mathx.Vec3{a.rng.Float32() - 0.5, 0, a.rng.Float32() - 0.5}.Scale(0.6)
			a.addDebris(c.Centre, c.Half, c.Mat, drift)
			ev.Breaks = append(ev.Breaks, Break{At: c.Centre, Half: c.Half, Mat: c.Mat, Collapsed: true})
		}
		s.dirty = false // the fallen chunks don't hold anything up either
	}
}

// addDebris adds a loose piece drawn as a box of the given half extents.
func (a *Arena) addDebris(at, half mathx.Vec3, m Material, vel mathx.Vec3) *Debris {
	if len(a.Debris) >= maxDebris {
		a.Phys.Remove(a.Debris[0].Body)
		a.Debris = append(a.Debris[:0], a.Debris[1:]...)
	}
	info := Materials[m]
	r := max((half[0]+half[1]+half[2])/3*0.9, 0.04)
	b := physics.NewSphere(r, clamp(info.Density*volume(half), 0.05, 300))
	b.Position, b.Velocity = at, vel
	b.Restitution, b.Friction = info.Restitution, info.Friction
	b.AngularVelocity = mathx.Vec3{a.rng.Float32()*8 - 4, a.rng.Float32()*8 - 4, a.rng.Float32()*8 - 4}
	d := &Debris{Body: b, Half: half, Mat: m, Life: info.DebrisLife * (0.8 + 0.4*a.rng.Float32())}
	b.UserData = d
	a.Phys.Add(b)
	a.Debris = append(a.Debris, d)
	return d
}

func (a *Arena) ageDebris(dt float32) {
	alive := a.Debris[:0]
	for _, d := range a.Debris {
		d.Age += dt
		if d.Age > d.Life || d.Body.Position[1] < -20 {
			a.Phys.Remove(d.Body)
			continue
		}
		alive = append(alive, d)
	}
	clear(a.Debris[len(alive):])
	a.Debris = alive
}

// blast damages every structure chunk within radius of at (full at the
// centre, falling to nothing at the edge) and shoves loose bodies outwards.
// bias is added to every shove (a hammer drives debris away from the swing).
// With hitsPlayers, players in range are hurt and shoved too; by gets the
// credit.
func (a *Arena) blast(at mathx.Vec3, radius, damage, push float32, bias mathx.Vec3, by *Player, hitsPlayers bool, ev *Events) {
	// Pieces this blast breaks off already carry its push; shove only the
	// ones that were lying around before.
	loose := append([]*Debris(nil), a.Debris...)
	for _, s := range a.Structures {
		for _, c := range s.Chunks {
			if !c.Alive {
				continue
			}
			d := c.distTo(at)
			if d >= radius {
				continue
			}
			f := 1 - d/radius
			dir := c.Centre.Sub(at).Normalize()
			a.damageChunk(c, damage*f, dir.Scale(push*f).Add(bias), by, ev)
		}
	}
	shove := func(b *physics.Body, scale float32) mathx.Vec3 {
		off := b.Position.Sub(at)
		dist := off.Len()
		if dist >= radius {
			return mathx.Vec3{}
		}
		dir := off.Normalize()
		if dist < 1e-4 {
			dir = mathx.Vec3{0, 1, 0}
		}
		return dir.Scale(push * (1 - dist/radius) * scale).Add(bias.Scale(scale))
	}
	for _, d := range loose {
		d.Body.Velocity = d.Body.Velocity.Add(shove(d.Body, 1))
	}
	for _, g := range a.Grenades {
		g.Body.Velocity = g.Body.Velocity.Add(shove(g.Body, 0.5))
	}
	if !hitsPlayers {
		return
	}
	for _, p := range a.Players {
		if p.Dead {
			continue
		}
		kick := shove(p.Body, 1)
		d := p.hitboxDist(at)
		if d >= radius {
			p.Body.Velocity = p.Body.Velocity.Add(kick)
			continue
		}
		a.hurtPlayer(p, by, BlastPlayerDamage*(1-d/radius), false, WeaponLauncher, at, kick, ev)
	}
}
