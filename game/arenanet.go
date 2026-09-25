package game

import (
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
		np.sender, np.recv, np.seq, np.left = old.sender, old.recv, old.seq, old.left
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
	if np.host {
		m.hostStep(dt, c)
	} else {
		m.guestStep(dt, c)
	}
	m.announce()
}

// hostStep reads the guests' inputs, steps the match and sends the result.
func (m *Arena) hostStep(dt float32, mine arena.Input) {
	np := m.net
	a := m.sim()
	for i, l := range np.links {
		if l == nil || np.left[i] {
			continue
		}
		m.drain(l, func(msg online.Msg) {
			switch {
			case msg.Input != nil:
				np.recv[i].Receive(*msg.Input)
			case msg.Bye:
				m.playerLeft(i)
			}
		}, func() { m.playerLeft(i) })
	}
	if m.match.Phase == arena.PhaseMatchOver && m.match.Timer < -1 && confirmPressed(m.lastIn) {
		m.rematch()
		return
	}
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
	ev := m.match.Step(dt, m.inputs)
	np.seq++
	frame := online.Encode(online.Msg{Frame: &online.FrameMsg{Events: ev.Net(), Match: m.match.Net()}})
	var snap []byte
	if np.seq%snapEvery == 0 || m.match.Arena != a {
		snap = online.Encode(online.Msg{Snap: &online.SnapMsg{Round: m.match.Round, Seq: np.seq, Snap: m.match.Arena.Snapshot()}})
	}
	for i, l := range np.links {
		if l == nil || np.left[i] {
			continue
		}
		if err := l.Send(online.Reliable, frame); err != nil {
			m.playerLeft(i)
			continue
		}
		if snap != nil {
			l.Send(online.Fast, snap)
		}
	}
	m.effects(dt, ev)
	if m.match.Arena != m.round {
		m.newRound()
	}
}

// guestStep sends our input to the host and plays what the host sent.
func (m *Arena) guestStep(dt float32, mine arena.Input) {
	np := m.net
	me := m.me()
	// Aim here and now, and tell the host where we're aiming.
	if !me.Dead {
		me.Yaw = float32(math.Remainder(float64(me.Yaw+mine.Look[0]), 2*math.Pi))
		me.Pitch = clampf(me.Pitch+mine.Look[1], -camera.MaxPitch, camera.MaxPitch)
	}
	if l := np.links[0]; l != nil {
		msg := np.sender.Next(mine, me.Yaw, me.Pitch)
		l.Send(online.Fast, online.Encode(online.Msg{Input: &msg}))
	}

	var ev arena.Events
	lost := false
	m.drain(np.links[0], func(msg online.Msg) {
		switch {
		case msg.Frame != nil:
			ev.Merge(m.sim().ApplyEvents(&msg.Frame.Events))
			if m.match.Apply(&msg.Frame.Match) {
				m.newRound()
			}
		case msg.Snap != nil && msg.Snap.Round == m.match.Round && msg.Snap.Seq > np.seq:
			np.seq = msg.Snap.Seq
			m.sim().ApplySnapshot(&msg.Snap.Snap, local)
		case msg.Start != nil: // a rematch
			m.startOnline(np.session, msg.Start)
		case msg.Bye:
			lost = true
		}
	}, func() { lost = true })
	if lost && np.over == "" {
		np.over = "The host left the match."
	}
	m.sim().StepCosmetic(dt)
	m.effects(dt, ev)
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
	m.feed = append(m.feed, feedLine{text: np.names[i] + " left"})
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
