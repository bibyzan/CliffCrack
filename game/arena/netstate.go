package arena

import (
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
)

// Online play is host-authoritative: the host steps the Match with every
// player's Input and sends the others the result. Everything is shared:
// each player's movement, aim, armour, weapons and ammo (Snapshot), the
// grenades and pickups, and every event (NetEvents): shots, hits, kills,
// hammer blows, explosions and every chunk of the site that breaks, is
// chipped or collapses. A client never steps the match itself; it applies
// what it's sent to its own copy (built from the same seed, so chunk IDs
// and the site agree) and only simulates the flying rubble, which is
// cosmetic.

// V3 is a vector on the wire.
type V3 = [3]float32

func v3(v mathx.Vec3) V3  { return V3(v) }
func vec(v V3) mathx.Vec3 { return mathx.Vec3(v) }

// NetGun is a gun's state.
type NetGun struct {
	Ammo      int     `json:"a,omitempty"`
	Reserve   int     `json:"r,omitempty"`
	Reloading float32 `json:"l,omitempty"`
	Kick      float32 `json:"k,omitempty"`
	SinceShot float32 `json:"s,omitempty"`
	Cooldown  float32 `json:"c,omitempty"`
	Bloom     float32 `json:"b,omitempty"`
}

// NetPlayer is everything about a player.
type NetPlayer struct {
	Pos, Vel     V3
	Yaw, Pitch   float32
	Recoil       float32
	Sway         [2]float32
	Breath       float32
	Shield       float32
	Health       float32
	Dead         bool
	DiedAt       float32
	Flash        float32
	OnGround     bool
	Boosted      bool
	BoostTop     float32
	Airborne     float32
	SinceLand    float32
	JumpQueue    float32
	OnSlope      bool
	Crouch       float32     `json:",omitempty"`
	Crouched     bool        `json:",omitempty"`
	WasCrouch    bool        `json:",omitempty"`
	Sliding      bool        `json:",omitempty"`
	SlideTime    float32     `json:",omitempty"`
	SlideCool    float32     `json:",omitempty"`
	Mantle       MantleState `json:",omitempty"`
	SincePad     float32
	SinceJump    float32
	Slots        [2]WeaponKind
	Active       int
	Current      WeaponKind
	Switching    float32
	ADS          float32
	Descope      float32
	Guns         [weaponCount]NetGun
	HammerSwing  float32
	HammerStruck bool
	HammerOut    bool
	ElbowSwing   float32
	ElbowStruck  bool
	Gadget       GadgetKind
	Grapple      NetGrapple
	Launcher     NetGun
	Grenades     [GrenadeKinds]int
	GrenadeKind  GrenadeKind
	ThrowWait    float32
	Stats        Stats
}

// NetGrapple is a player's grapple (what it's caught on stays with the host:
// the guest has where it is).
type NetGrapple struct {
	On, Miss             bool
	To                   V3
	Shot, Time, Cooldown float32
}

// NetGrenade is a grenade in flight or stuck.
type NetGrenade struct {
	ID      int
	Owner   int
	Kind    GrenadeKind
	Pos     V3
	Vel     V3
	Age     float32
	Stuck   bool
	StuckTo int // -1 for none
	Offset  V3
	StuckAt float32
}

// NetPickup is a pickup lying about.
type NetPickup struct {
	Weapon   WeaponKind
	Grenade  GrenadeKind
	Count    int
	Ammo     int
	Reserve  int
	At       V3
	Yaw      float32
	Table    bool
	IsGadget bool       `json:",omitempty"`
	Gadget   GadgetKind `json:",omitempty"`
	Spot     int
	Age      float32 `json:"-"` // (the host's business: when a dropped one clears)
}

// Snapshot is the state of the arena at a moment.
type Snapshot struct {
	Time     float32
	Live     bool
	Players  []NetPlayer
	Grenades []NetGrenade
	Pickups  []NetPickup `json:",omitempty"`
	// KeepPickups: the pickups haven't changed since the last snapshot sent
	// (so they're left out, keeping snapshots small).
	KeepPickups bool `json:",omitempty"`
}

