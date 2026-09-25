package online

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"CliffCrack/coordinator"
)

// Session is a player online: connected to the coordinator, perhaps in a
// room, and (once others are in it) linked to them over WebRTC: the host to
// each guest, a guest to the host. It runs in the background; the game
// polls it.
type Session struct {
	Name   string
	Server Server
	cfg    RTCConfig
	lobby  *Lobby

	mu     sync.Mutex
	room   *coordinator.RoomInfo
	links  map[string]*rtcLink // by peer id: the host's guests, or a guest's host
	err    string
	closed bool
	notes  []string // things that happened, for the UI (a player left, the room closed)
}

// Connect joins the coordinator at server as name.
func Connect(server Server, name string, cfg RTCConfig) (*Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	l, err := DialLobby(ctx, server, name)
	if err != nil {
		return nil, err
	}
	s := &Session{Name: name, Server: server, cfg: cfg, lobby: l, links: map[string]*rtcLink{}}
	go s.run()
	return s, nil
}

// ID is our peer id.
func (s *Session) ID() string { return s.lobby.ID }

func (s *Session) run() {
	for m := range s.lobby.Messages() {
		s.handle(m)
	}
	s.mu.Lock()
	if !s.closed {
		s.err = "lost the connection to the server"
	}
	s.mu.Unlock()
}

func (s *Session) handle(m coordinator.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch m.Type {
	case "room":
		s.room = m.Room
		s.syncLinks()
	case "closed":
		s.room = nil
		s.closeLinks()
		s.notes = append(s.notes, "the host closed the room")
	case "error":
		s.err = m.Error
	case "signal":
		l := s.links[m.From]
		if l == nil && s.room != nil && m.From == s.room.Host && !s.isHost() {
			// The host is calling: answer.
			var err error
			from := m.From
			l, err = GuestLink(s.cfg, func(sig Signal) { s.signal(from, sig) })
			if err != nil {
				s.err = err.Error()
				return
			}
			s.links[m.From] = l
		}
		if l != nil {
			if err := l.HandleJSON(m.Data); err != nil {
				s.err = "connecting: " + err.Error()
			}
		}
	}
}

func (s *Session) isHost() bool { return s.room != nil && s.room.Host == s.lobby.ID }

// syncLinks makes sure the host has a link to each guest and none to
// anyone who's gone. (Called with s.mu held.)
func (s *Session) syncLinks() {
	if s.room == nil {
		s.closeLinks()
		return
	}
	in := map[string]bool{}
	for _, mb := range s.room.Members {
		in[mb.ID] = true
	}
	for id, l := range s.links {
		if !in[id] {
			l.Close()
			delete(s.links, id)
		}
	}
	if !s.isHost() {
		return
	}
	for _, mb := range s.room.Members {
		if mb.ID == s.lobby.ID || s.links[mb.ID] != nil {
			continue
		}
		to := mb.ID
		l, err := HostLink(s.cfg, func(sig Signal) { s.signal(to, sig) })
		if err != nil {
			s.err = err.Error()
			continue
		}
		s.links[to] = l
	}
}

func (s *Session) closeLinks() {
	for id, l := range s.links {
		l.Close()
		delete(s.links, id)
	}
}

// signal sends a WebRTC signal to peer to through the coordinator.
func (s *Session) signal(to string, sig Signal) {
	data, _ := json.Marshal(sig)
	go s.lobby.Send(coordinator.Message{Type: "signal", To: to, Data: data}) // not under s.mu: it blocks on the network
}

// Create makes a room (for up to max players) and hosts it.
func (s *Session) Create(name string, max int) error {
	return s.lobby.Send(coordinator.Message{Type: "create", Room: &coordinator.RoomInfo{Name: name, Max: max}})
}

// Join joins a room.
func (s *Session) Join(id string) error {
	return s.lobby.Send(coordinator.Message{Type: "join", RoomID: id})
}

// Leave leaves the room (closing it, if we host it).
func (s *Session) Leave() error {
	s.mu.Lock()
	s.room = nil
	s.closeLinks()
	s.mu.Unlock()
	return s.lobby.Send(coordinator.Message{Type: "leave"})
}

// Start marks the room started (host only): it drops out of the browser.
func (s *Session) Start() error { return s.lobby.Send(coordinator.Message{Type: "start"}) }

// Room is the room we're in, if any.
func (s *Session) Room() (coordinator.RoomInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.room == nil {
		return coordinator.RoomInfo{}, false
	}
	return *s.room, true
}

// IsHost reports whether we host our room.
func (s *Session) IsHost() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isHost()
}

// Link is the link to peer id, and whether it's up.
func (s *Session) Link(id string) (Link, bool) {
	s.mu.Lock()
	l := s.links[id]
	s.mu.Unlock()
	if l == nil {
		return nil, false
	}
	select {
	case <-l.Ready():
		return l, true
	default:
		return l, false
	}
}

// Err is the latest error (and clears it).
func (s *Session) Err() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.err
	s.err = ""
	return e
}

// Notes are things that happened since last asked.
func (s *Session) Notes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.notes
	s.notes = nil
	return n
}

// Close leaves the room and the coordinator.
func (s *Session) Close() {
	s.mu.Lock()
	s.closed = true
	s.closeLinks()
	s.mu.Unlock()
	s.lobby.Close()
}
