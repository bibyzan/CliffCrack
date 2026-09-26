package arena

// Phase is where a match is in its round cycle.
type Phase int

const (
	PhaseCountdown Phase = iota // players are placed and choose their gadget; they can look around but not move or fire
	PhaseFight                  // one life each: last one standing takes the round
	PhaseRoundOver              // the result is shown; survivors can still move
	PhaseMatchOver              // someone has won enough rounds
)

// Match rules.
const (
	RoundsToWin   = 2   // best of three
	CountdownTime = 8.0 // choosing a gadget, the last CountdownCall of it counted down aloud
	CountdownCall = 3.0
	RoundTime     = 150.0 // s; when it runs out the healthier player takes the round
	RoundOverTime = 4.0
)

// Match is a series of single-life rounds on one site. Every round starts
// on a fresh copy of the site (the same seed), with the players swapping
// spawns each round.
type Match struct {
	Arena   *Arena // the current round
	Players int
	Round   int // 1-based
	Wins    []int
	Phase   Phase
	Timer   float32 // seconds left in the phase
	// RoundWinner is the index of the player who took the last round, or -1
	// for a draw. Winner is the match winner once Phase is PhaseMatchOver.
	RoundWinner int
	Winner      int
	Seed        uint64
	// Gadgets is each player's chosen gadget, kept from round to round.
	Gadgets []GadgetKind

	// Practice is the firing range: no rounds, and Dummies (players 1 and
	// up) strafe and get back up.
	Practice bool
	Dummies  []Dummy
}

// NewMatch starts a match for players on the site generated from seed.
func NewMatch(seed uint64, players int) *Match {
	m := &Match{Players: players, Wins: make([]int, players), Seed: seed, RoundWinner: -1, Winner: -1,
		Gadgets: make([]GadgetKind, players)}
	m.startRound()
	return m
}

func (m *Match) startRound() {
	m.Round++
	m.Arena = New(m.Seed, m.Players, m.Round-1)
	for i, p := range m.Arena.Players {
		if i < len(m.Gadgets) {
			p.Gadget = m.Gadgets[i]
		}
	}
	m.Phase, m.Timer = PhaseCountdown, CountdownTime
}

// Step advances the match by dt with one input per player. During the
// countdown and once the match is over players can only look around.
func (m *Match) Step(dt float32, inputs []Input) Events {
	if m.Practice {
		return m.stepRange(dt, inputs)
	}
	if m.Phase == PhaseCountdown {
		for i, in := range inputs {
			m.chooseGadget(i, in)
		}
	}
	if m.Phase == PhaseCountdown || m.Phase == PhaseMatchOver {
		held := make([]Input, len(inputs))
		for i, in := range inputs {
			held[i] = in.LookOnly()
		}
		inputs = held
	}
	ev := m.Arena.Step(dt, inputs)
	m.Timer -= dt

	switch m.Phase {
	case PhaseCountdown:
		if m.Timer <= 0 {
			m.Phase, m.Timer = PhaseFight, RoundTime
			m.Arena.Live = true // the launch bays fire
		}
	case PhaseFight:
		if alive := m.Arena.Alive(); alive <= 1 || m.Timer <= 0 {
			m.endRound()
		}
	case PhaseRoundOver:
		if m.Timer > 0 {
			break
		}
		if m.Winner >= 0 {
			m.Phase = PhaseMatchOver
		} else {
			m.startRound()
		}
	}
	return ev
}

// chooseGadget takes player i's gadget choice during the countdown: 1 or 2
// picks one, the swap or gadget button steps to the next.
func (m *Match) chooseGadget(i int, in Input) {
	if i >= len(m.Arena.Players) || i >= len(m.Gadgets) {
		return
	}
	g := m.Gadgets[i]
	switch {
	case in.Select >= 1 && in.Select <= int(GadgetKinds):
		g = GadgetKind(in.Select - 1)
	case in.Cycle != 0 || in.Gadget:
		g = (g + 1) % GadgetKinds
	}
	m.SetGadget(i, g)
}

// SetGadget gives player i gadget g, now and in the rounds to come.
func (m *Match) SetGadget(i int, g GadgetKind) {
	if i < len(m.Gadgets) {
		m.Gadgets[i] = g
	}
	if i < len(m.Arena.Players) {
		m.Arena.Players[i].Gadget = g
	}
}

// endRound scores the round: the last one standing, or when time runs out
// the one with the most armour and health left. A tie is a draw and scores
// nobody.
func (m *Match) endRound() {
	m.RoundWinner = -1
	best, tied := float32(0), false
	for i, p := range m.Arena.Players {
		if p.Dead {
			continue
		}
		switch {
		case p.Durability() > best:
			m.RoundWinner, best, tied = i, p.Durability(), false
		case p.Durability() == best:
			tied = true
		}
	}
	if tied {
		m.RoundWinner = -1
	}
	if m.RoundWinner >= 0 {
		m.Wins[m.RoundWinner]++
		if m.Wins[m.RoundWinner] >= RoundsToWin {
			m.Winner = m.RoundWinner
		}
	}
	m.Phase, m.Timer = PhaseRoundOver, RoundOverTime
}