// Snapshot captures the arena's state.
func (a *Arena) Snapshot() Snapshot {
	s := Snapshot{Time: a.Time, Live: a.Live}
	for _, p := range a.Players {
		np := NetPlayer{
			Pos: v3(p.Body.Position), Vel: v3(p.Body.Velocity), Yaw: p.Yaw, Pitch: p.Pitch, Recoil: p.recoil,
			Sway: p.sway, Breath: p.Breath, Shield: p.Shield, Health: p.Health, Dead: p.Dead, DiedAt: p.DiedAt,
			Flash: p.Flash, OnGround: p.onGround, Boosted: p.boosted, BoostTop: p.boostTop, SincePad: p.sincePad,
			SinceJump: p.sinceJump, Airborne: p.airborne, SinceLand: p.sinceLand, JumpQueue: p.jumpQueue, OnSlope: p.onSlope,
			Crouch: p.Crouch, Crouched: p.crouched, WasCrouch: p.wasCrouch, Sliding: p.Sliding, SlideTime: p.slideTime,
			SlideCool: p.slideCool, Mantle: p.Mantle,
			Slots: p.Slots, Active: p.Active, Current: p.Current, Switching: p.Switching,
			ADS: p.ADS, Descope: p.descope, HammerSwing: p.Hammer.Swing, HammerStruck: p.Hammer.struck,
			HammerOut: p.HammerOut, ElbowSwing: p.Elbow.Swing, ElbowStruck: p.Elbow.struck, Gadget: p.Gadget,
			Grapple: NetGrapple{On: p.Grapple.On, Miss: p.Grapple.Miss, To: v3(p.Grapple.To), Shot: p.Grapple.Shot,
				Time: p.Grapple.Time, Cooldown: p.Grapple.Cooldown},
			Grenades: p.Grenades, GrenadeKind: p.GrenadeKind, ThrowWait: p.throwWait, Stats: p.Stats,
			Launcher: NetGun{Ammo: p.Launcher.Ammo, Reserve: p.Launcher.Reserve, Reloading: p.Launcher.Reloading,
				Kick: p.Launcher.Kick, Cooldown: p.Launcher.cooldown},
		}
		for k, g := range p.States {
			np.Guns[k] = NetGun{Ammo: g.Ammo, Reserve: g.Reserve, Reloading: g.Reloading, Kick: g.Kick,
				SinceShot: g.SinceShot, Cooldown: g.cooldown, Bloom: g.bloom}
		}
		s.Players = append(s.Players, np)
	}
	for _, g := range a.Grenades {
		ng := NetGrenade{ID: g.ID, Owner: g.Owner.ID, Kind: g.Kind, Pos: v3(g.Body.Position), Vel: v3(g.Body.Velocity),
			Age: g.Age, Stuck: g.Stuck, StuckTo: -1, Offset: v3(g.offset), StuckAt: g.stuckAt}
		if g.StuckTo != nil {
			ng.StuckTo = g.StuckTo.ID
		}
		s.Grenades = append(s.Grenades, ng)
	}
	for _, p := range a.Pickups {
		s.Pickups = append(s.Pickups, NetPickup{Weapon: p.Weapon, Grenade: p.Grenade, Count: p.Count, Ammo: p.Ammo,
			Reserve: p.Reserve, At: v3(p.At), Yaw: p.Yaw, Table: p.Table, IsGadget: p.IsGadget, Gadget: p.Gadget,
			Spot: p.spot, Age: p.Age})
	}
	return s
}

