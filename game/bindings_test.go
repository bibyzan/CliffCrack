package game

import (
	"testing"

	"CliffCrack/engine/input"
)

func TestMenuNavigationFromEveryDevice(t *testing.T) {
	press := func(feed func(*input.State)) int {
		var in input.State
		in.NewFrame()
		feed(&in)
		return navY(&in)
	}
	cases := map[string]struct {
		feed func(*input.State)
		want int
	}{
		"key W":       {func(in *input.State) { in.KeyEvent(input.KeyW, true) }, -1},
		"arrow down":  {func(in *input.State) { in.KeyEvent(input.KeyDown, true) }, 1},
		"d-pad up":    {func(in *input.State) { in.PadEvent(input.PadUp, true) }, -1},
		"stick down":  {func(in *input.State) { in.PadAxisEvent(input.PadLeftY, 0.9) }, 1},
		"stick drift": {func(in *input.State) { in.PadAxisEvent(input.PadLeftY, 0.3) }, 0},
	}
	for name, c := range cases {
		if got := press(c.feed); got != c.want {
			t.Errorf("%s: navY = %d, want %d", name, got, c.want)
		}
	}
}

func TestPromptsFollowTheLastDevice(t *testing.T) {
	var in input.State
	in.PadEvent(input.PadA, true)
	if got := prompt(&in, "keys", "pad"); got != "pad" {
		t.Errorf("after a pad press the prompt is %q", got)
	}
	in.KeyEvent(input.KeyEnter, true)
	if got := prompt(&in, "keys", "pad"); got != "keys" {
		t.Errorf("after a key press the prompt is %q", got)
	}
	if !confirmPressed(&in) {
		t.Error("Enter should confirm")
	}
}
