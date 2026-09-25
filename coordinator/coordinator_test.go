package coordinator

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type testClient struct {
	t    *testing.T
	conn *websocket.Conn
	id   string
}

func dial(t *testing.T, srv *httptest.Server, name string) *testClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &testClient{t: t, conn: conn}
	c.send(Message{Type: "hello", Name: name})
	w := c.expect("welcome")
	c.id = w.ID
	if len(w.ICE) == 0 || !strings.HasPrefix(w.ICE[0].URLs[0], "stun:") {
		t.Errorf("welcome ICE %+v: want STUN servers", w.ICE)
	}
	return c
}

func (c *testClient) send(m Message) {
	c.t.Helper()
	data, _ := json.Marshal(m)
	if err := c.conn.Write(context.Background(), websocket.MessageText, data); err != nil {
		c.t.Fatal(err)
	}
}

// expect reads messages until one of the given type (failing on an error message).
func (c *testClient) expect(typ string) Message {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			c.t.Fatalf("waiting for %s: %v", typ, err)
		}
		var m Message
		json.Unmarshal(data, &m)
		if m.Type == typ {
			return m
		}
		if m.Type == "error" && typ != "error" {
			c.t.Fatalf("waiting for %s: error %q", typ, m.Error)
		}
	}
}

func newServer(t *testing.T) *httptest.Server {
	s := New()
	s.Log = log.New(io.Discard, "", 0)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func rooms(t *testing.T, srv *httptest.Server) []RoomInfo {
	t.Helper()
	resp, err := http.Get(srv.URL + "/rooms")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []RoomInfo
	json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func TestCreateListJoinSignalAndClose(t *testing.T) {
	srv := newServer(t)
	host := dial(t, srv, "Ben")
	host.send(Message{Type: "create", Room: &RoomInfo{Name: "Chasm", Max: 2}})
	room := host.expect("room").Room
	if room.Host != host.id || room.Name != "Chasm" || len(room.Members) != 1 {
		t.Fatalf("created %+v", room)
	}
	list := rooms(t, srv)
	if len(list) != 1 || list[0].ID != room.ID || list[0].Members[0].Name != "Ben" {
		t.Fatalf("listed %+v", list)
	}

	guest := dial(t, srv, "  Sam\n ")
	guest.send(Message{Type: "join", RoomID: strings.ToLower(room.ID)})
	if r := guest.expect("room").Room; len(r.Members) != 2 || r.Members[1].Name != "Sam" {
		t.Fatalf("joined %+v", r)
	}
	if r := host.expect("room").Room; len(r.Members) != 2 {
		t.Fatalf("the host should hear of the guest: %+v", r)
	}

	// A third can't join a full room.
	late := dial(t, srv, "Late")
	late.send(Message{Type: "join", RoomID: room.ID})
	if e := late.expect("error"); !strings.Contains(e.Error, "full") {
		t.Errorf("joining a full room: %q", e.Error)
	}

	// Signals pass between the host and guest, and only within the room.
	host.send(Message{Type: "signal", To: guest.id, Data: json.RawMessage(`{"sdp":"offer"}`)})
	if m := guest.expect("signal"); m.From != host.id || string(m.Data) != `{"sdp":"offer"}` {
		t.Errorf("guest got signal %+v", m)
	}
	late.send(Message{Type: "signal", To: host.id, Data: json.RawMessage(`{}`)})
	late.expect("error")

	// Starting marks it started; the host leaving closes it.
	host.send(Message{Type: "start"})
	if r := guest.expect("room").Room; !r.Started {
		t.Error("the room should be marked started")
	}
	host.conn.Close(websocket.StatusNormalClosure, "")
	guest.expect("closed")
	deadline := time.Now().Add(2 * time.Second)
	for len(rooms(t, srv)) != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := len(rooms(t, srv)); n != 0 {
		t.Errorf("%d rooms after the host left", n)
	}
}

func TestGuestLeavingUpdatesTheRoom(t *testing.T) {
	srv := newServer(t)
	host := dial(t, srv, "Host")
	host.send(Message{Type: "create"})
	room := host.expect("room").Room
	guest := dial(t, srv, "Guest")
	guest.send(Message{Type: "join", RoomID: room.ID})
	host.expect("room")
	guest.send(Message{Type: "leave"})
	if r := host.expect("room").Room; len(r.Members) != 1 {
		t.Errorf("after the guest left: %+v", r)
	}
}

func TestTURNCredentials(t *testing.T) {
	s := New()
	s.Log = log.New(io.Discard, "", 0)
	s.TURN = SharedSecretTURN([]string{"turn:turn.example:3478"}, "sekrit", time.Hour)
	ice := s.iceServers(context.Background())
	if len(ice) != 2 || ice[1].URLs[0] != "turn:turn.example:3478" || ice[1].Username == "" || ice[1].Credential == "" {
		t.Fatalf("ice %+v: want STUN then TURN with credentials", ice)
	}
	// A TURN provider that fails still leaves STUN.
	s.TURN = func(context.Context) ([]ICEServer, error) { return nil, io.EOF }
	if ice := s.iceServers(context.Background()); len(ice) != 1 {
		t.Errorf("with TURN failing: %+v, want STUN alone", ice)
	}
}
