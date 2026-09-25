package online

import (
	"io"
	"log"
	"math/rand/v2"
	"net/http/httptest"
	"testing"
	"time"

	"CliffCrack/coordinator"
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
	const dt = 1.0 / 60
	for step := 0; step < 60*30; step++ {
		// The guest (player 1) plays by bot, through the link.
		g := guest.Arena
		gin := bots[1].Think(g, g.Players[1], dt)
		yaw, pitch := g.Players[1].Yaw+gin.Look[0], g.Players[1].Pitch+gin.Look[1]
		guestEnd.Send(Fast, Encode(Msg{Input: ptr(sender.Next(gin, yaw, pitch))}))

		// The host takes whatever arrived, steps and sends.
	drain:
		for {
			select {
			case p := <-hostEnd.Recv():
				if m, _ := Decode(p.Data); m.Input != nil {
					receiver.Receive(*m.Input)
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
		guest.Arena.StepCosmetic(dt)
	}
	if host.Round != guest.Round {
		t.Fatalf("host round %d, guest %d", host.Round, guest.Round)
	}
	if h, g := host.Arena.Standing(), guest.Arena.Standing(); h != g {
		t.Errorf("the site: host %.4f standing, guest %.4f", h, g)
	}
	if h, g := host.Arena.Players[1].ShotsFired, guest.Arena.Players[1].ShotsFired; h == 0 {
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
	if in := recv.Input(p); !in.Jump || in.Move[1] != 1 {
		t.Errorf("after the rematch: %+v; want the new jump and movement", in)
	}

	// What went wrong before: a fresh sender behind a straggler is ignored.
	var fresh InputSender
	var reset InputReceiver
	reset.Receive(straggler)
	reset.Receive(fresh.Next(arena.Input{Jump: true}, 0, 0))
	if in := reset.Input(p); in.Jump {
		t.Error("expected a restarted stream to be shadowed by the straggler (the bug this guards)")
	}
}