// ApplySnapshot sets the arena to s. The aim of player keepAim (the local
// player on a client, who aims ahead of the host) isn't overwritten; -1
// overwrites everyone's.
func (a *Arena) ApplySnapshot(s *Snapshot, keepAim int) {
	a.Time, a.Live = s.Time, s.Live
	for i, np := range s.Players {
		if i >= len(a.Players) {
			break
		}
		p := a.Players[i]
		if np.Dead != p.Dead {
			if np.Dead {
				a.Phys.Remove(p.Body)
			} else if err := a.Phys.Add(p.Body); err != nil {
				panic(err)
			}
		}
		p.Body.Position, p.Body.Velocity = vec(np.Pos), vec(np.Vel)
		if i != keepAim {
			p.Yaw, p.Pitch = np.Yaw, np.Pitch
		}
		p.recoil, p.sway, p.Breath = np.Recoil, np.Sway, np.Breath
		p.Shield, p.Health, p.Dead, p.DiedAt, p.Flash = np.Shield, np.Health, np.Dead, np.DiedAt, np.Flash
		p.onGround, p.boosted, p.boostTop, p.sincePad, p.sinceJump = np.OnGround, np.Boosted, np.BoostTop, np.SincePad, np.SinceJump
		p.airborne, p.sinceLand, p.jumpQueue, p.onSlope = np.Airborne, np.SinceLand, np.JumpQueue, np.OnSlope
		p.Crouch, p.crouched, p.wasCrouch, p.Sliding = np.Crouch, np.Crouched, np.WasCrouch, np.Sliding
		p.slideTime, p.slideCool, p.Mantle = np.SlideTime, np.SlideCool, np.Mantle
		w := &p.Weapons
		w.Slots, w.Active, w.Current, w.Switching, w.descope = np.Slots, np.Active, np.Current, np.Switching, np.Descope
		if i != keepAim {
			w.ADS = np.ADS // (the local player's sights go up and down at once, predicted)
		}
		w.Hammer = HammerState{Swing: np.HammerSwing, struck: np.HammerStruck}
		w.HammerOut, w.Elbow, w.Gadget = np.HammerOut, ElbowState{Swing: np.ElbowSwing, struck: np.ElbowStruck}, np.Gadget
		g := np.Grapple
		w.Grapple = GrappleState{On: g.On, Miss: g.Miss, To: vec(g.To), Shot: g.Shot, Time: g.Time, Cooldown: g.Cooldown}
		w.Grenades, w.GrenadeKind, w.throwWait = np.Grenades, np.GrenadeKind, np.ThrowWait
		l := np.Launcher
		w.Launcher = LauncherState{Ammo: l.Ammo, Reserve: l.Reserve, Reloading: l.Reloading, Kick: l.Kick, cooldown: l.Cooldown}
		for k, g := range np.Guns {
			w.States[k] = GunState{Ammo: g.Ammo, Reserve: g.Reserve, Reloading: g.Reloading, Kick: g.Kick,
				SinceShot: g.SinceShot, cooldown: g.Cooldown, bloom: g.Bloom}
		}
		p.Stats = np.Stats
	}

	// Grenades: keep the ones still out (so they interpolate), add the new.
	byID := map[int]*Grenade{}
	for _, g := range a.Grenades {
		byID[g.ID] = g
	}
	a.Grenades = a.Grenades[:0]
	for _, ng := range s.Grenades {
		g := byID[ng.ID]
		if g == nil {
			g = &Grenade{ID: ng.ID, Body: physics.NewSphere(grenadeRadius, 0.6)}
			g.Body.Position = vec(ng.Pos)
			g.Body.Teleported()
		}
		g.Owner = a.player(ng.Owner)
		g.Kind, g.Age, g.Stuck, g.offset, g.stuckAt = ng.Kind, ng.Age, ng.Stuck, vec(ng.Offset), ng.StuckAt
		g.StuckTo = a.player(ng.StuckTo)
		g.Body.Teleported() // from where it was drawn to where it is now
		g.Body.Position, g.Body.Velocity = vec(ng.Pos), vec(ng.Vel)
		a.Grenades = append(a.Grenades, g)
	}

	if s.KeepPickups {
		return
	}
	a.Pickups = a.Pickups[:0]
	for _, np := range s.Pickups {
		a.Pickups = append(a.Pickups, &Pickup{Weapon: np.Weapon, Grenade: np.Grenade, Count: np.Count, Ammo: np.Ammo,
			Reserve: np.Reserve, At: vec(np.At), Yaw: np.Yaw, Table: np.Table, IsGadget: np.IsGadget, Gadget: np.Gadget,
			spot: np.Spot, Age: np.Age})
	}
}

