package game

import (
	"time"

	"CliffCrack/engine/audio"
	"CliffCrack/game/arena"
)

func ms(n float64) time.Duration { return time.Duration(n * float64(time.Millisecond)) }

// Layer shorthands.
func tone(w audio.Wave, from, to float32, delay, length float64, decay, volume float32) audio.Layer {
	return audio.Layer{Wave: w, Start: from, End: to, Delay: ms(delay), Length: ms(length), Decay: decay, Volume: volume}
}

func hiss(delay, length float64, decay, volume, lowPass, highPass float32) audio.Layer {
	return audio.Layer{Wave: audio.Noise, Delay: ms(delay), Length: ms(length), Decay: decay, Volume: volume,
		LowPass: lowPass, HighPass: highPass}
}

// click is a tiny dry tick of high noise: a trigger, a latch, a shell.
func click(delay float64, volume float32) audio.Layer { return hiss(delay, 18, 180, volume, 0, 2500) }

// newArenaSounds synthesises the Arena's sounds: paintball markers (gas
// pops and splats rather than gunshots), armour that tinks and shatters,
// structures that crack, crunch and clang by material, and booms that echo
// round the chasm.
func newArenaSounds() arenaSounds {
	s := arenaSounds{
		// Hits you land: a crisp hit-marker tick over a wet thwack on flesh
		// (as clear as the armour's tink: every hit should be heard), a
		// bright ding for the head.
		hit: audio.Synth(0.7, click(0, 1), tone(audio.Square, 1900, 1500, 0, 45, 60, 0.35),
			hiss(0, 60, 55, 0.8, 3000, 250), tone(audio.Sine, 320, 180, 0, 60, 45, 0.5)),
		headshot: audio.Synth(0.5, tone(audio.Sine, 1760, 1760, 0, 300, 12, 0.8), tone(audio.Sine, 2640, 2640, 0, 220, 18, 0.4), click(0, 0.6)),
		// Paint on your health: a body thud.
		hurt: audio.Synth(0.7, tone(audio.Sine, 120, 55, 0, 160, 16, 1), hiss(0, 120, 22, 0.8, 700, 0)),
		// A kill: a falling whoomp and a low chord.
		kill: audio.Synth(0.9, tone(audio.Triangle, 440, 110, 0, 520, 5, 0.8), hiss(0, 400, 8, 0.4, 1200, 0),
			tone(audio.Sine, 220, 220, 60, 600, 4, 0.35), tone(audio.Sine, 330, 330, 60, 600, 4, 0.25)),
		// Reloading: the hopper lid, a rattle of paint, the lid shut.
		reload: audio.Synth(0.4, click(0, 1), hiss(40, 160, 18, 0.35, 2500, 600), click(210, 1)),
		empty:  audio.Synth(0.3, click(0, 1), tone(audio.Sine, 900, 900, 0, 20, 120, 0.2)),
		jump:   audio.Synth(0.22, hiss(0, 90, 30, 1, 1400, 200), tone(audio.Sine, 180, 240, 0, 80, 30, 0.4)),
		land:   audio.Synth(0.55, hiss(0, 110, 26, 1, 450, 0), tone(audio.Sine, 95, 50, 0, 110, 22, 0.8)),
		// Traversal: a slide's long gritty scrape; a vault's whoosh and a
		// palm slapped on the top; a climb's scramble and the pull-up.
		slide: audio.Synth(0.5, hiss(0, 520, 4, 0.9, 2200, 300), hiss(0, 480, 5, 0.5, 700, 0)),
		vault: audio.Synth(0.4, hiss(0, 140, 18, 0.8, 2400, 500), tone(audio.Sine, 140, 90, 60, 60, 40, 0.6), hiss(60, 40, 60, 0.6, 3000, 800)),
		climb: audio.Synth(0.5, hiss(0, 90, 30, 0.6, 3000, 700), hiss(120, 90, 30, 0.6, 3000, 700), tone(audio.Sine, 110, 70, 200, 120, 18, 0.7)),
		// The hammer: a whoosh, and a heavy crunching thud.
		swing: audio.Synth(0.25, audio.Layer{Wave: audio.Noise, Length: ms(200), Attack: ms(70), Decay: 14, Volume: 1, LowPass: 2600, HighPass: 400}),
		thud:  audio.Synth(0.95, tone(audio.Sine, 85, 38, 0, 260, 11, 1), hiss(0, 160, 28, 0.9, 1600, 0), hiss(10, 90, 40, 0.5, 0, 900)),
		// The elbow: a short whoosh, and a dull knock.
		elbow: audio.Synth(0.2, audio.Layer{Wave: audio.Noise, Length: ms(120), Attack: ms(30), Decay: 22, Volume: 0.9, LowPass: 3000, HighPass: 600}),
		punch: audio.Synth(0.6, tone(audio.Sine, 140, 70, 0, 120, 26, 1), hiss(0, 80, 40, 0.7, 1800, 100)),
		// The grapple: the hook zipping out, catching with a clank, the line
		// snapping back; and the hammer lifted off the back.
		hook: audio.Synth(0.45, hiss(0, 180, 12, 0.8, 6000, 900), tone(audio.Saw, 700, 1500, 0, 160, 14, 0.25), click(0, 0.7)),
		catch: audio.Synth(0.5, tone(audio.Square, 520, 480, 0, 90, 30, 0.4), tone(audio.Sine, 1300, 1250, 0, 140, 20, 0.5),
			click(0, 1), hiss(0, 60, 50, 0.5, 5000, 800)),
		release: audio.Synth(0.3, click(0, 0.8), tone(audio.Sine, 900, 500, 0, 90, 30, 0.4)),
		draw:    audio.Synth(0.35, click(0, 0.7), hiss(0, 140, 18, 0.6, 1800, 300), tone(audio.Sine, 220, 160, 20, 120, 20, 0.5)),
		// The paint grenade launcher: a hollow thoonk.
		launch: audio.Synth(0.7, tone(audio.Sine, 150, 60, 0, 170, 14, 1), hiss(0, 90, 30, 0.6, 900, 0), click(0, 0.4)),
		// A grenade going off: a burst of paint and gas, echoing off the walls.
		boom: audio.Echo(audio.Synth(1, hiss(0, 1100, 4.5, 1, 900, 0), tone(audio.Sine, 65, 30, 0, 800, 5, 0.9),
			hiss(0, 150, 22, 0.6, 0, 2800), hiss(20, 400, 9, 0.5, 2500, 400)), ms(230), 0.3, 2),
		swap: audio.Synth(0.2, click(0, 1), tone(audio.Sine, 1300, 1100, 0, 30, 60, 0.4), click(70, 0.6)),
		boost: audio.Synth(0.6, audio.Layer{Wave: audio.Noise, Length: ms(520), Attack: ms(40), Decay: 3.5, Volume: 0.8, LowPass: 3200, HighPass: 300},
			tone(audio.Triangle, 190, 950, 0, 480, 3.5, 0.6)),
		tick: audio.Synth(0.35, tone(audio.Sine, 880, 880, 0, 140, 22, 1), tone(audio.Sine, 1760, 1760, 0, 90, 35, 0.3)),
		fight: audio.Synth(0.5, tone(audio.Saw, 440, 440, 0, 380, 3, 0.35), tone(audio.Saw, 660, 660, 0, 380, 3, 0.25),
			tone(audio.Sine, 880, 880, 0, 380, 3, 0.4)),
		win: audio.Synth(0.5, tone(audio.Triangle, 523, 523, 0, 300, 6, 0.8), tone(audio.Triangle, 659, 659, 110, 300, 6, 0.8),
			tone(audio.Triangle, 784, 784, 220, 520, 4, 0.8), tone(audio.Triangle, 1047, 1047, 220, 520, 4, 0.4)),
		lose: audio.Synth(0.5, tone(audio.Triangle, 392, 392, 0, 300, 6, 0.8), tone(audio.Triangle, 330, 330, 130, 300, 6, 0.8),
			tone(audio.Triangle, 262, 250, 260, 640, 3.5, 0.8)),
		// A collapse: a long low rumble with the rattle of rubble in it.
		collapse: audio.Echo(audio.Synth(1, hiss(0, 1700, 2.4, 1, 240, 0), hiss(80, 1200, 5, 0.45, 1400, 300),
			tone(audio.Sine, 48, 32, 0, 1500, 2.8, 0.6)), ms(300), 0.25, 2),
		// Armour: paint on it tinks off; breaking, it shatters like glass
		// over a thump; coming back, a rising shimmer.
		armour: audio.Synth(0.3, tone(audio.Sine, 2600, 2300, 0, 110, 32, 0.8), tone(audio.Sine, 3900, 3700, 0, 80, 40, 0.3), hiss(0, 40, 90, 0.4, 0, 5000)),
		pop:    audio.Synth(0.9, hiss(0, 320, 11, 1, 0, 2800), tone(audio.Sine, 1500, 380, 0, 260, 10, 0.7), tone(audio.Sine, 130, 60, 0, 200, 15, 0.8)),
		// ... coming back, a soft airy swell under a warm rising tone, and a
		// quiet chime as it settles: heard, not startling.
		recharge: audio.Synth(0.22,
			audio.Layer{Wave: audio.Noise, Length: ms(520), Attack: ms(260), Decay: 5, Volume: 0.3, LowPass: 2000, HighPass: 350},
			audio.Layer{Wave: audio.Sine, Start: 392, End: 587, Length: ms(480), Attack: ms(220), Decay: 3, Volume: 0.45},
			audio.Layer{Wave: audio.Sine, Start: 1175, End: 1175, Delay: ms(300), Length: ms(420), Attack: ms(12), Decay: 9, Volume: 0.18},
			audio.Layer{Wave: audio.Sine, Start: 1760, End: 1760, Delay: ms(300), Length: ms(320), Attack: ms(12), Decay: 12, Volume: 0.07}),
		// Grenades: the pin and a whoosh of the throw; a sticky's wet slap
		// and arming chirp as it sticks; its bright burst of paint.
		throw: audio.Synth(0.4, click(0, 0.8), audio.Layer{Wave: audio.Noise, Delay: ms(60), Length: ms(220), Attack: ms(80), Decay: 12, Volume: 1, LowPass: 2400, HighPass: 300}),
		stick: audio.Synth(0.6, hiss(0, 70, 45, 1, 1800, 150), tone(audio.Sine, 300, 150, 0, 60, 40, 0.6),
			tone(audio.Square, 1800, 1800, 80, 60, 30, 0.25), tone(audio.Square, 2400, 2400, 170, 60, 30, 0.25)),
		stickyBoom: audio.Echo(audio.Synth(1, hiss(0, 900, 5, 1, 1800, 0), tone(audio.Sine, 90, 35, 0, 700, 5, 0.9),
			tone(audio.Sine, 1400, 300, 0, 300, 9, 0.35), hiss(0, 200, 14, 0.6, 0, 2600)), ms(230), 0.3, 2),
		// Reloads: the magazine released and sliding out; seated with a
		// clack; a handle racked; a shell thumbed in; the pump.
		magOut:  audio.Synth(0.4, click(0, 1), hiss(10, 90, 30, 0.6, 3500, 900), tone(audio.Sine, 700, 450, 0, 60, 45, 0.3)),
		magIn:   audio.Synth(0.55, hiss(0, 50, 60, 0.5, 2500, 400), click(40, 1), tone(audio.Sine, 380, 260, 40, 70, 45, 0.6), click(60, 0.7)),
		charge:  audio.Synth(0.5, click(0, 0.9), hiss(10, 110, 30, 0.7, 4000, 1200), click(150, 1), tone(audio.Sine, 900, 1100, 150, 40, 60, 0.3)),
		shellIn: audio.Synth(0.4, hiss(0, 45, 60, 0.6, 3000, 600), click(35, 1), tone(audio.Sine, 520, 400, 35, 50, 50, 0.4)),
		pump:    audio.Synth(0.55, hiss(0, 120, 25, 0.8, 3200, 500), click(0, 0.8), click(130, 1), hiss(140, 90, 35, 0.6, 3500, 700)),
		// Taking a weapon: a rattle and the snap of it coming up.
		pickup: audio.Synth(0.45, hiss(0, 120, 25, 0.5, 3000, 700), click(90, 1), tone(audio.Sine, 700, 900, 90, 60, 40, 0.4), click(160, 0.8)),
		// Paint landing on a surface: a wet splut.
		splat: audio.Synth(0.25, hiss(0, 55, 60, 1, 2200, 300), tone(audio.Sine, 420, 200, 0, 50, 60, 0.5)),
		// Paint landing right by you: a ball fizzing past, then a big wet
		// SPLAT: a heavy slap with a thud in it and paint spattering.
		nearSplat: audio.Variants(3, func(i int) *audio.Sound {
			return audio.SynthTake(i, 0.6,
				audio.Layer{Wave: audio.Noise, Length: ms(60), Attack: ms(45), Decay: 20, Volume: 0.35, LowPass: 7000, HighPass: 2500}, // the fizz past
				hiss(55, 18, 140, 0.9, 0, 1400),               // the slap
				hiss(55, 150, 26, 1, 1500, 90),                // the splat
				tone(audio.Sine, 190, 60, 55, 70, 38, 0.9),    // the thud
				hiss(95, 120, 30, 0.35, 5000, 1800),           // spatter
				tone(audio.Sine, 900, 380, 110, 45, 55, 0.18)) // a drip
		}),
	}

	// The markers: gas pops with a click of the trigger, weightier up the
	// range; the shotgun racks its pump; the sniper cracks and echoes.
	//
	// The rifle fires every 75 ms, so each shot has to be short and read
	// as a shot: a crisp transient, a tuned pop with body under it, a puff
	// of gas, and the bolt's tick. Four takes, a little apart in pitch, so
	// a burst isn't one sound repeating (and each cuts the last off: see
	// the choke where they're played).
	s.guns[arena.WeaponRifle] = audio.Variants(4, func(i int) *audio.Sound {
		k := float32([]float64{1, 1.05, 0.96, 1.02}[i])
		return audio.SynthTake(i, 0.42,
			hiss(0, 7, 300, 0.55, 0, 3500),                 // the transient
			tone(audio.Sine, 560*k, 190*k, 0, 38, 70, 1),   // the pop
			tone(audio.Sine, 150*k, 75*k, 0, 60, 45, 0.55), // the body
			audio.Layer{Wave: audio.Noise, Length: ms(65), Attack: ms(3), Decay: 50, Volume: 0.4, LowPass: 4200, HighPass: 900}, // gas
			click(28, 0.22)) // the bolt
	})
	s.guns[arena.WeaponPistol] = audio.Synth(0.5, hiss(0, 80, 42, 1, 7500, 700), tone(audio.Sine, 190, 85, 0, 80, 32, 0.8), click(0, 0.6))
	s.guns[arena.WeaponShotgun] = audio.Synth(0.9, hiss(0, 260, 16, 1, 3200, 0), tone(audio.Sine, 95, 42, 0, 220, 12, 1),
		hiss(0, 60, 50, 0.5, 0, 2000), click(360, 0.7), hiss(380, 70, 45, 0.4, 3000, 800), click(470, 0.8))
	s.guns[arena.WeaponSniper] = audio.Echo(audio.Synth(0.85, hiss(0, 45, 70, 1, 0, 2500), tone(audio.Sine, 1300, 320, 0, 60, 45, 0.5),
		tone(audio.Sine, 75, 40, 0, 420, 6, 0.8), hiss(10, 700, 5, 0.3, 1100, 0)), ms(280), 0.28, 2)

	// Structures by material: wood cracks, brick and concrete crunch lower
	// and longer, glass tinkles, metal clangs.
	s.breaks[arena.Wood] = audio.Synth(0.45, hiss(0, 170, 26, 1, 2600, 200), tone(audio.Sine, 190, 110, 0, 120, 30, 0.6), click(30, 0.4))
	s.breaks[arena.Brick] = audio.Synth(0.55, hiss(0, 240, 17, 1, 1500, 100), tone(audio.Sine, 120, 70, 0, 160, 18, 0.5), hiss(60, 150, 30, 0.4, 3000, 800))
	s.breaks[arena.Concrete] = audio.Synth(0.65, hiss(0, 320, 11, 1, 900, 0), tone(audio.Sine, 75, 45, 0, 250, 12, 0.7), hiss(50, 200, 22, 0.4, 2500, 600))
	s.breaks[arena.Glass] = audio.Synth(0.32, hiss(0, 250, 10, 0.8, 0, 3000),
		tone(audio.Sine, 3100, 3100, 0, 220, 14, 0.4), tone(audio.Sine, 4200, 4200, 40, 200, 16, 0.35),
		tone(audio.Sine, 5300, 5300, 90, 180, 18, 0.3), tone(audio.Sine, 3700, 3700, 150, 160, 20, 0.25))
	s.breaks[arena.Metal] = audio.Synth(0.4, tone(audio.Sine, 410, 405, 0, 700, 5, 0.6), tone(audio.Sine, 1070, 1060, 0, 600, 6, 0.4),
		tone(audio.Sine, 1810, 1800, 0, 500, 8, 0.3), hiss(0, 60, 50, 0.5, 0, 2000))
	s.breaks[arena.Panel] = audio.Synth(0.55, hiss(0, 220, 19, 1, 2000, 150), tone(audio.Sine, 150, 85, 0, 180, 16, 0.6), click(20, 0.3))
	s.breaks[arena.Plate] = audio.Synth(0.7, hiss(0, 360, 9, 1, 700, 0), tone(audio.Sine, 62, 36, 0, 320, 9, 0.8), hiss(40, 200, 20, 0.35, 2200, 500))
	return s
}

