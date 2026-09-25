package online

import (
	"encoding/json"
	"testing"
	"time"
)

// TestRTCLinkConnectsAndCarriesBothChannels sets up a real WebRTC link
// between a host and a guest on this machine, passing their signals
// through JSON as the coordinator would.
func TestRTCLinkConnectsAndCarriesBothChannels(t *testing.T) {
	cfg := RTCConfig{Loopback: true}
	toGuest, toHost := make(chan []byte, 64), make(chan []byte, 64)
	send := func(ch chan []byte) func(Signal) {
		return func(s Signal) {
			data, _ := json.Marshal(s)
			ch <- data
		}
	}
	host, err := HostLink(cfg, send(toGuest))
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	guest, err := GuestLink(cfg, send(toHost))
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	done := make(chan struct{})
	defer close(done)
	relay := func(from chan []byte, to *rtcLink) {
		for {
			select {
			case d := <-from:
				if err := to.HandleJSON(d); err != nil {
					t.Errorf("signal: %v", err)
				}
			case <-done:
				return
			}
		}
	}
	go relay(toGuest, guest)
	go relay(toHost, host)

	if err := host.Wait(10 * time.Second); err != nil {
		t.Fatal(err)
	}
	if err := guest.Wait(10 * time.Second); err != nil {
		t.Fatal(err)
	}
	for _, c := range []Channel{Fast, Reliable} {
		if err := host.Send(c, []byte("hello")); err != nil {
			t.Fatal(err)
		}
		select {
		case p := <-guest.Recv():
			if string(p.Data) != "hello" || p.Channel != c {
				t.Errorf("got %q on %v, want hello on %v", p.Data, p.Channel, c)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("nothing arrived on channel %v", c)
		}
	}
	if err := guest.Send(Reliable, []byte("back")); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-host.Recv():
		if string(p.Data) != "back" {
			t.Errorf("host got %q", p.Data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the guest's message didn't reach the host")
	}
}