// player is player i, or nil.
func (a *Arena) player(i int) *Player {
	if i >= 0 && i < len(a.Players) {
		return a.Players[i]
	}
	return nil
}

func playerID(p *Player) int {
	if p == nil {
		return -1
	}
	return p.ID
}

// NetEvents are a step's Events on the wire: players, chunks and grenades
// by ID.
type NetEvents struct {
	Shots      []NetShot      `json:",omitempty"`
	Hurts      []NetHurt      `json:",omitempty"`
	Kills      []NetKill      `json:",omitempty"`
	Breaks     []NetBreak     `json:",omitempty"`
	Chipped    []NetChip      `json:",omitempty"`
	Smashes    []NetSmash     `json:",omitempty"`
	Explosions []NetExplosion `json:",omitempty"`
	Stuck      []NetStick     `json:",omitempty"`
	Actions    []NetAction    `json:",omitempty"`
	Topples    []NetTopple    `json:",omitempty"`
}

type (
	NetShot struct {
		By, Victim       int
		From, To, Normal V3
		Head             bool
		Chunk            int
		Weapon           WeaponKind
	}
	NetHurt struct {
		Victim, By           int
		Damage               float32
		Head, Popped, Armour bool
		From                 V3
	}
	NetKill struct {
		Victim, By int
		Weapon     WeaponKind
		Head       bool
	}
	NetBreak struct {
		Chunk     int
		Push      V3
		Collapsed bool
	}
	NetChip struct {
		Chunk int
		HP    float32
	}
	NetSmash struct {
		By, Victim int
		At, Normal V3
		Mat        Material
		Light      bool `json:",omitempty"`
	}
	NetExplosion struct {
		At   V3
		By   int
		Kind GrenadeKind
	}
	NetTopple struct {
		Chunks        []int
		Pivot, Axis   V3
		Support, Spin float32
		Lever, Far    float32
		By            int
	}
	NetStick  struct{ Grenade, On int }
	NetAction struct {
		By    int
		Kind  ActionKind
		Value float32
	}
)

// Empty reports whether nothing happened.
func (e *NetEvents) Empty() bool {
	return len(e.Shots)+len(e.Hurts)+len(e.Kills)+len(e.Breaks)+len(e.Chipped)+len(e.Smashes)+
		len(e.Explosions)+len(e.Stuck)+len(e.Actions)+len(e.Topples) == 0
}

func chunkID(c *Chunk) int {
	if c == nil {
		return -1
	}
	return c.ID
}

// Net converts a step's events for the wire.
func (ev *Events) Net() NetEvents {
	var n NetEvents
	for _, s := range ev.Shots {
		n.Shots = append(n.Shots, NetShot{By: playerID(s.By), Victim: playerID(s.Victim), From: v3(s.From), To: v3(s.To),
			Normal: v3(s.Normal), Head: s.Head, Chunk: chunkID(s.Chunk), Weapon: s.Weapon})
	}
	for _, h := range ev.Hurts {
		n.Hurts = append(n.Hurts, NetHurt{Victim: playerID(h.Victim), By: playerID(h.By), Damage: h.Damage, Head: h.Head,
			Popped: h.Popped, Armour: h.Armour, From: v3(h.From)})
	}
	for _, k := range ev.Kills {
		n.Kills = append(n.Kills, NetKill{Victim: playerID(k.Victim), By: playerID(k.By), Weapon: k.Weapon, Head: k.Head})
	}
	for _, b := range ev.Breaks {
		n.Breaks = append(n.Breaks, NetBreak{Chunk: chunkID(b.Chunk), Push: v3(b.Push), Collapsed: b.Collapsed})
	}
	seen := map[*Chunk]bool{}
	for i := len(ev.Chipped) - 1; i >= 0; i-- { // the latest HP of each
		c := ev.Chipped[i]
		if !seen[c] && c.Alive {
			seen[c] = true
			n.Chipped = append(n.Chipped, NetChip{Chunk: c.ID, HP: c.HP})
		}
	}
	for _, s := range ev.Smashes {
		n.Smashes = append(n.Smashes, NetSmash{By: playerID(s.By), Victim: playerID(s.Victim), At: v3(s.At), Normal: v3(s.Normal), Mat: s.Mat, Light: s.Light})
	}
	for _, x := range ev.Explosions {
		n.Explosions = append(n.Explosions, NetExplosion{At: v3(x.At), By: playerID(x.By), Kind: x.Kind})
	}
	for _, s := range ev.Stuck {
		n.Stuck = append(n.Stuck, NetStick{Grenade: s.Grenade.ID, On: playerID(s.On)})
	}
	for _, act := range ev.Actions {
		n.Actions = append(n.Actions, NetAction{By: playerID(act.By), Kind: act.Kind, Value: act.Value})
	}
	for _, t := range ev.Topples {
		nt := NetTopple{Pivot: v3(t.Pivot), Axis: v3(t.Axis), Support: t.Support, Spin: t.Spin, Lever: t.lever, Far: t.far, By: playerID(t.By)}
		for _, c := range t.Chunks {
			nt.Chunks = append(nt.Chunks, c.ID)
		}
		n.Topples = append(n.Topples, nt)
	}
	return n
}

