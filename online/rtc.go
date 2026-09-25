package online

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"CliffCrack/coordinator"

	"github.com/pion/webrtc/v4"
)

// Signal is one step of setting up a WebRTC link, passed between the two
// sides through the coordinator.
type Signal struct {
	SDP       *webrtc.SessionDescription `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit   `json:"candidate,omitempty"`
}

// RTCConfig is how links find each other. On a local network nothing is
// needed (the players' own addresses work); across the internet, STUN finds
// each player's public address, and TURN relays for players behind strict
// NATs (phone networks, some routers). The coordinator hands these out when
// you connect (see Connect).
type RTCConfig struct {
	ICEServers []coordinator.ICEServer
	// Loopback lets links use 127.0.0.1 (two games on one machine, tests).
	Loopback bool
}

func (c RTCConfig) api() (*webrtc.API, webrtc.Configuration) {
	var se webrtc.SettingEngine
	se.SetIncludeLoopbackCandidate(c.Loopback)
	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	var cfg webrtc.Configuration
	for _, s := range c.ICEServers {
		cfg.ICEServers = append(cfg.ICEServers, webrtc.ICEServer{URLs: s.URLs, Username: s.Username, Credential: s.Credential})
	}
	return api, cfg
}

// rtcLink is a Link over a WebRTC peer connection.
type rtcLink struct {
	pc         *webrtc.PeerConnection
	fast, slow *webrtc.DataChannel
	in         chan Packet
	ready      chan struct{} // closed when both channels are open
	readyOnce  sync.Once
	mu         sync.Mutex
	closed     bool
	opened     int

	signal  func(Signal) // sends a signal to the other side
	pending []webrtc.ICECandidateInit
	haveSDP bool
}

func newRTCLink(cfg RTCConfig, signal func(Signal)) (*rtcLink, error) {
	api, c := cfg.api()
	pc, err := api.NewPeerConnection(c)
	if err != nil {
		return nil, err
	}
	l := &rtcLink{pc: pc, in: make(chan Packet, 1024), ready: make(chan struct{}), signal: signal}
	pc.OnICECandidate(func(cand *webrtc.ICECandidate) {
		if cand != nil {
			init := cand.ToJSON()
			signal(Signal{Candidate: &init})
		}
	})
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed ||
			s == webrtc.PeerConnectionStateDisconnected {
			l.Close()
		}
	})
	return l, nil
}

func (l *rtcLink) attach(dc *webrtc.DataChannel) {
	ch := Reliable
	if dc.Label() == "fast" {
		ch = Fast
		l.fast = dc
	} else {
		l.slow = dc
	}
	dc.OnOpen(func() {
		l.mu.Lock()
		l.opened++
		both := l.opened == 2
		l.mu.Unlock()
		if both {
			l.readyOnce.Do(func() { close(l.ready) })
		}
	})
	dc.OnMessage(func(m webrtc.DataChannelMessage) {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.closed {
			return
		}
		select {
		case l.in <- Packet{Channel: ch, Data: m.Data}:
		default: // the game's fallen behind; fast packets can go, and reliable ones mean trouble
		}
	})
}

// HostLink starts a link to a guest, offering the connection. Feed the
// guest's signals to Handle.
func HostLink(cfg RTCConfig, signal func(Signal)) (*rtcLink, error) {
	l, err := newRTCLink(cfg, signal)
	if err != nil {
		return nil, err
	}
	unordered, noRetransmits := false, uint16(0)
	fast, err := l.pc.CreateDataChannel("fast", &webrtc.DataChannelInit{Ordered: &unordered, MaxRetransmits: &noRetransmits})
	if err != nil {
		return nil, err
	}
	slow, err := l.pc.CreateDataChannel("reliable", nil)
	if err != nil {
		return nil, err
	}
	l.attach(fast)
	l.attach(slow)
	offer, err := l.pc.CreateOffer(nil)
	if err != nil {
		return nil, err
	}
	if err := l.pc.SetLocalDescription(offer); err != nil {
		return nil, err
	}
	signal(Signal{SDP: &offer})
	return l, nil
}

// GuestLink starts the guest's side of a link, waiting for the host's offer
// (through Handle).
func GuestLink(cfg RTCConfig, signal func(Signal)) (*rtcLink, error) {
	l, err := newRTCLink(cfg, signal)
	if err != nil {
		return nil, err
	}
	l.pc.OnDataChannel(l.attach)
	return l, nil
}

// Handle applies a signal from the other side.
func (l *rtcLink) Handle(s Signal) error {
	switch {
	case s.SDP != nil:
		if err := l.pc.SetRemoteDescription(*s.SDP); err != nil {
			return err
		}
		if s.SDP.Type == webrtc.SDPTypeOffer {
			answer, err := l.pc.CreateAnswer(nil)
			if err != nil {
				return err
			}
			if err := l.pc.SetLocalDescription(answer); err != nil {
				return err
			}
			l.signal(Signal{SDP: &answer})
		}
		l.mu.Lock()
		l.haveSDP = true
		pending := l.pending
		l.pending = nil
		l.mu.Unlock()
		for _, c := range pending {
			if err := l.pc.AddICECandidate(c); err != nil {
				return err
			}
		}
	case s.Candidate != nil:
		l.mu.Lock()
		if !l.haveSDP { // candidates can overtake the description
			l.pending = append(l.pending, *s.Candidate)
			l.mu.Unlock()
			return nil
		}
		l.mu.Unlock()
		return l.pc.AddICECandidate(*s.Candidate)
	}
	return nil
}

// HandleJSON applies a signal as it came from the coordinator.
func (l *rtcLink) HandleJSON(data []byte) error {
	var s Signal
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return l.Handle(s)
}

// Wait blocks until the link is up, or the timeout.
func (l *rtcLink) Wait(timeout time.Duration) error {
	select {
	case <-l.ready:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("online: couldn't connect within %v", timeout)
	}
}

// Ready is closed once the link is up.
func (l *rtcLink) Ready() <-chan struct{} { return l.ready }

func (l *rtcLink) Send(ch Channel, data []byte) error {
	l.mu.Lock()
	closed := l.closed
	l.mu.Unlock()
	if closed {
		return ErrClosed
	}
	dc := l.slow
	if ch == Fast {
		dc = l.fast
	}
	if dc == nil || dc.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("online: channel not open")
	}
	return dc.Send(data)
}

func (l *rtcLink) Recv() <-chan Packet { return l.in }

func (l *rtcLink) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	close(l.in)
	l.mu.Unlock()
	return l.pc.Close()
}
