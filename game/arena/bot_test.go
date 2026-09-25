package arena

import (
	"math"
	"testing"
	"time"

	"CliffCrack/engine/mathx"
)

// withBots steps a with bots driving the players that have one (the rest
// get fixed), until done or seconds pass. It returns the time taken.
func withBots(a *Arena, bots []*Bot, fixed []Input, seconds float32, done func() bool) float32 {
	inputs := make([]Input, len(a.Players))
	for t := float32(0); t < seconds; t += frame {
		for i, p := range a.Players {
			switch {
			case i < len(bots) && bots[i] != nil:
				inputs[i] = bots[i].Think(a, p, frame)
			case i < len(fixed):
				inputs[i] = fixed[i]
			}
		}
		ev := a.Step(frame, inputs)
		for i, b := range bots {
			if b != nil {
				b.Hear(a, a.Players[i], &ev)
			}
		}
		if done() {
			return t
		}
	}
	return seconds
}

func TestBotShootsAStandingTarget(t *testing.T) {
	a := flatArena(nil, at(0, 10, 0), at(3, -10, math.Pi))
	target := a.Players[0]
	took := withBots(a, []*Bot{nil, NewBot(1)}, nil, 10, func() bool { return target.Dead })
	if !target.Dead {
		t.Fatalf("the bot didn't kill a target standing in the open in 10 s (health %v)", target.Health)
	}
	bot := a.Players[1]
	t.Logf("killed in %.1f s, accuracy %.0f%%", took, bot.Accuracy()*100)
	if took < 1 {
		t.Errorf("killed in %.2f s: too quick to be fair", took)
	}
}

func TestBotSkillLevels(t *testing.T) {
	// Against a target strafing side to side 18 m away, a harder bot lands
	// more of its shots.
	accuracy := func(skill BotSkill) float32 {
		hits, shots := 0, 0
		for seed := uint64(1); seed <= 4; seed++ {
			a := flatArena(nil, at(0, 9, 0), at(0, -9, math.Pi))
			a.InfiniteAmmo = true
			bot := NewBot(seed)
			bot.Skill = skill
			target := a.Players[0]
			target.Health = 1e6 // keep it going for the whole test
			inputs := make([]Input, 2)
			for i := range 60 * 6 {
				inputs[0] = Input{Move: [2]float32{float32(1 - 2*((i/50)%2)), 0}} // switch every 0.83 s
				inputs[1] = bot.Think(a, a.Players[1], frame)
				ev := a.Step(frame, inputs)
				bot.Hear(a, a.Players[1], &ev)
			}
			hits += a.Players[1].ShotsHit
			shots += a.Players[1].ShotsFired
		}
		return float32(hits) / float32(max(shots, 1))
	}
	easy, normal, hard := accuracy(BotEasy), accuracy(BotNormal), accuracy(BotHard)
	t.Logf("accuracy against a strafing target: easy %.0f%%, normal %.0f%%, hard %.0f%%", easy*100, normal*100, hard*100)
	if !(easy < normal && normal < hard) {
		t.Error("harder bots should hit more")
	}
	if normal < 0.15 || normal > 0.75 {
		t.Errorf("a normal bot hits %.0f%% of its shots on a strafing target: want a fair fight", normal*100)
	}
}

func TestBotDoesNotSeeThroughWalls(t *testing.T) {
	b := newBuilder("wall", mathx.Vec3{0, 0, 0}, 0)
	b.wall(-12, 0, 12, 0, 0, 3, 0.3, Metal)
	a := flatArena([]*Structure{b.finish()}, at(0, 5, 0), at(0, -5, math.Pi))
	bot := NewBot(4)
	withBots(a, []*Bot{nil, bot}, nil, 2, func() bool { return false })
	if bot.sees || bot.known {
		t.Error("the bot noticed a silent player behind a wall")
	}
	// A shot gives them away.
	a.Players[0].Pitch = 1
	withBots(a, []*Bot{nil, bot}, []Input{{Fire: true, FirePressed: true}}, frame*2, func() bool { return false })
	if !bot.known {
		t.Error("the bot should hear gunfire")
	}
}

func TestBotBreaksThroughToAHiddenPlayer(t *testing.T) {
	// A wooden wall right across the arena between them; the target stays
	// behind it and gives itself away with one shot.
	b := newBuilder("wall", mathx.Vec3{0, 0, 0}, 0)
	b.wall(-flatHalf, 0, flatHalf, 0, 0, 3, 0.3, Wood)
	wall := b.finish()
	a := flatArena([]*Structure{wall}, at(0, 5, 0), at(0, -6, math.Pi))
	target := a.Players[0]
	target.Pitch = 1
	bots := []*Bot{nil, NewBot(5)}
	withBots(a, bots, []Input{{Fire: true, FirePressed: true}}, frame*2, func() bool { return false })
	before := wall.Alive()
	took := withBots(a, bots, nil, 30, func() bool { return target.Dead })
	if !target.Dead {
		t.Fatalf("the bot didn't get to a player hiding behind a wood wall in 30 s (bot at %v, wall %d/%d standing)",
			a.Players[1].Body.Position, wall.Alive(), before)
	}
	t.Logf("broke %d panels and got the kill in %.1f s", before-wall.Alive(), took)
	if wall.Alive() == before {
		t.Error("the bot should have broken through the wall")
	}
}

func TestBotsPlayAMatch(t *testing.T) {
	m := NewMatch(11, 2)
	bots := []*Bot{NewBot(1), NewBot(2)}
	start := time.Now()
	inputs := make([]Input, 2)
	var simulated float32
	for simulated < 8*60 && m.Phase != PhaseMatchOver {
		a := m.Arena
		for i, p := range a.Players {
			inputs[i] = bots[i].Think(a, p, frame)
		}
		ev := m.Step(frame, inputs)
		for i, b := range bots {
			if m.Arena == a {
				b.Hear(a, a.Players[i], &ev)
			} else {
				b.Reset() // a new round
			}
		}
		simulated += frame
	}
	t.Logf("match over after %d rounds, %.0f s simulated in %v: wins %v", m.Round, simulated, time.Since(start), m.Wins)
	if m.Phase != PhaseMatchOver || m.Winner < 0 {
		t.Fatalf("two bots didn't finish a match in 8 minutes: round %d phase %v wins %v", m.Round, m.Phase, m.Wins)
	}
}
