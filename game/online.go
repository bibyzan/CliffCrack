package game

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"CliffCrack/coordinator"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/input"
	"CliffCrack/engine/ui"
	"CliffCrack/online"
)

// The online screens: connecting to the coordinator, the lobby browser (the
// rooms on it, and making one), and a room (who's in it, and for the host,
// starting the match). Once the match starts, the Arena mode plays it (see
// arenanet.go).

type onlineState int

const (
	onlineConnecting onlineState = iota
	onlineBrowser
	onlineRoom
	onlineFailed
)

const (
	roomsRefresh = 2 * time.Second
	linkTimeout  = 15 * time.Second
)

// onlineScreen is the lobby browser and room screen.
type onlineScreen struct {
	state    onlineState
	server   online.Server
	name     string
	session  *online.Session
	rooms    []coordinator.RoomInfo
	fetched  time.Time
	fetch    chan roomsResult
	dial     chan dialResult
	status   string // a line of news or trouble
	choice   int
	clicked  int // the button clicked this frame, -1 none
	start    *online.StartMsg
	joinedAt time.Time

	autoHost, autoJoin bool // make a room and start it / join the first room, without being asked
	autoDone           bool
}

type roomsResult struct {
	rooms []coordinator.RoomInfo
	err   error
}

type dialResult struct {
	s   *online.Session
	err error
}

// playerName is the name to go by online: the setting, else the account.
func (s *Settings) playerName() string {
	if n := strings.TrimSpace(s.Name); n != "" {
		return n
	}
	for _, k := range []string{"USERNAME", "USER"} {
		if n := os.Getenv(k); n != "" {
			return n
		}
	}
	return "Player"
}

// open connects to the coordinator (in the background).
func (o *onlineScreen) open(server, name string) {
	auto := [2]bool{o.autoHost, o.autoJoin}
	*o = onlineScreen{state: onlineConnecting, server: online.Server(server), name: name, clicked: -1,
		dial: make(chan dialResult, 1)}
	o.autoHost, o.autoJoin = auto[0], auto[1] // (kept across a retry)
	go func() {
		s, err := online.Connect(o.server, name, online.RTCConfig{Loopback: true})
		o.dial <- dialResult{s, err}
	}()
}

// close leaves the coordinator.
func (o *onlineScreen) close() {
	if o.session != nil {
		o.session.Close()
		o.session = nil
	}
}

// update runs the screens. It reports the match to start (and the session
// to play it over) once there is one, or back when the player leaves.
func (o *onlineScreen) update(in *input.State) (start *online.StartMsg, back bool) {
	clicked := o.clicked
	o.clicked = -1
	if pausePressed(in) || in.PadPressed(input.PadB) {
		if o.state == onlineRoom {
			o.session.Leave()
			o.state, o.status = onlineBrowser, ""
			return nil, false
		}
		return nil, true
	}
	switch o.state {
	case onlineConnecting:
		select {
		case r := <-o.dial:
			if r.err != nil {
				o.state, o.status = onlineFailed, fmt.Sprintf("Couldn't reach the server at %s: %v", o.server, r.err)
				return nil, false
			}
			o.session, o.state = r.s, onlineBrowser
		default:
		}
		return nil, false
	case onlineFailed:
		if clicked == 0 || confirmPressed(in) {
			o.open(string(o.server), o.name)
		}
		if clicked == 1 {
			return nil, true
		}
		return nil, false
	}

	if e := o.session.Err(); e != "" {
		o.status = e
	}
	for _, n := range o.session.Notes() {
		o.status = n
	}
	room, inRoom := o.session.Room()
	switch {
	case inRoom && o.state != onlineRoom:
		o.state, o.choice, o.joinedAt = onlineRoom, 0, time.Now()
	case !inRoom && o.state == onlineRoom:
		o.state, o.choice = onlineBrowser, 0
	}

	if o.state == onlineBrowser {
		o.refreshRooms()
		if o.autoHost && !o.autoDone {
			o.autoDone = true
			o.session.Create(o.name+"'s room", coordinator.MaxPlayers)
		}
		if rooms := o.openRooms(); o.autoJoin && !o.autoDone && len(rooms) > 0 {
			o.autoDone = true
			o.session.Join(rooms[0].ID)
		}
		n := len(o.openRooms()) + 3 // the rooms, then Create, Refresh, Back
		o.choice = navigate(o.choice, n, in)
		pick := clicked
		if confirmPressed(in) {
			pick = o.choice
		}
		rooms := o.openRooms()
		switch {
		case pick < 0:
		case pick < len(rooms):
			o.session.Join(rooms[pick].ID)
			o.status = "Joining " + rooms[pick].Name + "..."
		case pick == len(rooms):
			o.session.Create(o.name+"'s room", coordinator.MaxPlayers)
		case pick == len(rooms)+1:
			o.fetched = time.Time{}
		case pick == len(rooms)+2:
			return nil, true
		}
		return nil, false
	}

	// In a room.
	host := o.session.IsHost()
	o.choice = navigate(o.choice, 2, in) // Start (host) / Leave
	pick := clicked
	if confirmPressed(in) {
		pick = o.choice
	}
	if pick == 1 {
		o.session.Leave()
		return nil, false
	}
	if host {
		ready := o.everyoneLinked(room) && len(room.Members) >= 2
		if (pick == 0 || o.autoHost) && ready {
			return o.startMatch(room), false
		}
		return nil, false
	}
	// A guest waits for the host's start, on the link to the host.
	if l, up := o.session.Link(room.Host); up {
		for {
			select {
			case p, ok := <-l.Recv():
				if !ok {
					o.status = "lost the host"
					return nil, false
				}
				if m, err := online.Decode(p.Data); err == nil && m.Start != nil {
					return m.Start, false
				}
				continue
			default:
			}
			break
		}
	} else if time.Since(o.joinedAt) > linkTimeout {
		o.status = "Can't reach the host directly (a firewall?). Still trying..."
	}
	return nil, false
}