// ApplyEvents plays the host's events into this (client) arena: chunks
// break and collapse here too (throwing their own rubble), damaged chunks
// take the host's HP, and the rest comes back as Events for effects and
// sounds.
func (a *Arena) ApplyEvents(n *NetEvents) Events {
	var ev Events
	chunk := func(id int) *Chunk {
		if id >= 0 && id < len(a.chunks) {
			return a.chunks[id]
		}
		return nil
	}
	for _, c := range n.Chipped {
		if ch := chunk(c.Chunk); ch != nil && ch.Alive {
			ch.HP = c.HP
		}
	}
	for _, b := range n.Breaks {
		c := chunk(b.Chunk)
		if c == nil || !c.Alive {
			continue
		}
		if b.Collapsed {
			a.removeChunk(c, nil)
			drift := mathx.Vec3{a.rng.Float32() - 0.5, -0.5, a.rng.Float32() - 0.5}.Scale(0.8)
			d := a.addDebris(c.Centre, c.Half, c.Mat, drift)
			d.Collapsed = true
			ev.Breaks = append(ev.Breaks, Break{At: c.Centre, Half: c.Half, Mat: c.Mat, Collapsed: true, Chunk: c})
		} else {
			a.breakChunk(c, vec(b.Push), nil, &ev)
		}
		c.Structure.dirty = false // the host decides what falls; it'll say
	}
	// Pieces falling over: they fall here too (and shatter here, cosmetically).
	for _, nt := range n.Topples {
		t := &Topple{ID: a.nextTopple, Pivot: vec(nt.Pivot), Axis: vec(nt.Axis), Support: nt.Support, Spin: nt.Spin,
			lever: nt.Lever, far: nt.Far, By: a.player(nt.By)}
		a.nextTopple++
		t.own = map[*Structure]bool{}
		for _, id := range nt.Chunks {
			if c := chunk(id); c != nil && c.Alive {
				a.removeChunk(c, nil)
				c.Structure.dirty = false
				t.Chunks = append(t.Chunks, c)
				t.own[c.Structure] = true
			}
		}
		if len(t.Chunks) > 0 {
			a.Topples = append(a.Topples, t)
			ev.Topples = append(ev.Topples, t)
		}
	}
	for _, s := range n.Shots {
		ev.Shots = append(ev.Shots, Shot{By: a.player(s.By), Victim: a.player(s.Victim), From: vec(s.From), To: vec(s.To),
			Normal: vec(s.Normal), Head: s.Head, Chunk: chunk(s.Chunk), Weapon: s.Weapon})
	}
	for _, h := range n.Hurts {
		ev.Hurts = append(ev.Hurts, Hurt{Victim: a.player(h.Victim), By: a.player(h.By), Damage: h.Damage, Head: h.Head,
			Popped: h.Popped, Armour: h.Armour, From: vec(h.From)})
	}
	for _, k := range n.Kills {
		ev.Kills = append(ev.Kills, Kill{Victim: a.player(k.Victim), By: a.player(k.By), Weapon: k.Weapon, Head: k.Head})
	}
	for _, s := range n.Smashes {
		ev.Smashes = append(ev.Smashes, Smash{By: a.player(s.By), Victim: a.player(s.Victim), At: vec(s.At), Normal: vec(s.Normal), Mat: s.Mat, Light: s.Light})
	}
	for _, x := range n.Explosions {
		ev.Explosions = append(ev.Explosions, Explosion{At: vec(x.At), By: a.player(x.By), Kind: x.Kind})
	}
	for _, s := range n.Stuck {
		for _, g := range a.Grenades {
			if g.ID == s.Grenade {
				ev.Stuck = append(ev.Stuck, Stick{Grenade: g, On: a.player(s.On)})
			}
		}
	}
	for _, act := range n.Actions {
		if p := a.player(act.By); p != nil {
			ev.Actions = append(ev.Actions, Action{By: p, Kind: act.Kind, Value: act.Value})
		}
	}
	return ev
}

