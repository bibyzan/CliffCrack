package online

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"io"
	"math"

	"CliffCrack/game/arena"
)

// Msg is a game message on a link.
type Msg struct {
	Start  *StartMsg  `json:",omitempty"` // host -> guest, reliable: the match begins
	Inputs []InputMsg `json:",omitempty"` // guest -> host, fast: the newest few (see InputBatch)
	Frame  *FrameMsg  `json:",omitempty"` // host -> guest, reliable: a step's events and the standing
	Snap   *SnapMsg   `json:",omitempty"` // host -> guest, fast: the state of everything
	Bye    bool       `json:",omitempty"` // either way, reliable: leaving the match
}

// StartMsg starts a match: every machine builds the same site from Seed.
type StartMsg struct {
	Seed  uint64
	Names []string // by player index; the host is player 0
	You   int      // the receiver's player index
}

// FrameMsg is one host step's events and the match standing after it.
type FrameMsg struct {
	Events arena.NetEvents
	Match  arena.NetMatch
}

// SnapMsg is the host's arena, in the given round, as of step Seq, with
// the last input the host had used from each player (for the guests'
// prediction) and the match standing (its clock; changes of phase and
// round come reliably, in frames).
type SnapMsg struct {
	Round int
	Seq   uint32
	Acks  []uint32
	Match arena.NetMatch
	Snap  arena.Snapshot
}

// InputMsg is a guest's input. Held things (moving, the trigger, aiming)
// are sent as they are; presses as running counts, so a lost packet can't
// lose a jump or a reload: the host acts on each count that went up. The
// aim is sent as it is too: the guest aims locally, at once.
type InputMsg struct {
	Seq                      uint32
	Move                     [2]float32
	Fire, Aim, Sprint        bool
	Yaw, Pitch               float32
	Jump, Fired, Reload      uint32
	Melee, Throw, SwitchGren uint32
	Interact, Cycle          uint32
	Select, Selects          uint32  // the last slot picked (1 or 2), and how many times
	View                     float32 // the host's time the guest was seeing others at (lag compensation)
}

// Messages bigger than compressAbove are deflated (marked by a leading
// zero byte), so a snapshot fits in a packet or two: on the fast channel a
// message that needs more packets is lost if any one of them is.
const compressAbove = 400

// Encode marshals a message.
func Encode(m Msg) []byte {
	data, err := json.Marshal(m)
	if err != nil {
		panic(err) // all our types marshal
	}
	if len(data) <= compressAbove {
		return data
	}
	var buf bytes.Buffer
	buf.WriteByte(0)
	w, _ := flate.NewWriter(&buf, flate.BestSpeed)
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

// Decode unmarshals a message.
func Decode(data []byte) (Msg, error) {
	var m Msg
	if len(data) > 0 && data[0] == 0 {
		raw, err := io.ReadAll(flate.NewReader(bytes.NewReader(data[1:])))
		if err != nil {
			return m, err
		}
		data = raw
	}
	err := json.Unmarshal(data, &m)
	return m, err
}

// InputSender turns a guest's per-step Inputs into InputMsgs.
type InputSender struct{ last InputMsg }

// Next folds in this step's input and returns the message to send.
func (s *InputSender) Next(in arena.Input, yaw, pitch float32) InputMsg {
	m := &s.last
	m.Seq++
	m.Move, m.Fire, m.Aim, m.Sprint, m.Yaw, m.Pitch, m.View = in.Move, in.Fire, in.Aim, in.Sprint, yaw, pitch, in.ViewTime
	count := func(c *uint32, pressed bool) {
		if pressed {
			*c++
		}
	}
	count(&m.Jump, in.Jump)
	count(&m.Fired, in.FirePressed)
	count(&m.Reload, in.Reload)
	count(&m.Melee, in.Melee)
	count(&m.Throw, in.Throw)
	count(&m.SwitchGren, in.SwitchGrenade)
	count(&m.Interact, in.Interact)
	count(&m.Cycle, in.Cycle != 0)
	if in.Select != 0 {
		m.Select = uint32(in.Select)
		m.Selects++
	}
	return *m
}

// InputReceiver turns a guest's InputMsgs back into the host's per-step
// Inputs: one each step, in order, as the guest took them (so the host
// moves the guest's player exactly as the guest predicted), riding out
// bunching and gaps. If none has arrived for a step, the last one's held
// buttons carry on; if too many have piled up, it catches up (presses are
// running counts, so none are lost either way).
type InputReceiver struct {
	queue []InputMsg // arrived, not yet used, in order
	used  InputMsg
	have  bool
}

// maxQueued inputs can wait (at 60 Hz, 100 ms: a hotspot bunches them)
// before older ones are skipped to catch up.
const maxQueued = 6

// Acked is the sequence number of the last input used.
func (r *InputReceiver) Acked() uint32 { return r.used.Seq }

// Receive takes a message: new ones queue in order; ones already used or
// queued are ignored.
func (r *InputReceiver) Receive(m InputMsg) {
	if r.have && m.Seq <= r.used.Seq {
		return
	}
	i := len(r.queue)
	for i > 0 && r.queue[i-1].Seq >= m.Seq {
		if r.queue[i-1].Seq == m.Seq {
			return
		}
		i--
	}
	r.queue = append(r.queue, InputMsg{})
	copy(r.queue[i+1:], r.queue[i:])
	r.queue[i] = m
}

// Input is the guest's input for this step on player p: the next one it
// sent (its held buttons, and presses new since the last used), and the
// look that turns p to where the guest was aiming.
func (r *InputReceiver) Input(p *arena.Player) arena.Input {
	if len(r.queue) > maxQueued {
		r.queue = r.queue[len(r.queue)-maxQueued:]
	}
	m := r.used
	if len(r.queue) > 0 {
		m, r.queue = r.queue[0], r.queue[1:]
	} else if !r.have {
		return arena.Input{}
	}
	u := r.used
	in := arena.Input{Move: m.Move, Fire: m.Fire, Aim: m.Aim, Sprint: m.Sprint,
		Jump: m.Jump != u.Jump, FirePressed: m.Fired != u.Fired, Reload: m.Reload != u.Reload,
		Melee: m.Melee != u.Melee, Throw: m.Throw != u.Throw, SwitchGrenade: m.SwitchGren != u.SwitchGren,
		Interact: m.Interact != u.Interact, ViewTime: m.View}
	if m.Cycle != u.Cycle {
		in.Cycle = 1
	}
	if m.Selects != u.Selects {
		in.Select = int(m.Select)
	}
	yaw := m.Yaw - p.Yaw
	yaw = float32(math.Remainder(float64(yaw), 2*math.Pi))
	in.Look = [2]float32{yaw, m.Pitch - p.Pitch}
	r.used, r.have = m, true
	return in
}

// InputBatch is what a guest sends each tick: its newest input and the few
// before, so one lost packet loses nothing.
type InputBatch struct {
	sent []InputMsg
}

// batchSize inputs go in each packet.
const batchSize = 3

// Add takes the newest input and returns the batch to send.
func (b *InputBatch) Add(m InputMsg) []InputMsg {
	b.sent = append(b.sent, m)
	if len(b.sent) > batchSize {
		b.sent = b.sent[len(b.sent)-batchSize:]
	}
	return append([]InputMsg(nil), b.sent...)
}
