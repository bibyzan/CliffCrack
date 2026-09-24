package script

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vkgame/engine/scene"
)

func writeScript(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// forcePoll bypasses the poll interval and makes sure the change is visible
// even on file systems with coarse modification times.
func forcePoll(t *testing.T, h *Host) (bool, error) {
	t.Helper()
	h.lastPoll = time.Time{}
	h.stamp = "" // treat the directory as changed
	return h.Poll()
}

const moveX = `package scripts

import "vkgame/engine/scene"

// MoveX moves the entity along +X at 2 units per second.
func MoveX(w *scene.World, e *scene.Entity, dt float32) {
	e.Transform.Position[0] += 2 * dt
}

// helper has the wrong signature and must be ignored.
func Helper() int { return 1 }
`

func TestLoadRunAndReload(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, "move.go", moveX)

	var logs []string
	h, err := New(dir, func(f string, a ...any) { logs = append(logs, f) })
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(h.Names(), ","); got != "MoveX" {
		t.Fatalf("loaded behaviours = %q, want MoveX", got)
	}

	w := scene.NewWorld()
	e := w.Spawn("mover", scene.ID{}).AddBehaviour(h.Behaviour("MoveX"))
	w.Update(0.5)
	if x := e.Transform.Position[0]; x != 1 {
		t.Fatalf("after one update x = %v, want 1", x)
	}

	// Hot reload: same name, new code. The attached behaviour picks it up.
	writeScript(t, dir, "move.go", strings.Replace(moveX, "2 * dt", "10 * dt", 1))
	if ok, err := forcePoll(t, h); !ok || err != nil {
		t.Fatalf("reload = %v, %v", ok, err)
	}
	w.Update(0.5)
	if x := e.Transform.Position[0]; x != 6 {
		t.Fatalf("after reload x = %v, want 6 (1 + 10*0.5)", x)
	}

	// A broken edit keeps the previous version running.
	writeScript(t, dir, "move.go", "package scripts\nfunc MoveX( {")
	if ok, err := forcePoll(t, h); ok || err == nil {
		t.Fatal("reload of broken code should fail")
	}
	w.Update(0.5)
	if x := e.Transform.Position[0]; x != 11 {
		t.Fatalf("after failed reload x = %v, want 11 (old code still running)", x)
	}
	if h.Version() != 2 {
		t.Errorf("version = %d, want 2", h.Version())
	}
}

func TestPanicDisablesBehaviourUntilReload(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, "boom.go", `package scripts

import "vkgame/engine/scene"

func Boom(w *scene.World, e *scene.Entity, dt float32) {
	var m map[string]int
	m["x"] = 1 // nil map write panics
}
`)
	h, err := New(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := scene.NewWorld()
	w.Spawn("e", scene.ID{}).AddBehaviour(h.Behaviour("Boom"))

	w.Update(0.1) // must not crash the test
	if !h.failed["Boom"] {
		t.Fatal("panicking behaviour should be disabled")
	}

	if _, err := forcePoll(t, h); err != nil {
		t.Fatal(err)
	}
	if h.failed["Boom"] {
		t.Error("reload should re-enable behaviours")
	}
}

func TestUnknownBehaviourIsNoop(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, "a.go", moveX)
	h, err := New(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := scene.NewWorld()
	w.Spawn("e", scene.ID{}).AddBehaviour(h.Behaviour("DoesNotExist"))
	w.Update(1) // no panic
}
