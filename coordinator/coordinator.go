// Package coordinator is the online lobby: a small HTTP and WebSocket
// server that lists rooms, lets players create and join them, and relays
// the WebRTC signalling (offers, answers and ICE candidates) between a
// room's host and its guests. Game traffic never passes through it: once
// connected, players talk directly over WebRTC data channels, the host
// running the match.
//
// It is built to run on fly.io (see cmd/coordinator, the Dockerfile and
// fly.toml) but is as happy on a laptop on the local network.
//
// The protocol:
//
//	GET /healthz   200 "ok"
//	GET /rooms     the open rooms, as JSON ([]RoomInfo)
//	GET /ws        a WebSocket of JSON Messages:
//	  client: hello {name}          server: welcome {id, ice}
//	  client: create {room: {name, max}}
//	  client: join {roomId}         server: room {room} (to everyone in it, whenever it changes)
//	  client: leave                 server: closed (the host left) / error {error}
//	  client: start (host)          server: room {room, started}
//	  client: signal {to, data}     server: signal {from, data}
package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Message is one JSON message on the WebSocket, either way.
type Message struct {
	Type   string          `json:"type"`
	Name   string          `json:"name,omitempty"`   // hello
	ID     string          `json:"id,omitempty"`     // welcome: your peer id
	Room   *RoomInfo       `json:"room,omitempty"`   // create (name, max) / room
	RoomID string          `json:"roomId,omitempty"` // join
	To     string          `json:"to,omitempty"`     // signal
	From   string          `json:"from,omitempty"`   // signal
	Data   json.RawMessage `json:"data,omitempty"`   // signal: opaque to the coordinator
	Error  string          `json:"error,omitempty"`
	ICE    []ICEServer     `json:"ice,omitempty"` // welcome: the STUN/TURN servers to connect with
}

// Member is a player in a room.
type Member struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RoomInfo is a room as the lobby browser and its members see it.
type RoomInfo struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Host    string   `json:"host"` // the host's peer id
	Members []Member `json:"members"`
	Max     int      `json:"max"`
	Started bool     `json:"started"`
	Created int64    `json:"created"` // unix seconds
}

// Limits.
const (
	MaxPlayers  = 4
	maxNameLen  = 24
	maxRooms    = 200
	maxSignal   = 64 << 10 // bytes in one signal message
	writeWait   = 5 * time.Second
	pingEvery   = 20 * time.Second
	sendBacklog = 64
)

type client struct {
	id   string
	name string
	room *room
	send chan Message
}

type room struct {
	info    RoomInfo
	host    *client
	members []*client
}

// Server is the coordinator. Its zero value isn't usable; use New.
type Server struct {
	mu      sync.Mutex
	rooms   map[string]*room
	clients map[string]*client
	rng     *rand.Rand
	nextID  int
	Log     *log.Logger // nil: log.Default()
	// TURN gives relay servers for players who can't connect directly
	// (see ICEFromEnv); nil for STUN only.
	TURN ICEProvider
}

// New makes a coordinator.
func New() *Server {
	return &Server{rooms: map[string]*room{}, clients: map[string]*client{},
		rng: rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 7))}
}

func (s *Server) logf(format string, args ...any) {
	l := s.Log
	if l == nil {
		l = log.Default()
	}
	l.Printf(format, args...)
}

// Handler serves the coordinator's endpoints.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "ok") })
	mux.HandleFunc("GET /rooms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.Rooms())
	})
	mux.HandleFunc("GET /ws", s.serveWS)
	return mux
}

// Rooms lists the rooms, newest first: open ones and ones already playing
// (the browser shows those as in progress).
func (s *Server) Rooms() []RoomInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RoomInfo, 0, len(s.rooms))
	for _, r := range s.rooms {
		out = append(out, r.snapshot())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Created > out[j].Created || (out[i].Created == out[j].Created && out[i].ID < out[j].ID)
	})
	return out
}

func (r *room) snapshot() RoomInfo {
	info := r.info
	info.Members = make([]Member, len(r.members))
	for i, m := range r.members {
		info.Members[i] = Member{ID: m.id, Name: m.name}
	}
	return info
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true}) // any origin: the clients are games, not pages
	if err != nil {
		return
	}
	conn.SetReadLimit(maxSignal * 2)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	s.mu.Lock()
	s.nextID++
	c := &client{id: fmt.Sprintf("p%d", s.nextID), name: "Player", send: make(chan Message, sendBacklog)}
	s.clients[c.id] = c
	s.mu.Unlock()
	defer s.disconnect(c)

	go s.writeLoop(ctx, cancel, conn, c)
	for {
		var m Message
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if err := json.Unmarshal(data, &m); err != nil {
			s.deliver(c, Message{Type: "error", Error: "bad message"})
			continue
		}
		if m.Type == "hello" { // (the TURN lookup may take a moment: not under the lock)
			m.ICE = s.iceServers(ctx)
		}
		s.handle(c, m)
	}
}

