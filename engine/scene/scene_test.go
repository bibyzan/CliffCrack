package scene

import (
	"math"
	"testing"

	"vkgame/engine/gfx"
	"vkgame/engine/mathx"
)

func nearVec(a, b mathx.Vec3) bool { return a.Sub(b).Len() < 1e-4 }

func origin(e *Entity) mathx.Vec3 { return e.WorldMatrix().TransformPoint(mathx.Vec3{}) }

func TestHierarchyTransforms(t *testing.T) {
	w := NewWorld()
	pivot := w.Spawn("pivot", ID{})
	pivot.Transform.Position = mathx.Vec3{10, 0, 0}
	pivot.Transform.Rotation = mathx.AxisAngle(mathx.Vec3{0, 1, 0}, math.Pi/2)
	pivot.Transform.Scale = mathx.Vec3{2, 2, 2}

	child := w.Spawn("child", pivot.ID())
	child.Transform.Position = mathx.Vec3{0, 0, -1}

	w.UpdateTransforms()
	// child at (0,0,-1) -> scaled (0,0,-2) -> rotated 90 deg about Y (-2,0,0) -> +(10,0,0)
	if got := origin(child); !nearVec(got, mathx.Vec3{8, 0, 0}) {
		t.Errorf("child world position = %v, want (8,0,0)", got)
	}
	if child.Parent() != pivot.ID() || len(pivot.Children()) != 1 {
		t.Error("parent/child links not set")
	}
}

func TestDestroyInvalidatesIDsAndRemovesSubtree(t *testing.T) {
	w := NewWorld()
	root := w.Spawn("root", ID{})
	kid := w.Spawn("kid", root.ID())
	grandkid := w.Spawn("grandkid", kid.ID())
	other := w.Spawn("other", ID{})

	w.Destroy(kid.ID())
	if w.Get(kid.ID()) != nil || w.Get(grandkid.ID()) != nil {
		t.Fatal("destroyed entity or its child is still reachable")
	}
	if len(root.Children()) != 0 {
		t.Error("destroyed child still listed under its parent")
	}
	if w.Len() != 2 {
		t.Errorf("Len = %d, want 2", w.Len())
	}

	// The freed slot is reused, but the stale ID must not see the newcomer.
	reused := w.Spawn("reused", ID{})
	if w.Get(kid.ID()) != nil || w.Get(grandkid.ID()) != nil {
		t.Error("stale ID resolved to a reused slot")
	}
	if w.Get(reused.ID()) != reused || w.Get(other.ID()) != other {
		t.Error("live IDs should still resolve")
	}
	if w.Find("reused") != reused || w.Find("kid") != nil {
		t.Error("Find returned the wrong entity")
	}
}

func TestBehavioursAndDeferredDestroy(t *testing.T) {
	w := NewWorld()
	var ticks int
	spinner := w.Spawn("spinner", ID{}).AddBehaviour(func(w *World, e *Entity, dt float32) {
		ticks++
		e.Transform.Position[0] += dt
	})
	doomed := w.Spawn("doomed", ID{})
	doomed.AddBehaviour(func(w *World, e *Entity, dt float32) {
		w.Destroy(e.ID())
		if w.Get(e.ID()) == nil {
			t.Error("destroy during Update should be deferred to the end of the update")
		}
		w.Spawn("spawned", ID{}).AddBehaviour(func(*World, *Entity, float32) {
			t.Error("entities spawned during Update should not run until the next frame")
		})
	})

	w.Update(0.5)
	if w.Time() != 0.5 {
		t.Errorf("Time = %v, want 0.5", w.Time())
	}
	if ticks != 1 || spinner.Transform.Position[0] != 0.5 {
		t.Errorf("behaviour ran %d times, x = %v", ticks, spinner.Transform.Position[0])
	}
	if w.Get(doomed.ID()) != nil {
		t.Error("doomed entity survived the update")
	}
	if !nearVec(origin(spinner), mathx.Vec3{0.5, 0, 0}) {
		t.Error("Update should refresh world matrices")
	}
}

func TestSetParentRejectsCycles(t *testing.T) {
	w := NewWorld()
	a := w.Spawn("a", ID{})
	b := w.Spawn("b", a.ID())
	c := w.Spawn("c", b.ID())

	if w.SetParent(a.ID(), c.ID()) {
		t.Error("parenting a under its own grandchild should fail")
	}
	if w.SetParent(a.ID(), a.ID()) {
		t.Error("parenting an entity to itself should fail")
	}
	if !w.SetParent(c.ID(), ID{}) || !c.Parent().IsZero() || len(b.Children()) != 0 {
		t.Error("making c a root should detach it from b")
	}
}

func TestAppendDraws(t *testing.T) {
	w := NewWorld()
	w.Spawn("empty", ID{}) // no renderable: no draw
	e := w.Spawn("box", ID{})
	e.Renderable = &Renderable{Mesh: 3, Texture: 2, Color: [4]float32{1, 0, 0, 1}}
	e.Transform.Position = mathx.Vec3{1, 2, 3}
	w.Spawn("no-mesh", ID{}).Renderable = &Renderable{}
	w.UpdateTransforms()

	draws := w.AppendDraws(nil)
	if len(draws) != 1 {
		t.Fatalf("got %d draws, want 1", len(draws))
	}
	d := draws[0]
	if d.Mesh != gfx.Mesh(3) || d.Texture != gfx.Texture(2) || d.Color != [4]float32{1, 0, 0, 1} {
		t.Errorf("draw = %+v", d)
	}
	if got := d.Model.TransformPoint(mathx.Vec3{}); !nearVec(got, mathx.Vec3{1, 2, 3}) {
		t.Errorf("draw position = %v, want (1,2,3)", got)
	}
}
