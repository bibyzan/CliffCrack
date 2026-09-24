// Package scene is the entity model: a World of named entities with
// hierarchical transforms, optional renderables and per-frame behaviours.
// It is pure Go (it builds gfx draw lists, it never talks to the GPU).
package scene

import (
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
)

// ID refers to an entity. It stays valid until the entity is destroyed; after
// that, lookups with it return nil even if the slot is reused. The zero ID
// means "no entity" (e.g. no parent).
type ID struct {
	index uint32
	gen   uint32 // 0 is never used by a live entity
}

func (id ID) IsZero() bool { return id == ID{} }

// Transform is an entity's placement relative to its parent.
type Transform struct {
	Position mathx.Vec3
	Rotation mathx.Quat
	Scale    mathx.Vec3
}

func NewTransform() Transform {
	return Transform{Rotation: mathx.QuatIdentity(), Scale: mathx.Vec3{1, 1, 1}}
}

// Matrix is translate * rotate * scale.
func (t Transform) Matrix() mathx.Mat4 {
	return mathx.Translate(t.Position[0], t.Position[1], t.Position[2]).
		Mul(t.Rotation.Mat4()).
		Mul(mathx.Scale(t.Scale[0], t.Scale[1], t.Scale[2]))
}

// Renderable makes an entity draw a mesh.
type Renderable struct {
	Mesh    gfx.Mesh
	Texture gfx.Texture
	Color   [4]float32 // linear RGBA
	Flags   gfx.DrawFlags
}

// Behaviour runs once per World.Update for the entity it is attached to.
type Behaviour func(w *World, e *Entity, dt float32)

type Entity struct {
	Name       string
	Transform  Transform
	Renderable *Renderable
	Behaviours []Behaviour

	id       ID
	parent   ID
	children []ID
	world    mathx.Mat4
	dying    bool
}

func (e *Entity) ID() ID         { return e.id }
func (e *Entity) Parent() ID     { return e.parent }
func (e *Entity) Children() []ID { return e.children }

// WorldMatrix is the entity's object-to-world transform as of the last
// World.Update (or UpdateTransforms).
func (e *Entity) WorldMatrix() mathx.Mat4 { return e.world }

// AddBehaviour attaches b and returns e for chaining.
func (e *Entity) AddBehaviour(b Behaviour) *Entity {
	e.Behaviours = append(e.Behaviours, b)
	return e
}

type slot struct {
	gen    uint32
	entity *Entity // nil when free
}

type World struct {
	slots    []slot
	free     []uint32
	pending  []ID // destroyed during Update, removed at the end of it
	updating bool
	time     float64
}

// Time is the total simulated time in seconds (the sum of all Update dts).
// During Update it already includes the current frame's dt.
func (w *World) Time() float32 { return float32(w.time) }

func NewWorld() *World { return &World{} }

// Spawn creates an entity under parent (zero ID for a root entity).
// If parent is not a live entity, the new entity becomes a root.
func (w *World) Spawn(name string, parent ID) *Entity {
	var index uint32
	if n := len(w.free); n > 0 {
		index = w.free[n-1]
		w.free = w.free[:n-1]
	} else {
		index = uint32(len(w.slots))
		w.slots = append(w.slots, slot{})
	}
	s := &w.slots[index]
	s.gen++
	e := &Entity{Name: name, Transform: NewTransform(), id: ID{index, s.gen}, world: mathx.Identity()}
	s.entity = e
	if p := w.Get(parent); p != nil {
		e.parent = parent
		p.children = append(p.children, e.id)
	}
	return e
}

// Get returns the live entity for id, or nil.
func (w *World) Get(id ID) *Entity {
	if id.gen == 0 || int(id.index) >= len(w.slots) {
		return nil
	}
	s := w.slots[id.index]
	if s.gen != id.gen || s.entity == nil {
		return nil
	}
	return s.entity
}

