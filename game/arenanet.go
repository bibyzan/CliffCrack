package game

import (
	"encoding/json"
	"math"

	"CliffCrack/engine/camera"
	"CliffCrack/engine/input"
	"CliffCrack/game/arena"
	"CliffCrack/online"
)

// netPlay is an online match in the Arena mode: the host steps it with
// everyone's input and sends the rest what happened; a guest sends its
// input and plays what it's sent (see package online).
type netPlay struct {
	session *online.Session
	host    bool
	names   []string
	peers   []string      // peer id by player index ("" for us)
	links   []online.Link // the host's link to each guest by player index; a guest's to the host at [0]
	recv    []online.InputReceiver
	sender  online.InputSender
	seq     uint32 // host: steps sent; guest: the newest snapshot applied
	left    []bool // players whose link went down
	over    string // why the match ended for us (the host left)

	acc      float32     // time towards the next tick
	pending  arena.Input // our input for the next tick
	batch    online.InputBatch
	pred     online.Predictor // guest: our own movement, predicted
	interp   online.Interp    // guest: everyone else, smoothed
	standing arena.NetMatch   // host: the match standing last sent
	pickups  []byte           // host: the pickups last sent
}

const snapEvery = 2 // host steps per snapshot

// Online reports whether the match is online.
func (m *Arena) Online() bool { return m.net != nil }

// startOnline begins an online match from the host's start message.
func (m *Arena) startOnline(s *online.Session, start *online.StartMsg) {
	room, _ := s.Room()
	np := &netPlay{session: s, host: start.You == 0, names: start.Names, recv: make([]online.InputReceiver, len(start.Names)),
		links: make([]online.Link, len(start.Names)), peers: make([]string, len(start.Names)), left: make([]bool, len(start.Names))}
	// A rematch carries on the same streams: the guest's input sequence and
	// press counts, the host's view of them, and the snapshot numbering.
	// Starting them over would let a packet sent just before the rematch
	// (numbered higher than anything new) arrive first and make every later
	// input look stale: the guest's controls would freeze.
	if old := m.net; old != nil && old.session == s && len(old.recv) == len(np.recv) {
		np.sender, np.recv, np.seq, np.left, np.batch = old.sender, old.recv, old.seq, old.left, old.batch
	}
	for i, mb := range room.Members {
		if i < len(np.peers) && mb.ID != s.ID() {
			np.peers[i] = mb.ID
		}
	}
	if np.host {
		for i, id := range np.peers {
			if id != "" {
				np.links[i], _ = s.Link(id)
			}
		}
	} else {
		np.links[0], _ = s.Link(room.Host)
	}
	m.net = np
	m.Practice = false
	local = start.You
	m.match = arena.NewMatch(start.Seed, len(start.Names))
	m.bots = make([]*arena.Bot, len(start.Names))
	m.inputs = make([]arena.Input, len(start.Names))
	m.strides = make([]float32, len(start.Names))
	m.buildLevel()
	m.feed = m.feed[:0]
	m.newRound()
	m.wantGadget = m.settings.Gadget + 1
}

// leaveOnline ends our part in the match and the room.
func (m *Arena) leaveOnline() {
	np := m.net
	if np == nil {
		return
	}
	for _, l := range np.links {
		if l != nil {
			l.Send(online.Reliable, online.Encode(online.Msg{Bye: true}))
		}
	}
	np.session.Leave()
	np.session.Close()
	m.net = nil
	local = 0
}

// Online matches run on a fixed tick, the same on every machine, so the
// host takes each guest's inputs one per tick exactly as the guest made
// them (and the guest's prediction matches). Looking still happens every
// frame, locally.
const (
	netTick     = float32(1.0 / 60)
	maxNetTicks = 5 // per frame, before the backlog is dropped
)

// updateOnline is Update for an online match.
func (m *Arena) updateOnline(dt float32, in *input.State, mouseFree bool) {
	np := m.net
	if np.over != "" {
		if !m.inputBlocked && (confirmPressed(in) || in.MousePressed(input.MouseLeft)) {
			m.wantsMenu = true
		}
		m.effects(dt, arena.Events{})
		return
	}
	var c arena.Input
	if !m.inputBlocked {
		c = m.input(in, dt, mouseFree)
	}
	// Look now; everything else waits for the tick (presses held until then).
	me := m.me()
	if !me.Dead {
		me.Yaw = float32(math.Remainder(float64(me.Yaw+c.Look[0]), 2*math.Pi))
		me.Pitch = clampf(me.Pitch+c.Look[1], -camera.MaxPitch, camera.MaxPitch)
	}
	c.Look = [2]float32{}
	np.pending = mergePresses(np.pending, c)

	// The match steps on the net tick, drawn between ticks (see alpha).
	m.sim().Phys.PrevPerUpdate = true
	var ev arena.Events
	if np.host {
		ev = m.hostFrame(dt)
	} else {
		ev = m.guestFrame(dt)
	}
	m.effects(dt, ev)
	m.announce()
}