// UI sounds: a soft tick moving between items, a two-note chime to pick one.
var (
	uiMoveSound = func() *audio.Sound {
		return audio.Synth(0.22, click(0, 0.6), tone(audio.Sine, 1200, 1100, 0, 40, 60, 0.5))
	}
	uiPickSound = func() *audio.Sound {
		return audio.Synth(0.45, tone(audio.Triangle, 660, 660, 0, 140, 14, 0.8), tone(audio.Triangle, 990, 990, 70, 220, 9, 0.8))
	}
)

// newRunSounds are Run mode's: a snowy whump off the ground, a crunch on
// landing, and a wipeout that thumps and sprays snow.
func newRunSounds() runSounds {
	return runSounds{
		jump: audio.Synth(0.45, hiss(0, 160, 18, 0.8, 1800, 250), tone(audio.Sine, 160, 260, 0, 140, 16, 0.6)),
		land: audio.Synth(0.7, hiss(0, 180, 20, 1, 1200, 80), tone(audio.Sine, 110, 55, 0, 120, 20, 0.8), hiss(20, 120, 35, 0.4, 0, 2500)),
		crash: audio.Echo(audio.Synth(1, tone(audio.Sine, 140, 38, 0, 450, 7, 1), hiss(0, 700, 5, 0.9, 1600, 100),
			hiss(0, 90, 40, 0.6, 0, 2000)), ms(260), 0.25, 2),
		move: uiMoveSound(),
		pick: uiPickSound(),
		// A rising rush of air.
		boost: audio.Synth(0.8, hiss(0, 700, 3, 0.9, 5000, 400), tone(audio.Saw, 180, 720, 0, 500, 5, 0.35),
			tone(audio.Sine, 360, 1440, 60, 440, 6, 0.3)),
		// A bright chime, ringing on.
		shield: audio.Echo(audio.Synth(0.6, tone(audio.Sine, 880, 880, 0, 500, 7, 0.6),
			tone(audio.Sine, 1320, 1320, 40, 460, 8, 0.45), tone(audio.Triangle, 1760, 1760, 80, 420, 9, 0.3)),
			ms(120), 0.35, 3),
		// Rock or wood bursting apart.
		smash: audio.Synth(0.9, hiss(0, 260, 14, 1, 3000, 150), tone(audio.Square, 220, 60, 0, 160, 18, 0.35),
			click(0, 0.8), click(35, 0.5)),
	}
}