// Find returns the first live entity with the given name, or nil.
func (w *World) Find(name string) *Entity {
	for _, s := range w.slots {
		if s.entity != nil && s.entity.Name == name {
			return s.entity
		}
	}
	return nil
}

// Len is the number of live entities.
func (w *World) Len() int { return len(w.slots) - len(w.free) }

// Destroy removes an entity and all its descendants. During Update the removal
// is deferred to the end of the update, so iteration stays valid.
func (w *World) Destroy(id ID) {
	e := w.Get(id)
	if e == nil || e.dying {
		return
	}
	if w.updating {
		e.dying = true
		w.pending = append(w.pending, id)
		return
	}
	w.remove(e)
}

func (w *World) remove(e *Entity) {
	for _, child := range e.children {
		if c := w.Get(child); c != nil {
			c.parent = ID{} // detached first so it doesn't edit our slice while we range over it
			w.remove(c)
		}
	}
	if p := w.Get(e.parent); p != nil {
		p.children = removeID(p.children, e.id)
	}
	w.slots[e.id.index].entity = nil
	w.free = append(w.free, e.id.index)
}

// SetParent re-parents child under parent (zero ID makes it a root). The local
// transform is kept, so the entity may move in world space. Returns false if
// either entity is dead or the change would create a cycle.
func (w *World) SetParent(child, parent ID) bool {
	c := w.Get(child)
	if c == nil {
		return false
	}
	if !parent.IsZero() {
		if w.Get(parent) == nil {
			return false
		}
		for a := parent; !a.IsZero(); a = w.Get(a).parent {
			if a == child {
				return false
			}
		}
	}
	if old := w.Get(c.parent); old != nil {
		old.children = removeID(old.children, child)
	}
	c.parent = parent
	if p := w.Get(parent); p != nil {
		p.children = append(p.children, child)
	}
	return true
}

// Update runs every entity's behaviours, applies deferred destroys and
// recomputes world matrices. Entities spawned during Update start next frame.
func (w *World) Update(dt float32) {
	w.time += float64(dt)
	w.updating = true
	count := len(w.slots)
	for i := 0; i < count; i++ {
		e := w.slots[i].entity
		if e == nil || e.dying {
			continue
		}
		for _, b := range e.Behaviours {
			b(w, e, dt)
			if e.dying {
				break
			}
		}
	}
	w.updating = false
	for _, id := range w.pending {
		if e := w.Get(id); e != nil {
			w.remove(e)
		}
	}
	w.pending = w.pending[:0]
	w.UpdateTransforms()
}

// UpdateTransforms recomputes world matrices from the hierarchy.
func (w *World) UpdateTransforms() {
	for _, s := range w.slots {
		if s.entity != nil && s.entity.parent.IsZero() {
			w.updateSubtree(s.entity, mathx.Identity())
		}
	}
}

func (w *World) updateSubtree(e *Entity, parent mathx.Mat4) {
	e.world = parent.Mul(e.Transform.Matrix())
	for _, child := range e.children {
		if c := w.Get(child); c != nil {
			w.updateSubtree(c, e.world)
		}
	}
}

// AppendDraws appends one draw command per renderable entity, using world
// matrices from the last update.
func (w *World) AppendDraws(out []gfx.DrawCmd) []gfx.DrawCmd {
	for _, s := range w.slots {
		e := s.entity
		if e == nil || e.Renderable == nil || e.Renderable.Mesh == 0 {
			continue
		}
		out = append(out, gfx.DrawCmd{
			Model:   e.world,
			Color:   e.Renderable.Color,
			Texture: e.Renderable.Texture,
			Flags:   e.Renderable.Flags,
			Mesh:    e.Renderable.Mesh,
		})
	}
	return out
}

func removeID(ids []ID, id ID) []ID {
	for i, v := range ids {
		if v == id {
			return append(ids[:i], ids[i+1:]...)
		}
	}
	return ids
}
