package online

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"CliffCrack/coordinator"

	"github.com/coder/websocket"
)

// Server is a coordinator's address as the player typed it: "host:port"
// on a local network (plain ws/http), a bare name on the internet (TLS), or
// a full ws://, wss://, http:// or https:// URL.
type Server string

func (s Server) base() (ws, http string) {
	a := strings.TrimRight(strings.TrimSpace(string(s)), "/")
	switch {
	case strings.HasPrefix(a, "ws://"), strings.HasPrefix(a, "http://"):
		h := a[strings.Index(a, "//")+2:]
		return "ws://" + h, "http://" + h
	case strings.HasPrefix(a, "wss://"), strings.HasPrefix(a, "https://"):
		h := a[strings.Index(a, "//")+2:]
		return "wss://" + h, "https://" + h
	}
	host := a
	if h, _, err := net.SplitHostPort(a); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); host == "localhost" || (ip != nil && (ip.IsPrivate() || ip.IsLoopback())) {
		return "ws://" + a, "http://" + a
	}
	return "wss://" + a, "https://" + a
}

// ListRooms fetches the coordinator's rooms.
func ListRooms(ctx context.Context, s Server) ([]coordinator.RoomInfo, error) {
	_, base := s.base()
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/rooms", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the coordinator said %s", resp.Status)
	}
	var rooms []coordinator.RoomInfo
	return rooms, json.NewDecoder(resp.Body).Decode(&rooms)
}

// Lobby is a connection to the coordinator.
type Lobby struct {
	ID   string                  // our peer id
	ICE  []coordinator.ICEServer // the STUN/TURN servers the coordinator gave us
	conn *websocket.Conn
	in   chan coordinator.Message
	mu   sync.Mutex
	done chan struct{}
}

// DialLobby connects to the coordinator as name.
func DialLobby(ctx context.Context, s Server, name string) (*Lobby, error) {
	wsURL, _ := s.base()
	if _, err := url.Parse(wsURL); err != nil {
		return nil, err
	}
	conn, _, err := websocket.Dial(ctx, wsURL+"/ws", nil)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(1 << 20)
	l := &Lobby{conn: conn, in: make(chan coordinator.Message, 256), done: make(chan struct{})}
	if err := l.Send(coordinator.Message{Type: "hello", Name: name}); err != nil {
		conn.CloseNow()
		return nil, err
	}
	go l.read()
	select {
	case m := <-l.in:
		if m.Type != "welcome" {
			l.Close()
			return nil, fmt.Errorf("unexpected %s from the coordinator", m.Type)
		}
		l.ID, l.ICE = m.ID, m.ICE
	case <-ctx.Done():
		l.Close()
		return nil, ctx.Err()
	}
	return l, nil
}

func (l *Lobby) read() {
	defer close(l.in)
	for {
		_, data, err := l.conn.Read(context.Background())
		if err != nil {
			return
		}
		var m coordinator.Message
		if json.Unmarshal(data, &m) == nil {
			select {
			case l.in <- m:
			case <-l.done:
				return
			}
		}
	}
}

// Messages are the coordinator's messages to us; closed when disconnected.
func (l *Lobby) Messages() <-chan coordinator.Message { return l.in }

// Send sends a message to the coordinator.
func (l *Lobby) Send(m coordinator.Message) error {
	data, _ := json.Marshal(m)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.conn.Write(ctx, websocket.MessageText, data)
}

// Close disconnects.
func (l *Lobby) Close() {
	select {
	case <-l.done:
	default:
		close(l.done)
	}
	l.conn.Close(websocket.StatusNormalClosure, "")
}
