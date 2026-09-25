package online

import (
	"encoding/json"
	"math"

	"CliffCrack/game/arena"
)

// Msg is a game message on a link.
type Msg struct {
	Start *StartMsg `json:",omitempty"` // host -> guest, reliable: the match begins
	Input *InputMsg `json:",omitempty"` // guest -> host, fast
	Frame *FrameMsg `json:",omitempty"` // host -> guest, reliable: a step's events and the standing
	Snap  *SnapMsg  `json:",omitempty"` // host -> guest, fast: the state of everything
	Bye   bool      `json:",omitempty"` // either way, reliable: leaving the match
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

// SnapMsg is the host's arena, in the given round, as of step Seq.
type SnapMsg struct {
	Round int
	Seq   uint32
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
	Select, Selects          uint32 // the last slot picked (1 or 2), and how many times
}

// Encode marshals a message.
func Encode(m Msg) []byte {
	data, err := json.Marshal(m)
	if err != nil {
		panic(err) // all our types marshal
	}
	return data
}

// Decode unmarshals a message.
func Decode(data []byte) (Msg, error) {
	var m Msg
	err := json.Unmarshal(data, &m)
	return m, err
}

// InputSender turns a guest's per-step Inputs into InputMsgs.
type InputSender struct{ last InputMsg }

// Next folds in this step's input and returns the message to send.
func (s *InputSender) Next(in arena.Input, yaw, pitch float32) InputMsg {
	m := &s.last
	m.Seq++
	m.Move, m.Fire, m.Aim, m.Sprint, m.Yaw, m.Pitch = in.Move, in.Fire, in.Aim, in.Sprint, yaw, pitch
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
// Inputs, one each step, however many arrived (or none).
type InputReceiver struct {
	latest, used InputMsg
	have         bool
}

// Receive takes a message (ignoring ones older than the latest).
func (r *InputReceiver) Receive(m InputMsg) {
	if !r.have || m.Seq > r.latest.Seq {
		r.latest, r.have = m, true
	}
}

// Input is the guest's input for this step on player p: held things as
// they last were, presses that are new since the last step, and the look
// that turns p to where the guest is aiming.
func (r *InputReceiver) Input(p *arena.Player) arena.Input {
	if !r.have {
		return arena.Input{}
	}
	m, u := r.latest, r.used
	in := arena.Input{Move: m.Move, Fire: m.Fire, Aim: m.Aim, Sprint: m.Sprint,
		Jump: m.Jump != u.Jump, FirePressed: m.Fired != u.Fired, Reload: m.Reload != u.Reload,
		Melee: m.Melee != u.Melee, Throw: m.Throw != u.Throw, SwitchGrenade: m.SwitchGren != u.SwitchGren,
		Interact: m.Interact != u.Interact}
	if m.Cycle != u.Cycle {
		in.Cycle = 1
	}
	if m.Selects != u.Selects {
		in.Select = int(m.Select)
	}
	yaw := m.Yaw - p.Yaw
	yaw = float32(math.Remainder(float64(yaw), 2*math.Pi))
	in.Look = [2]float32{yaw, m.Pitch - p.Pitch}
	r.used = m
	return in
}