// mergePresses is the latest input's held buttons, with any press since
// the last tick kept.
func mergePresses(pending, c arena.Input) arena.Input {
	c.Jump = c.Jump || pending.Jump
	c.FirePressed = c.FirePressed || pending.FirePressed
	c.Reload = c.Reload || pending.Reload
	c.Melee = c.Melee || pending.Melee
	c.Throw = c.Throw || pending.Throw
	c.SwitchGrenade = c.SwitchGrenade || pending.SwitchGrenade
	c.Interact = c.Interact || pending.Interact
	if c.Select == 0 {
		c.Select = pending.Select
	}
	if c.Cycle == 0 {
		c.Cycle = pending.Cycle
	}
	return c
}

// ticks runs tick for each fixed tick due this frame, the first with the
// frame's presses.
func (np *netPlay) ticks(dt float32, tick func(in arena.Input)) {
	np.acc += dt
	n := 0
	for np.acc >= netTick {
		np.acc -= netTick
		if n++; n > maxNetTicks {
			np.acc = 0 // too far behind (a hitch): let it go
			break
		}
		tick(np.pending)
		// Held buttons carry on; presses are used up.
		np.pending = arena.Input{Move: np.pending.Move, Fire: np.pending.Fire, Aim: np.pending.Aim, Sprint: np.pending.Sprint}
	}
}

// hostFrame reads the guests' inputs and runs the ticks due: stepping the
// match with everyone's input and sending the guests what happened.
func (m *Arena) hostFrame(dt float32) arena.Events {
	np := m.net
	for i, l := range np.links {
		if l == nil || np.left[i] {
			continue
		}
		m.drain(l, func(msg online.Msg) {
			for _, in := range msg.Inputs {
				np.recv[i].Receive(in)
			}
			if msg.Bye {
				m.playerLeft(i)
			}
		}, func() { m.playerLeft(i) })
	}
	if m.match.Phase == arena.PhaseMatchOver && m.match.Timer < -1 &&
		(confirmPressed(m.lastIn) || (m.touchOn && m.lastIn.MousePressed(input.MouseLeft))) {
		m.rematch()
		return arena.Events{}
	}
	var all arena.Events
	np.ticks(dt, func(mine arena.Input) {
		a := m.sim()
		for i, p := range a.Players {
			switch {
			case i == local:
				m.inputs[i] = mine
			case np.links[i] != nil && !np.left[i]:
				m.inputs[i] = np.recv[i].Input(p)
			default:
				m.inputs[i] = arena.Input{}
			}
		}
		ev := m.match.Step(netTick, m.inputs)
		np.seq++
		m.sendTick(ev, m.match.Arena != a)
		if m.match.Arena != m.round {
			m.newRound()
			all = arena.Events{} // the last round's are gone with it
			return
		}
		all.Merge(ev)
	})
	return all
}

// sendTick sends the guests a tick: its events (reliably, when there are
// any, or the standing changed) and every snapEvery ticks a snapshot.
func (m *Arena) sendTick(ev arena.Events, newRound bool) {
	np := m.net
	standing := m.match.Net()
	var frame, snap []byte
	net := ev.Net()
	if !net.Empty() || newRound || standingChanged(np.standing, standing) {
		frame = online.Encode(online.Msg{Frame: &online.FrameMsg{Events: net, Match: standing}})
		np.standing = standing
	}
	if np.seq%snapEvery == 0 || newRound {
		s := online.SnapMsg{Round: m.match.Round, Seq: np.seq, Match: standing, Snap: m.match.Arena.Snapshot(),
			Acks: make([]uint32, len(np.recv))}
		for i := range np.recv {
			s.Acks[i] = np.recv[i].Acked()
		}
		// Pickups change seldom: send them when they do (and every second,
		// in case one went missing).
		pick, _ := json.Marshal(s.Snap.Pickups)
		if string(pick) == string(np.pickups) && np.seq%(snapEvery*30) != 0 && !newRound {
			s.Snap.Pickups, s.Snap.KeepPickups = nil, true
		}
		np.pickups = pick
		snap = online.Encode(online.Msg{Snap: &s})
	}
	for i, l := range np.links {
		if l == nil || np.left[i] {
			continue
		}
		if frame != nil {
			if err := l.Send(online.Reliable, frame); err != nil {
				m.playerLeft(i)
				continue
			}
		}
		if snap != nil {
			l.Send(online.Fast, snap)
		}
	}
}

