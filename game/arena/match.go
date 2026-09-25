package arena

// Phase is where a match is in its round cycle.
type Phase int

const (
	PhaseCountdown Phase = iota // players are placed; they can look around but not move or fire
	PhaseFight                  // one life each: last one standing takes the round
	PhaseRoundOver              // the result is shown; survivors can still move
	PhaseMatchOver              // someone has won enough rounds
)

// Match rules.
const (
	RoundsToWin   = 2 // best of three
	CountdownTime = 3.0
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
}

// NewMatch starts a match for players on the site generated from seed.
func NewMatch(seed uint64, players int) *Match {
	m := &Match{Players: players, Wins: make([]int, players), Seed: seed, RoundWinner: -1, Winner: -1}
	m.startRound()
	return m
}

func (m *Match) startRound() {
	m.Round++
	m.Arena = New(m.Seed, m.Players, m.Round-1)
	m.Phase, m.Timer = PhaseCountdown, CountdownTime
}

// Step advances the match by dt with one input per player. During the
// countdown and once the match is over players can only look around.
func (m *Match) Step(dt float32, inputs []Input) Events {
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

// endRound scores the round: the last one standing, or when time runs out
// the one with the most health left. A tie is a draw and scores nobody.
func (m *Match) endRound() {
	m.RoundWinner = -1
	best, tied := float32(0), false
	for i, p := range m.Arena.Players {
		if p.Dead {
			continue
		}
		switch {
		case p.Health > best:
			m.RoundWinner, best, tied = i, p.Health, false
		case p.Health == best:
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