func (s *Server) writeLoop(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, c *client) {
	defer cancel()
	ping := time.NewTicker(pingEvery)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "")
			return
		case <-ping.C:
			pctx, done := context.WithTimeout(ctx, writeWait)
			err := conn.Ping(pctx)
			done()
			if err != nil {
				return
			}
		case m := <-c.send:
			data, _ := json.Marshal(m)
			wctx, done := context.WithTimeout(ctx, writeWait)
			err := conn.Write(wctx, websocket.MessageText, data)
			done()
			if err != nil {
				return
			}
		}
	}
}

// deliver queues a message for c, dropping it if c has fallen far behind.
func (s *Server) deliver(c *client, m Message) {
	select {
	case c.send <- m:
	default:
		s.logf("coordinator: %s is backed up, dropping a %s", c.id, m.Type)
	}
}

func (s *Server) handle(c *client, m Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch m.Type {
	case "hello":
		c.name = cleanName(m.Name)
		s.deliver(c, Message{Type: "welcome", ID: c.id, ICE: m.ICE})
	case "create":
		if err := s.create(c, m.Room); err != nil {
			s.deliver(c, Message{Type: "error", Error: err.Error()})
		}
	case "join":
		if err := s.join(c, m.RoomID); err != nil {
			s.deliver(c, Message{Type: "error", Error: err.Error()})
		}
	case "leave":
		s.leave(c)
	case "start":
		if r := c.room; r != nil && r.host == c {
			r.info.Started = true
			s.broadcast(r)
		}
	case "signal":
		to, ok := s.clients[m.To]
		switch {
		case !ok || c.room == nil || to.room != c.room:
			s.deliver(c, Message{Type: "error", Error: "no such peer in your room"})
		case len(m.Data) > maxSignal:
			s.deliver(c, Message{Type: "error", Error: "signal too big"})
		default:
			s.deliver(to, Message{Type: "signal", From: c.id, Data: m.Data})
		}
	default:
		s.deliver(c, Message{Type: "error", Error: "unknown message " + m.Type})
	}
}

func cleanName(n string) string {
	n = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, n))
	if n == "" {
		return "Player"
	}
	if r := []rune(n); len(r) > maxNameLen {
		n = string(r[:maxNameLen])
	}
	return n
}

func (s *Server) create(c *client, want *RoomInfo) error {
	if len(s.rooms) >= maxRooms {
		return errors.New("the server is full of rooms; try again soon")
	}
	s.leave(c)
	name, max := c.name+"'s room", 2
	if want != nil {
		if want.Name != "" {
			name = cleanName(want.Name)
		}
		if want.Max >= 2 && want.Max <= MaxPlayers {
			max = want.Max
		}
	}
	r := &room{host: c, members: []*client{c}, info: RoomInfo{ID: s.roomID(), Name: name, Host: c.id, Max: max,
		Created: time.Now().Unix()}}
	s.rooms[r.info.ID] = r
	c.room = r
	s.logf("coordinator: %s (%s) made room %s %q", c.id, c.name, r.info.ID, name)
	s.broadcast(r)
	return nil
}

// roomID is a short, unused code: four letters, no lookalikes.
func (s *Server) roomID() string {
	const letters = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	for {
		b := make([]byte, 4)
		for i := range b {
			b[i] = letters[s.rng.IntN(len(letters))]
		}
		if _, taken := s.rooms[string(b)]; !taken {
			return string(b)
		}
	}
}

func (s *Server) join(c *client, id string) error {
	r, ok := s.rooms[strings.ToUpper(strings.TrimSpace(id))]
	switch {
	case !ok:
		return errors.New("no room " + id)
	case r == c.room:
		return nil
	case r.info.Started:
		return errors.New("that match has started")
	case len(r.members) >= r.info.Max:
		return errors.New("that room is full")
	}
	s.leave(c)
	r.members = append(r.members, c)
	c.room = r
	s.broadcast(r)
	return nil
}

// leave takes c out of its room; the room closes if c hosted it.
func (s *Server) leave(c *client) {
	r := c.room
	if r == nil {
		return
	}
	c.room = nil
	if r.host == c {
		delete(s.rooms, r.info.ID)
		for _, m := range r.members {
			if m != c {
				m.room = nil
				s.deliver(m, Message{Type: "closed", RoomID: r.info.ID})
			}
		}
		s.logf("coordinator: room %s closed", r.info.ID)
		return
	}
	for i, m := range r.members {
		if m == c {
			r.members = append(r.members[:i], r.members[i+1:]...)
			break
		}
	}
	s.broadcast(r)
}

// broadcast tells everyone in r how it stands.
func (s *Server) broadcast(r *room) {
	info := r.snapshot()
	for _, m := range r.members {
		s.deliver(m, Message{Type: "room", Room: &info})
	}
}

func (s *Server) disconnect(c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leave(c)
	delete(s.clients, c.id)
}