// startMatch (host) tells each guest the match is on and which player
// they are, and marks the room started.
func (o *onlineScreen) startMatch(room coordinator.RoomInfo) *online.StartMsg {
	seed := uint64(time.Now().UnixNano())
	names := make([]string, len(room.Members))
	for i, mb := range room.Members {
		names[i] = mb.Name
	}
	for i, mb := range room.Members {
		if mb.ID == o.session.ID() {
			continue
		}
		if l, up := o.session.Link(mb.ID); up {
			l.Send(online.Reliable, online.Encode(online.Msg{Start: &online.StartMsg{Seed: seed, Names: names, You: i}}))
		}
	}
	o.session.Start()
	return &online.StartMsg{Seed: seed, Names: names, You: 0}
}

func (o *onlineScreen) everyoneLinked(room coordinator.RoomInfo) bool {
	for _, mb := range room.Members {
		if mb.ID == o.session.ID() {
			continue
		}
		if _, up := o.session.Link(mb.ID); !up {
			return false
		}
	}
	return true
}

// openRooms are the rooms that can be joined.
func (o *onlineScreen) openRooms() []coordinator.RoomInfo {
	var out []coordinator.RoomInfo
	for _, r := range o.rooms {
		if !r.Started && len(r.Members) < r.Max {
			out = append(out, r)
		}
	}
	return out
}

// refreshRooms fetches the room list every so often (in the background).
func (o *onlineScreen) refreshRooms() {
	if o.fetch != nil {
		select {
		case r := <-o.fetch:
			o.fetch = nil
			if r.err != nil {
				o.status = "Couldn't list the rooms: " + r.err.Error()
			} else {
				o.rooms = r.rooms
			}
		default:
		}
		return
	}
	if time.Since(o.fetched) < roomsRefresh {
		return
	}
	o.fetched = time.Now()
	o.fetch = make(chan roomsResult, 1)
	ch := o.fetch
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rooms, err := online.ListRooms(ctx, o.server)
		ch <- roomsResult{rooms, err}
	}()
}

// navigate moves a selection through n items with up/down.
func navigate(choice, n int, in *input.State) int {
	if n <= 0 {
		return 0
	}
	return ((choice+navY(in))%n + n) % n
}

func (o *onlineScreen) ui(b *ui.Builder, in *input.State) {
	b.Panel("##onlinetitle", 0.5, 0.12, hudText, 3.5)
	b.Text("ONLINE")
	b.End()
	b.Panel("##onlinesub", 0.5, 0.2, hudText, 1.1)
	b.ColorText(uiMuted, "server %s   ·   playing as %s", o.server, o.name)
	b.End()

	const width = 520
	b.Panel("##online", 0.5, 0.56, card, 1.3)
	button := func(i int, label string) {
		if b.MenuButton(label, width, 0, o.choice == i) {
			o.clicked = i
		}
	}
	switch o.state {
	case onlineConnecting:
		b.Text("Connecting...")
	case onlineFailed:
		b.ColorText(hurtColor, "%s", o.status)
		b.Separator()
		button(0, "Try again")
		button(1, "Back")
	case onlineBrowser:
		rooms := o.openRooms()
		b.ColorText(uiMuted, "ROOMS")
		if len(rooms) == 0 {
			b.ColorText(uiMuted, "No open rooms yet: make one, and others on the server will see it here.")
		}
		for i, r := range rooms {
			host := ""
			if len(r.Members) > 0 {
				host = r.Members[0].Name
			}
			button(i, fmt.Sprintf("%s   %d/%d   hosted by %s##room%s", r.Name, len(r.Members), r.Max, host, r.ID))
		}
		if started := len(o.rooms) - len(rooms); started > 0 {
			b.ColorText(uiMuted, "(%d more playing or full)", started)
		}
		b.Separator()
		button(len(rooms), "Create a room")
		button(len(rooms)+1, "Refresh")
		button(len(rooms)+2, "Back")
	case onlineRoom:
		room, _ := o.session.Room()
		host := o.session.IsHost()
		b.ColorText(uiAccent, "%s   (code %s)", room.Name, room.ID)
		for i, mb := range room.Members {
			tag := ""
			switch {
			case mb.ID == room.Host:
				tag = "host"
			case mb.ID == o.session.ID():
				tag = "you"
			}
			link := ""
			if mb.ID != o.session.ID() && (host || mb.ID == room.Host) {
				if _, up := o.session.Link(mb.ID); up {
					link = "connected"
				} else {
					link = "connecting..."
				}
			}
			b.Text("%d.  %s   %s   %s", i+1, mb.Name, tag, link)
		}
		for i := len(room.Members); i < room.Max; i++ {
			b.ColorText(uiMuted, "%d.  (open)", i+1)
		}
		b.Separator()
		switch {
		case !host:
			button(0, "Waiting for the host to start...##wait")
		case len(room.Members) < 2:
			button(0, "Start (waiting for players)##start")
		case !o.everyoneLinked(room):
			button(0, "Start (connecting players...)##start")
		default:
			button(0, "Start the match")
		}
		button(1, "Leave")
	}
	if o.status != "" && o.state != onlineFailed {
		b.Separator()
		b.ColorText(uiAccent, "%s", o.status)
	}
	b.End()

	b.Panel("##onlinekeys", 0.5, 0.975, hudText&^gfx.UICentered, 1.05)
	b.ColorText(uiMuted, "%s", prompt(in, "W / S  choose      Enter  select      Esc  back", "D-pad  choose      A  select      B  back"))
	b.End()
}