// Predict moves p (a network client's own player) through one step of in
// as the host's Step would: movement, jumps, stairs and ceilings, landing
// and launch pads. Its look should already be applied; nothing else
// happens (the host decides hits, weapons and all the rest). Replaying the
// inputs the host hasn't had yet from its last snapshot puts the player
// where the host will have it.
func (a *Arena) Predict(p *Player, in Input, dt float32) {
	if p.Dead {
		return
	}
	var ev Events
	if !a.traverse(p, dt, &in, &ev) {
		a.movePlayer(p, dt, in, &ev)
		if p.Grapple.On && (in.Jump || !a.pullGrapple(p, dt)) {
			p.Grapple.On = false // (the host decides; this is what it will)
		}
		a.stepUp(p, dt)
	}
	a.Phys.StepBody(p.Body, dt)
	a.headRoom(p)
	a.touchDown(p, dt)
	a.usePads(p, &ev)
}

// StepCosmetic advances a client's copy between the host's updates: the
// players carry on along their velocities, rubble flies and settles, and
// old pieces clear. Nothing that matters to the match happens here. keep
// (the client's own, predicted player; nil for none) isn't moved.
//
// It returns what happened that the game shows: pieces falling over land
// and shatter here in their own time.
func (a *Arena) StepCosmetic(dt float32, keep *Player) Events {
	var pos, vel mathx.Vec3
	var grounded bool
	if keep != nil {
		pos, vel, grounded = keep.Body.Position, keep.Body.Velocity, keep.Body.Grounded
	}
	a.Phys.Update(dt)
	if keep != nil {
		keep.Body.Position, keep.Body.Velocity, keep.Body.Grounded = pos, vel, grounded
	}
	a.ageDebris(dt)
	for _, g := range a.Grenades {
		if !g.Stuck {
			g.Body.Position = g.Body.Position.Add(g.Body.Velocity.Scale(dt))
			g.Body.Velocity[1] -= gravity * dt
		}
	}
	var ev Events
	a.stepTopples(dt, false, &ev) // (the host pushes players; the guest just watches)
	return ev
}

// NetMatch is a match's standing.
type NetMatch struct {
	Round       int
	Phase       Phase
	Timer       float32
	Wins        []int
	RoundWinner int
	Winner      int
}

// Net is the match's standing, for the wire.
func (m *Match) Net() NetMatch {
	return NetMatch{Round: m.Round, Phase: m.Phase, Timer: m.Timer, Wins: append([]int(nil), m.Wins...),
		RoundWinner: m.RoundWinner, Winner: m.Winner}
}

// Apply brings a client's copy of the match to the host's standing,
// starting the next round's site when the host has. It reports whether a
// new round began.
func (m *Match) Apply(n *NetMatch) bool {
	newRound := false
	if n.Round != m.Round && n.Round > 0 {
		m.Round = n.Round - 1
		m.startRound()
		newRound = true
	}
	m.Phase, m.Timer, m.RoundWinner, m.Winner = n.Phase, n.Timer, n.RoundWinner, n.Winner
	copy(m.Wins, n.Wins)
	m.Arena.Live = m.Phase != PhaseCountdown
	return newRound
}
