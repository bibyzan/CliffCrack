package online

import (
	"io"
	"log"
	"math/rand/v2"
	"net/http/httptest"
	"testing"
	"time"

	"CliffCrack/coordinator"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/physics"
	"CliffCrack/game/arena"
)

func TestPressesSurviveLostPackets(t *testing.T) {
	var send InputSender
	var recv InputReceiver
	p := &arena.Player{}
	// A jump and a reload, in packets that are lost; then one that arrives.
	send.Next(arena.Input{Jump: true}, 0, 0)
	send.Next(arena.Input{Reload: true, Select: 2}, 0.5, 0.1)
	recv.Receive(send.Next(arena.Input{Fire: true}, 0.5, 0.1))
	in := recv.Input(p)
	if !in.Jump || !in.Reload || in.Select != 2 || !in.Fire {
		t.Errorf("after lost packets: %+v; want the jump, reload and slot 2 kept, and the trigger held", in)
	}
	if in.Look[0] != 0.5 || abs(in.Look[1]-0.1) > 1e-6 {
		t.Errorf("look %v, want to turn to the guest's aim", in.Look)
	}
	// Next step, nothing new: presses don't repeat, holds do.
	in = recv.Input(p)
	if in.Jump || in.Reload || in.Select != 0 || !in.Fire {
		t.Errorf("a step later: %+v; want no repeated presses", in)
	}
	// An old packet arriving late is ignored.
	recv.Receive(InputMsg{Seq: 1, Jump: 99})
	if in := recv.Input(p); in.Jump {
		t.Error("a stale packet counted")
	}
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// TestSessionsFindEachOtherAndLink runs a coordinator and two sessions: one
// hosts a room, the other finds it in the list and joins, and they end up
// linked over WebRTC.
func TestSessionsFindEachOtherAndLink(t *testing.T) {
	c := coordinator.New()
	c.Log = log.New(io.Discard, "", 0)
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	server := Server(srv.URL)
	cfg := RTCConfig{Loopback: true}

	host, err := Connect(server, "Ben", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	host.Create("Chasm", 2)
	waitFor(t, "the room", func() bool { _, ok := host.Room(); return ok })

	guest, err := Connect(server, "Sam", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	rooms, err := ListRooms(t.Context(), server)
	if err != nil || len(rooms) != 1 || rooms[0].Name != "Chasm" {
		t.Fatalf("listed %+v (%v)", rooms, err)
	}
	guest.Join(rooms[0].ID)

	waitFor(t, "the host's link to the guest", func() bool { _, up := host.Link(guest.ID()); return up })
	waitFor(t, "the guest's link to the host", func() bool { _, up := guest.Link(host.ID()); return up })
	hl, _ := host.Link(guest.ID())
	gl, _ := guest.Link(host.ID())
	hl.Send(Reliable, Encode(Msg{Start: &StartMsg{Seed: 7, Names: []string{"Ben", "Sam"}, You: 1}}))
	select {
	case p := <-gl.Recv():
		m, err := Decode(p.Data)
		if err != nil || m.Start == nil || m.Start.You != 1 || m.Start.Names[1] != "Sam" {
			t.Errorf("start: %+v (%v)", m, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the start didn't arrive")
	}

	// The guest leaving drops the host's link.
	guest.Leave()
	waitFor(t, "the host to drop the link", func() bool { _, up := host.Link(guest.ID()); return !up })
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestAMatchOverALossyLink plays a match over an in-memory link that loses
// a fifth of the fast packets: the guest's copy of the site still matches
// the host's exactly, since the destruction travels reliably.
func TestAMatchOverALossyLink(t *testing.T) {
	hostEnd, guestEnd := MemLinks()
	rng := rand.New(rand.NewPCG(1, 2))
	hostEnd.(*memLink).drop = func() bool { return rng.IntN(5) == 0 }
	guestEnd.(*memLink).drop = func() bool { return rng.IntN(5) == 0 }

	host, guest := arena.NewMatch(5, 2), arena.NewMatch(5, 2)
	bots := []*arena.Bot{arena.NewBot(1), arena.NewBot(2)}
	var sender InputSender
	var receiver InputReceiver
	var batch InputBatch
	const dt = 1.0 / 60
	fired, round := 0, host.Arena // the guest's shots, over the rounds
	for step := 0; step < 60*30; step++ {
		if host.Arena != round {
			fired += round.Players[1].ShotsFired
			round = host.Arena
		}
		// The guest (player 1) plays by bot, through the link.
		g := guest.Arena
		gin := bots[1].Think(g, g.Players[1], dt)
		yaw, pitch := g.Players[1].Yaw+gin.Look[0], g.Players[1].Pitch+gin.Look[1]
		guestEnd.Send(Fast, Encode(Msg{Inputs: batch.Add(sender.Next(gin, yaw, pitch))}))

		// The host takes whatever arrived, steps and sends.
	drain:
		for {
			select {
			case p := <-hostEnd.Recv():
				m, _ := Decode(p.Data)
				for _, in := range m.Inputs {
					receiver.Receive(in)
				}
			default:
				break drain
			}
		}
		h := host.Arena
		inputs := []arena.Input{bots[0].Think(h, h.Players[0], dt), receiver.Input(h.Players[1])}
		ev := host.Step(dt, inputs)
		hostEnd.Send(Reliable, Encode(Msg{Frame: &FrameMsg{Events: ev.Net(), Match: host.Net()}}))
		hostEnd.Send(Fast, Encode(Msg{Snap: &SnapMsg{Round: host.Round, Seq: uint32(step), Snap: h.Snapshot()}}))

		// The guest applies it.
		for {
			var p Packet
			select {
			case p = <-guestEnd.Recv():
			default:
			}
			if p.Data == nil {
				break
			}
			m, _ := Decode(p.Data)
			switch {
			case m.Frame != nil:
				guest.Arena.ApplyEvents(&m.Frame.Events)
				guest.Apply(&m.Frame.Match)
			case m.Snap != nil && m.Snap.Round == guest.Round:
				guest.Arena.ApplySnapshot(&m.Snap.Snap, 1)
			}
		}
		guest.Arena.StepCosmetic(dt, nil)
	}
	if host.Round != guest.Round {
		t.Fatalf("host round %d, guest %d", host.Round, guest.Round)
	}
	if h, g := host.Arena.Standing(), guest.Arena.Standing(); h != g {
		t.Errorf("the site: host %.4f standing, guest %.4f", h, g)
	}
	fired += host.Arena.Players[1].ShotsFired
	if fired == 0 {
		t.Errorf("the guest never fired through the link, in %d rounds", host.Round)
	}
	if h, g := host.Arena.Players[1].ShotsFired, guest.Arena.Players[1].ShotsFired; h > 0 && g == 0 {
		t.Errorf("the guest never fired through the link (host saw %d, guest %d)", h, g)
	}
}

func ptr[T any](v T) *T { return &v }

// TestInputsSurviveARematch: across a rematch the guest's input stream
// carries on (as the game does), so a packet sent just before the rematch
// arriving late can't make the new match's inputs look stale.
func TestInputsSurviveARematch(t *testing.T) {
	var send InputSender
	var recv InputReceiver
	p := &arena.Player{}
	for range 100 {
		recv.Receive(send.Next(arena.Input{}, 0, 0))
		recv.Input(p)
	}
	straggler := send.Next(arena.Input{}, 0, 0) // in flight as the rematch starts
	// The rematch: the same sender and receiver carry on.
	recv.Receive(straggler)
	recv.Receive(send.Next(arena.Input{Jump: true, Move: [2]float32{0, 1}}, 0, 0))
	recv.Input(p) // the straggler's step
	if in := recv.Input(p); !in.Jump || in.Move[1] != 1 {
		t.Errorf("after the rematch: %+v; want the new jump and movement", in)
	}

	// What went wrong before: a fresh stream after a straggler. The host
	// uses the fresh input, then the straggler, and after that every new
	// input looks old: the guest's controls freeze.
	var fresh InputSender
	var reset InputReceiver
	reset.Receive(straggler)
	reset.Receive(fresh.Next(arena.Input{}, 0, 0))
	reset.Input(p)
	reset.Input(p)
	reset.Receive(fresh.Next(arena.Input{Jump: true}, 0, 0))
	if in := reset.Input(p); in.Jump {
		t.Error("expected a restarted stream to be shadowed by the straggler (the bug this guards)")
	}
}

// TestPredictionTracksTheHost plays a guest through a link with a delay
// each way (steady, then jittery with lost packets, like a phone hotspot):
// running, turning and jumping. With prediction, where the guest has its
// player after each input stays close to where the host has it after the
// same input; without, it's a round trip behind.
func TestPredictionTracksTheHost(t *testing.T) {
	for _, c := range []struct {
		name           string
		lag, jitter    int // steps each way, and up to this many more
		loss           int // 1 in loss inputs lost (0: none)
		worst, typical float32
	}{
		{"steady 100 ms", 6, 0, 0, 0.05, 0.05},
		{"hotspot: 60-150 ms, 10% loss", 4, 5, 10, 1.0, 0.25},
	} {
		t.Run(c.name, func(t *testing.T) { predictionOver(t, c.lag, c.jitter, c.loss, c.worst, c.typical) })
	}
}

func predictionOver(t *testing.T, lag, jitter, loss int, worstOK, typicalOK float32) {
	rng := rand.New(rand.NewPCG(9, 9))
	delay := func() int { return lag + rng.IntN(jitter+1) }
	const dt = float32(1.0 / 60)
	host, guest := arena.NewMatch(3, 2), arena.NewMatch(3, 2)
	for _, m := range []*arena.Match{host, guest} {
		m.Phase, m.Arena.Live = arena.PhaseFight, true
	}
	hp, gp := host.Arena.Players[1], guest.Arena.Players[1]
	var send InputSender
	var recv InputReceiver
	var pred Predictor
	var batch InputBatch
	type inFlight struct {
		due int
		in  []InputMsg
		snp SnapMsg
	}
	var up, down []inFlight
	hostAt := map[uint32]mathx.Vec3{} // host's position after each input
	guestAt := map[uint32]mathx.Vec3{}
	var worst, typical, naive float32
	var lastSnap uint32
	n := 0
	for step := 0; step < 60*6; step++ {
		// The guest: aim, move, send, predict.
		in := arena.Input{Move: [2]float32{0.3, 1}, Sprint: true, Jump: step%70 == 0}
		gp.Yaw += 0.01
		msg := send.Next(in, gp.Yaw, gp.Pitch)
		if loss == 0 || rng.IntN(loss) != 0 {
			up = append(up, inFlight{due: step + delay(), in: batch.Add(msg)})
		} else {
			batch.Add(msg) // lost, but it goes again in the next packets
		}
		pred.Step(guest.Arena, gp, msg.Seq, in, dt)
		guestAt[msg.Seq] = gp.Body.Position

		// The host: take what's arrived, step, snapshot.
		keep := up[:0]
		for _, f := range up {
			if f.due <= step {
				for _, in := range f.in {
					recv.Receive(in)
				}
			} else {
				keep = append(keep, f)
			}
		}
		up = keep
		inputs := []arena.Input{{}, recv.Input(hp)}
		host.Step(dt, inputs)
		ack := recv.Acked()
		hostAt[ack] = hp.Body.Position
		down = append(down, inFlight{due: step + delay(), snp: SnapMsg{Seq: uint32(step), Acks: []uint32{0, ack}, Snap: host.Arena.Snapshot()}})

		// The guest gets the snapshots due.
		var arrived []SnapMsg
		keepDown := down[:0]
		for _, f := range down {
			if f.due <= step {
				arrived = append(arrived, f.snp)
			} else {
				keepDown = append(keepDown, f)
			}
		}
		down = keepDown
		for _, s := range arrived {
			if s.Seq <= lastSnap && lastSnap > 0 {
				continue // older than one already applied
			}
			lastSnap = s.Seq
			before := gp.Body.Position
			guest.Arena.ApplySnapshot(&s.Snap, 1)
			pred.Reconcile(guest.Arena, gp, s.Acks[1], before)
			if h, ok := hostAt[s.Acks[1]]; ok && step > 60 {
				// Compare where the guest had predicted it for that input
				// with where the host put it; and how far behind the plain
				// snapshot (no prediction) would have shown it.
				e := guestAt[s.Acks[1]].Sub(h).Len()
				worst = max(worst, e)
				typical += e
				naive += h.Sub(guestAt[msg.Seq]).Len()
				n++
			}
		}
		pred.Ease(dt)
	}
	naive /= float32(n)
	typical /= float32(n)
	t.Logf("prediction error: %.3f m on average, %.3f m at worst; without prediction the player would show %.2f m behind on average",
		typical, worst, naive)
	if worst > worstOK || typical > typicalOK {
		t.Errorf("prediction drifted from the host: %.2f m on average, %.2f m at worst", typical, worst)
	}
	if naive < 1 {
		t.Errorf("the test isn't moving fast enough to show lag (%.2f m)", naive)
	}
}

func TestSnapshotsCompress(t *testing.T) {
	m := arena.NewMatch(3, 2)
	for range 300 {
		m.Step(1.0/60, nil)
	}
	msg := Msg{Snap: &SnapMsg{Round: 1, Seq: 9, Acks: []uint32{0, 7}, Snap: m.Arena.Snapshot()}}
	data := Encode(msg)
	back, err := Decode(data)
	if err != nil || back.Snap == nil || back.Snap.Seq != 9 || len(back.Snap.Snap.Players) != 2 {
		t.Fatalf("round trip: %+v, %v", back.Snap, err)
	}
	t.Logf("a snapshot is %d bytes on the wire", len(data))
	if len(data) > 1100 {
		t.Errorf("a snapshot is %d bytes: more than one packet", len(data))
	}
}

// The guest's view time is the host's time, interpDelay back, and follows
// a new round's clock.
func TestInterpViewTime(t *testing.T) {
	var ip Interp
	if ip.ViewTime() != 0 {
		t.Error("a view time before any snapshot")
	}
	for i := range 60 {
		ip.Tick(1.0 / 60)
		ip.Add(&arena.Snapshot{Time: 10 + float32(i)/60, Players: []arena.NetPlayer{{}}})
	}
	if v := ip.ViewTime(); abs(v-(10+59.0/60-interpDelay)) > 0.02 {
		t.Errorf("view time %.3f, want ~%.3f", v, 10+59.0/60-interpDelay)
	}
	ip.Reset()
	ip.Tick(1.0 / 60)
	ip.Add(&arena.Snapshot{Time: 0.5, Players: []arena.NetPlayer{{}}})
	if v := ip.ViewTime(); abs(v-(0.5-interpDelay)) > 0.02 {
		t.Errorf("after a new round: view time %.3f, want ~%.3f", v, 0.5-interpDelay)
	}
}

// TestInterpIsSmoothUnderJitter feeds Interp snapshots of a player running
// in a straight line at 6 m/s, 30 a second, arriving with hotspot-like
// jitter (0-90 ms), and draws it every frame of a 60 Hz display: its speed
// on screen, frame to frame, should stay close to 6 m/s (no skips or stalls).
func TestInterpIsSmoothUnderJitter(t *testing.T) {
	const speed = 6
	rng := rand.New(rand.NewPCG(4, 4))
	var ip Interp
	type pending struct {
		at   float32 // our time it arrives
		snap arena.Snapshot
	}
	var queue []pending
	p := &arena.Player{Body: physics.NewSphere(0.4, 80)}
	var worst, last float32
	var clock float32
	for frame := 0; frame < 60*20; frame++ {
		dt := float32(1.0 / 60)
		clock += dt
		ip.Tick(dt)
		if frame%2 == 0 { // the host sends at 30 Hz
			host := clock
			queue = append(queue, pending{at: clock + 0.03 + rng.Float32()*0.09, snap: arena.Snapshot{Time: host,
				Players: []arena.NetPlayer{{Pos: arena.V3{host * speed, 0, 0}, Vel: arena.V3{speed, 0, 0}}}}})
		}
		keep := queue[:0]
		for _, q := range queue {
			if q.at <= clock {
				ip.Add(&q.snap)
			} else {
				keep = append(keep, q)
			}
		}
		queue = keep
		if !ip.Place(0, p) {
			continue
		}
		x := p.Body.Position[0]
		if frame > 120 {
			v := (x - last) / dt
			worst = max(worst, abs(v-speed))
		}
		last = x
	}
	t.Logf("worst on-screen speed error %.2f m/s (of %d m/s)", worst, speed)
	if worst > 1.5 {
		t.Errorf("the player skips or stalls: speed on screen off by up to %.1f m/s", worst)
	}
}

// TestPredictedViewIsSmooth runs a guest's own player on the fixed net tick
// (rubble, then prediction, as the game does) and draws it every frame of
// an uneven display, between the last two ticks: running flat out, its
// speed on screen should hardly waver, frames with no tick or two alike.
func TestPredictedViewIsSmooth(t *testing.T) {
	const tick = float32(1.0 / 60)
	m := arena.NewMatch(3, 2)
	m.Phase, m.Arena.Live = arena.PhaseFight, true
	a, p := m.Arena, m.Arena.Players[1]
	a.Phys.PrevPerUpdate = true // as online
	var pred Predictor
	rng := rand.New(rand.NewPCG(7, 7))
	var acc, worst, speed float32
	var last mathx.Vec3
	var seq uint32
	for frame := 0; frame < 60*6; frame++ {
		dt := 1/75.0 + rng.Float32()*(1/45.0-1/75.0) // 45-75 fps, all over the place
		for acc += dt; acc >= tick; acc -= tick {
			a.StepCosmetic(tick, p)
			seq++
			pred.Step(a, p, seq, arena.Input{Move: [2]float32{0, 1}}, tick)
		}
		drawn, _ := p.Body.Interpolated(acc / tick)
		if frame > 240 { // off the launch pad, landed and up to running speed
			v := drawn.Sub(last).Len() / dt
			if speed == 0 {
				speed = p.Body.Velocity.Len()
			}
			worst = max(worst, abs(v-speed))
		}
		last = drawn
	}
	t.Logf("running at %.2f m/s: worst on-screen speed error %.2f m/s", speed, worst)
	if speed < 1 || worst > speed*0.15 {
		t.Errorf("the view stutters: speed on screen off by up to %.2f m/s of %.2f", worst, speed)
	}
}