// standingChanged reports a change in the match other than its clock.
func standingChanged(a, b arena.NetMatch) bool {
	if a.Round != b.Round || a.Phase != b.Phase || a.RoundWinner != b.RoundWinner || a.Winner != b.Winner || len(a.Wins) != len(b.Wins) {
		return true
	}
	for i := range a.Wins {
		if a.Wins[i] != b.Wins[i] {
			return true
		}
	}
	return false
}

// guestFrame plays what the host sent, then runs the ticks due: sending
// our input and predicting our own movement. Everyone else is drawn
// smoothly a little behind the host.
func (m *Arena) guestFrame(dt float32) arena.Events {
	np := m.net
	var ev arena.Events
	var snap *online.SnapMsg
	lost := false
	m.drain(np.links[0], func(msg online.Msg) {
		switch {
		case msg.Frame != nil:
			ev.Merge(m.sim().ApplyEvents(&msg.Frame.Events))
			if m.match.Apply(&msg.Frame.Match) {
				m.newRound()
				m.net.pred.Reset()
				m.net.interp.Reset()
				ev = arena.Events{}
			}
		case msg.Snap != nil && msg.Snap.Seq > m.net.seq:
			m.net.seq, snap = msg.Snap.Seq, msg.Snap
		case msg.Start != nil: // a rematch
			m.startOnline(np.session, msg.Start)
			ev = arena.Events{}
		case msg.Bye:
			lost = true
		}
	}, func() { lost = true })
	np = m.net // (a rematch replaces it)
	if lost && np.over == "" {
		np.over = "The host left the match."
	}
	a, me := m.sim(), m.me()
	if snap != nil && snap.Round == m.match.Round {
		before := me.Body.Position
		a.ApplySnapshot(&snap.Snap, local)
		if m.match.Phase == snap.Match.Phase {
			m.match.Timer = snap.Match.Timer
		}
		if local < len(snap.Acks) {
			np.pred.Reconcile(a, me, snap.Acks[local], before)
		}
		np.interp.Add(&snap.Snap)
	}
	np.ticks(dt, func(in arena.Input) {
		// Rubble first: stepping the world marks every body's last position,
		// ours included, and ours should be where prediction moves it from.
		a.StepCosmetic(netTick, me)
		if l := np.links[0]; l != nil {
			in.ViewTime = np.interp.ViewTime() // judge our shots where we see everyone
			msg := np.sender.Next(in, me.Yaw, me.Pitch)
			l.Send(online.Fast, online.Encode(online.Msg{Inputs: np.batch.Add(msg)}))
			if m.match.Phase != arena.PhaseCountdown && m.match.Phase != arena.PhaseMatchOver {
				np.pred.Step(a, me, msg.Seq, in, netTick)
			}
		}
		predictSights(me, in, netTick)
	})
	np.pred.Ease(dt)
	np.interp.Tick(dt)
	for i, p := range a.Players {
		if i != local && !p.Dead {
			np.interp.Place(i, p)
		}
	}
	return ev
}

// predictSights raises and lowers our sights at once, rather than a round
// trip later when the host says so.
func predictSights(me *arena.Player, in arena.Input, dt float32) {
	g, s := me.Gun()
	if g == nil {
		me.ADS = 0
		return
	}
	if in.Aim && s.Reloading == 0 && me.Switching == 0 && !me.Swinging() && !(in.Sprint && in.Move[1] > 0.3) {
		me.ADS = min(me.ADS+dt/g.ADSTime, 1)
	} else {
		me.ADS = max(me.ADS-dt/g.ADSTime, 0)
	}
}

// drain handles every message waiting on l; gone is called if it's closed.
func (m *Arena) drain(l online.Link, handle func(online.Msg), gone func()) {
	if l == nil {
		return
	}
	for {
		select {
		case p, ok := <-l.Recv():
			if !ok {
				gone()
				return
			}
			if msg, err := online.Decode(p.Data); err == nil {
				handle(msg)
			}
			continue
		default:
		}
		return
	}
}

// playerLeft notes a guest gone (host): their player stands still.
func (m *Arena) playerLeft(i int) {
	np := m.net
	if np.left[i] {
		return
	}
	np.left[i] = true
	m.feed = append(m.feed, feedLine{note: np.names[i] + " left"})
}

// rematch (host) starts a new match with everyone still here.
func (m *Arena) rematch() {
	np := m.net
	start := online.StartMsg{Seed: m.match.Seed + 1, Names: np.names}
	for i, l := range np.links {
		if l != nil && !np.left[i] {
			s := start
			s.You = i
			l.Send(online.Reliable, online.Encode(online.Msg{Start: &s}))
		}
	}
	start.You = 0
	m.startOnline(np.session, &start)
}

// netName is a player's name online.
func (m *Arena) netName(p *arena.Player) (string, bool) {
	if m.net == nil || p == nil || p.ID >= len(m.net.names) {
		return "", false
	}
	return m.net.names[p.ID], true
}
