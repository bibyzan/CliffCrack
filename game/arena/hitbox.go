package arena

import (
	"math"

	"CliffCrack/engine/mathx"
)

// A player's physics collider is just the sphere at their feet; bullets,
// blows and blasts use a hitbox instead: a capsule for the body and a sphere
// for the head (which takes extra damage).
const (
	bodyRadius = 0.36
	bodyLength = 0.75 // capsule axis, up from the feet sphere's centre
	headRadius = 0.21
	headHeight = 1.17 // head centre above the feet sphere's centre
	headMult   = 1.75 // damage multiplier for headshots
)

// capsule is the body's axis, bottom to top.
func (p *Player) capsule() (a, b mathx.Vec3) {
	a = p.Body.Position
	return a, a.Add(mathx.Vec3{0, bodyLength - 0.6*CrouchDrop*p.Crouch, 0})
}

// Head is the centre of the player's head.
func (p *Player) Head() mathx.Vec3 {
	return p.Body.Position.Add(mathx.Vec3{0, headHeight - CrouchDrop*p.Crouch, 0})
}

// Chest is where to aim for a body shot.
func (p *Player) Chest() mathx.Vec3 {
	return p.Body.Position.Add(mathx.Vec3{0, (bodyLength - 0.6*CrouchDrop*p.Crouch) * 0.8, 0})
}

// rayHit is how far along the ray (dir a unit vector) it enters the
// player's hitbox, within maxDist, and whether that's the head.
func (p *Player) rayHit(from, dir mathx.Vec3, maxDist float32) (dist float32, head, ok bool) {
	a, b := p.capsule()
	dist = maxDist
	if t, hit := rayCapsule(from, dir, a, b, bodyRadius); hit && t < dist {
		dist, ok = t, true
	}
	if t, hit := raySphere(from, dir, p.Head(), headRadius); hit && t <= dist {
		dist, head, ok = t, true, true
	}
	return dist, head, ok
}

// hitboxDist is the distance from q to the surface of the player's hitbox
// (0 inside it).
func (p *Player) hitboxDist(q mathx.Vec3) float32 {
	a, b := p.capsule()
	d := closestOnSegment(q, a, b).Sub(q).Len() - bodyRadius
	d = min(d, p.Head().Sub(q).Len()-headRadius)
	return max(d, 0)
}

func closestOnSegment(q, a, b mathx.Vec3) mathx.Vec3 {
	ab := b.Sub(a)
	t := clamp(q.Sub(a).Dot(ab)/ab.Dot(ab), 0, 1)
	return a.Add(ab.Scale(t))
}

// raySphere is the distance along a ray (unit dir) to where it enters a
// sphere. Rays starting inside don't hit.
func raySphere(o, dir, c mathx.Vec3, r float32) (float32, bool) {
	oc := o.Sub(c)
	b := oc.Dot(dir)
	cc := oc.Dot(oc) - r*r
	if cc <= 0 {
		return 0, false
	}
	h := b*b - cc
	if h < 0 {
		return 0, false
	}
	t := -b - float32(math.Sqrt(float64(h)))
	return t, t >= 0
}

// rayCapsule is the distance along a ray (unit dir) to where it enters the
// capsule around segment a-b.
func rayCapsule(o, dir, a, b mathx.Vec3, r float32) (float32, bool) {
	best, ok := float32(math.MaxFloat32), false
	try := func(t float32, hit bool) {
		if hit && t < best {
			best, ok = t, true
		}
	}
	// The cylinder between the caps.
	ba, oa := b.Sub(a), o.Sub(a)
	baba, bard, baoa := ba.Dot(ba), ba.Dot(dir), ba.Dot(oa)
	k2 := baba - bard*bard
	if k2 > 1e-6 {
		k1 := baba*oa.Dot(dir) - baoa*bard
		k0 := baba*oa.Dot(oa) - baoa*baoa - r*r*baba
		if h := k1*k1 - k2*k0; h >= 0 && k0 > 0 {
			t := (-k1 - float32(math.Sqrt(float64(h)))) / k2
			if y := baoa + t*bard; y > 0 && y < baba {
				try(t, t >= 0)
			}
		}
	}
	try(raySphere(o, dir, a, r))
	try(raySphere(o, dir, b, r))
	return best, ok
}
