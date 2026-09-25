package arena

import (
	"encoding/json"
	"testing"
)

// wire sends v through JSON, as it would travel.
func wire[T any](t *testing.T, v T) T {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAClientMirrorsTheHost plays bots through a match on a host and keeps
// a client's copy up to date only from what the host would send, then
// checks the client's site and players match the host's exactly:
// destruction included.
func TestAClientMirrorsTheHost(t *testing.T) {
	const seed = 11
	host := NewMatch(seed, 2)
	client := NewMatch(seed, 2)
	bots := []*Bot{NewBot(1), NewBot(2)}
	inputs := make([]Input, 2)
	broke, collapsed := 0, 0
	for step := 0; step < 60*90 && host.Phase != PhaseMatchOver; step++ {
		a := host.Arena
		for i, p := range a.Players {
			inputs[i] = bots[i].Think(a, p, frame)
		}
		ev := host.Step(frame, inputs)
		for i, b := range bots {
			if host.Arena == a {
				b.Hear(a, a.Players[i], &ev)
			} else {
				b.Reset()
			}
		}
		// Every couple of seconds, a big blast somewhere on the site (and
		// halfway through, one that takes out a player): plenty of breaks
		// and collapses, and a round change.
		if step%120 == 60 && host.Phase == PhaseFight {
			h := host.Arena
			c := h.chunks[(step*7919)%len(h.chunks)]
			if c.Alive {
				h.blast(c.Centre, 3.5, 3000, 0, 6, V3{}, h.Players[0], WeaponLauncher, &ev)
				h.settle(&ev)
			}
		}
		if step == 60*40 {
			h := host.Arena
			h.hurtPlayer(h.Players[1], h.Players[0], 1e6, false, WeaponLauncher, h.Players[1].Body.Position, V3{}, &ev)
		}
		broke += len(ev.Breaks)
		for _, b := range ev.Breaks {
			if b.Collapsed {
				collapsed++
			}
		}

		// What the host sends: the step's events, the match standing, and
		// (every other step) a snapshot.
		net := wire(t, ev.Net())
		client.Arena.ApplyEvents(&net)
		nm := wire(t, host.Net())
		client.Apply(&nm)
		if step%2 == 0 || host.Arena != a {
			snap := wire(t, host.Arena.Snapshot())
			client.Arena.ApplySnapshot(&snap, -1)
		}
		client.Arena.StepCosmetic(frame)

		if client.Round != host.Round {
			t.Fatalf("step %d: client on round %d, host on %d", step, client.Round, host.Round)
		}
	}
	if broke < 100 || collapsed == 0 || host.Round < 2 {
		t.Fatalf("%d breaks, %d collapsed, round %d: want plenty of destruction and a new round to prove it's shared", broke, collapsed, host.Round)
	}
	h, c := host.Arena, client.Arena
	if len(h.chunks) != len(c.chunks) {
		t.Fatalf("host has %d chunks, client %d", len(h.chunks), len(c.chunks))
	}
	diff := 0
	for i := range h.chunks {
		if h.chunks[i].Alive != c.chunks[i].Alive || (h.chunks[i].Alive && h.chunks[i].HP != c.chunks[i].HP) {
			diff++
		}
	}
	if diff != 0 {
		t.Errorf("%d chunks differ between host and client (of %d; %d breaks this match)", diff, len(h.chunks), broke)
	}
	if h.Standing() != c.Standing() {
		t.Errorf("standing: host %.3f, client %.3f", h.Standing(), c.Standing())
	}
	for i := range h.Players {
		hp, cp := h.Players[i], c.Players[i]
		if hp.Dead != cp.Dead || hp.Shield != cp.Shield || hp.Health != cp.Health || hp.Current != cp.Current ||
			hp.Grenades != cp.Grenades || hp.Stats != cp.Stats {
			t.Errorf("player %d: host %+v / client %+v", i, hp.Stats, cp.Stats)
		}
	}
	if len(h.Pickups) != len(c.Pickups) {
		t.Errorf("pickups: host %d, client %d", len(h.Pickups), len(c.Pickups))
	}
	if host.Wins[0] != client.Wins[0] || host.Wins[1] != client.Wins[1] || host.Phase != client.Phase {
		t.Errorf("standing: host %v %v, client %v %v", host.Wins, host.Phase, client.Wins, client.Phase)
	}
	t.Logf("round %d, %d breaks (%d collapses) mirrored", host.Round, broke, collapsed)
}
