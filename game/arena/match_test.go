package arena

import (
	"testing"
)

// toFight steps a match through its countdown.
func toFight(t *testing.T, m *Match) {
	t.Helper()
	for i := 0; m.Phase == PhaseCountdown && i < 600; i++ {
		m.Step(frame, nil)
	}
	if m.Phase != PhaseFight {
		t.Fatalf("phase %v after the countdown, want the fight", m.Phase)
	}
}

// kill finishes victim off (credited to by) and steps once.
func kill(m *Match, victim, by int) {
	var ev Events
	a := m.Arena
	a.hurtPlayer(a.Players[victim], a.Players[by], MaxHealth, false, WeaponRifle, a.Players[by].Eye(1), a.Players[by].Forward(), &ev)
	m.Step(frame, nil)
}

// nextRound waits out the round-over screen.
func nextRound(m *Match) {
	for i := 0; m.Phase == PhaseRoundOver && i < 600; i++ {
		m.Step(frame, nil)
	}
}

func TestCountdownHoldsPlayers(t *testing.T) {
	m := NewMatch(5, 2)
	p := m.Arena.Players[0]
	for range 30 {
		m.Step(frame, nil) // settle onto the ground
	}
	start, yaw := p.Body.Position, p.Yaw
	var shots int
	for range 60 {
		ev := m.Step(frame, []Input{{Move: [2]float32{0, 1}, Fire: true, FirePressed: true, Look: [2]float32{0.01, 0}}})
		shots += len(ev.Shots)
	}
	if moved := p.Body.Position.Sub(start).Len(); moved > 0.05 || shots > 0 {
		t.Errorf("during the countdown a player moved %.2f m and fired %d shots", moved, shots)
	}
	if p.Yaw == yaw {
		t.Error("players should be able to look around during the countdown")
	}
	toFight(t, m)
	ev := m.Step(frame, []Input{{Fire: true, FirePressed: true}})
	if len(ev.Shots) != 1 {
		t.Error("once the fight starts, firing works")
	}
}

func TestRoundGoesToTheLastOneStanding(t *testing.T) {
	m := NewMatch(5, 2)
	south := m.Arena.Players[0].Body.Position
	toFight(t, m)
	kill(m, 1, 0)
	if m.Phase != PhaseRoundOver || m.RoundWinner != 0 || m.Wins[0] != 1 || m.Wins[1] != 0 {
		t.Fatalf("after a kill: phase %v, round winner %d, wins %v", m.Phase, m.RoundWinner, m.Wins)
	}
	first := m.Arena
	nextRound(m)
	if m.Round != 2 || m.Phase != PhaseCountdown || m.Arena == first {
		t.Fatalf("round %d phase %v: want a fresh round 2 in its countdown", m.Round, m.Phase)
	}
	// Sides swap: player 1 starts where player 0 did.
	if p1 := m.Arena.Players[1].Body.Position; p1.Sub(south).Len() > 0.1 {
		t.Errorf("player 1 starts round 2 at %v, want the south spawn %v", p1, south)
	}
	if m.Arena.Players[0].Dead || m.Arena.Players[1].Health != MaxHealth || m.Arena.Standing() != 1 {
		t.Error("a new round starts with everyone alive on an untouched site")
	}
}

func TestBestOfThree(t *testing.T) {
	m := NewMatch(6, 2)
	toFight(t, m)
	kill(m, 1, 0) // 1-0
	nextRound(m)
	toFight(t, m)
	kill(m, 0, 1) // 1-1
	nextRound(m)
	if m.Round != 3 || m.Winner != -1 {
		t.Fatalf("at 1-1: round %d winner %d, want round 3 and no winner yet", m.Round, m.Winner)
	}
	toFight(t, m)
	kill(m, 0, 1) // 1-2
	if m.Winner != 1 || m.Wins[1] != RoundsToWin {
		t.Fatalf("winner %d wins %v", m.Winner, m.Wins)
	}
	nextRound(m)
	if m.Phase != PhaseMatchOver {
		t.Fatalf("phase %v, want the match over", m.Phase)
	}
	for range 120 {
		m.Step(frame, nil)
	}
	if m.Phase != PhaseMatchOver || m.Round != 3 {
		t.Error("a finished match stays finished")
	}
}

func TestTimeoutGoesToTheHealthier(t *testing.T) {
	m := NewMatch(7, 2)
	toFight(t, m)
	var ev Events
	m.Arena.hurtPlayer(m.Arena.Players[0], nil, 30, false, WeaponRifle, m.Arena.Players[0].Eye(1), m.Arena.Players[0].Forward().Scale(0), &ev)
	m.Timer = frame / 2
	m.Step(frame, nil)
	if m.Phase != PhaseRoundOver || m.RoundWinner != 1 {
		t.Fatalf("timeout: phase %v round winner %d, want player 1 (more health)", m.Phase, m.RoundWinner)
	}

	// Level on health: a draw, nobody scores.
	nextRound(m)
	toFight(t, m)
	m.Timer = frame / 2
	m.Step(frame, nil)
	if m.RoundWinner != -1 || m.Wins[0] != 0 || m.Wins[1] != 1 {
		t.Errorf("a level timeout should be a draw: round winner %d wins %v", m.RoundWinner, m.Wins)
	}
}

func TestBothDownIsADraw(t *testing.T) {
	m := NewMatch(8, 2)
	toFight(t, m)
	var ev Events
	a := m.Arena
	a.hurtPlayer(a.Players[0], a.Players[1], MaxHealth, false, WeaponLauncher, a.Players[1].Eye(1), a.Players[1].Forward().Scale(0), &ev)
	a.hurtPlayer(a.Players[1], a.Players[1], MaxHealth/selfDamage, false, WeaponLauncher, a.Players[1].Eye(1), a.Players[1].Forward().Scale(0), &ev)
	m.Step(frame, nil)
	if m.Phase != PhaseRoundOver || m.RoundWinner != -1 {
		t.Errorf("both down: phase %v round winner %d, want a draw", m.Phase, m.RoundWinner)
	}
}
