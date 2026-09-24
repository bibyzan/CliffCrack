// Package game is the gameplay layer. It only talks to the engine through
// plain Go types; it never touches cgo or Vulkan.
package game

import (
	"fmt"
	"image/color"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"vkgame/engine/asset"
	"vkgame/engine/audio"
	"vkgame/engine/geom"
	"vkgame/engine/input"
	"vkgame/engine/mathx"
	"vkgame/engine/physics"
	"vkgame/engine/render"
	"vkgame/engine/scene"
	"vkgame/engine/script"
	"vkgame/engine/ui"
)

var worldUp = mathx.Vec3{0, 1, 0}

type Game struct {
	world   *scene.World
	camera  cameraRig
	scripts *script.Host // nil when running without scripts

	// Tunables exposed in the debug UI.
	sunIntensity float32
	ambientLevel float32
	timeScale    float32

	phys     *physics.World
	links    []physLink
	balls    []scene.ID
	ballMesh render.Mesh
	rng      *rand.Rand

	sound       *audio.Mixer // nil without audio
	volume      float32
	dropSound   *audio.Sound
	clearSound  *audio.Sound
	bounceSound *audio.Sound
}

// Stats are engine numbers shown in the debug UI.
type Stats struct {
	FPS     float32
	FrameMS float32
	Draws   int
}

type Options struct {
	Model      string       // optional glTF file shown in the centre instead of the sphere
	ScriptsDir string       // optional directory of hot-reloadable behaviours
	Audio      *audio.Mixer // optional; sounds are skipped when nil
	DropBalls  int          // balls to drop at startup
}

// New builds the demo scene.
func New(opts Options) (*Game, error) {
	g := &Game{
		world:        scene.NewWorld(),
		camera:       newCameraRig(),
		sunIntensity: 1,
		ambientLevel: 1,
		timeScale:    1,
		rng:          rand.New(rand.NewPCG(1, 2)),
		sound:        opts.Audio,
		volume:       0.8,
		dropSound:    audio.Blip(120*time.Millisecond, 520, 880, 0.5),
		clearSound:   audio.Blip(250*time.Millisecond, 600, 180, 0.6),
		bounceSound:  audio.Blip(60*time.Millisecond, 260, 140, 0.8),
	}
	w := g.world
	modelPath := opts.Model

	if opts.ScriptsDir != "" {
		host, err := script.New(opts.ScriptsDir, logf)
		if err != nil {
			// Keep going: the host retries when the files change.
			logf("script: initial load failed: %v", err)
		} else {
			logf("script: loaded %s: %s", opts.ScriptsDir, strings.Join(host.Names(), ", "))
		}
		g.scripts = host
	}

	groundMesh, err := render.CreateMesh(scaledUVs(geom.Plane(1), 7))
	if err != nil {
		return nil, err
	}
	groundTex, err := render.CreateTexture(checker(256, 2,
		color.NRGBA{0x8a, 0x93, 0x9e, 255}, color.NRGBA{0x6b, 0x72, 0x80, 255}), true)
	if err != nil {
		return nil, err
	}
	ground := w.Spawn("ground", scene.ID{})
	ground.Transform.Scale = mathx.Vec3{14, 1, 14}
	ground.Renderable = &scene.Renderable{Mesh: groundMesh, Texture: groundTex, Color: [4]float32{1, 1, 1, 1}}
	if err := g.setupPhysics(7); err != nil {
		return nil, err
	}

	centre := w.Spawn("centre", scene.ID{}).AddBehaviour(spin(0.3))
	if modelPath != "" {
		if err := g.addModel(modelPath, centre.ID()); err != nil {
			return nil, err
		}
	} else {
		sphere, err := render.CreateMesh(geom.Sphere(0.75, 48, 24))
		if err != nil {
			return nil, err
		}
		centre.Transform.Position = mathx.Vec3{0, 1, 0}
		centre.Renderable = &scene.Renderable{Mesh: sphere, Color: mathx.Hex(0xe5e7eb)}
		centre.AddBehaviour(g.script("Bob"))
		g.attachKinematic(centre, physics.Sphere, mathx.Vec3{0.75})
	}

	cube, err := render.CreateMesh(geom.Cube(1))
	if err != nil {
		return nil, err
	}
	panelTex, err := render.CreateTexture(panel(128, 12), true)
	if err != nil {
		return nil, err
	}
	if err := g.addWalls(cube, panelTex, 7); err != nil {
		return nil, err
	}

	// The cubes hang off a slowly turning ring, and each also spins on its own.
	ring := w.Spawn("ring", scene.ID{}).AddBehaviour(spin(-0.1)).AddBehaviour(g.script("Pulse"))
	palette := []uint32{0xe05252, 0xf0923a, 0xe8cf45, 0x4cbf6b, 0x4a90e2, 0x9b6ce0}
	for i, hex := range palette {
		angle := float64(i) / float64(len(palette)) * 2 * math.Pi
		c := w.Spawn(fmt.Sprintf("cube%d", i), ring.ID()).AddBehaviour(spin(0.8 + 0.2*float32(i)))
		c.Transform.Position = mathx.Vec3{float32(math.Cos(angle)) * 3.5, 0.5, float32(math.Sin(angle)) * 3.5}
		c.Renderable = &scene.Renderable{Mesh: cube, Texture: panelTex, Color: mathx.Hex(hex)}
		g.attachKinematic(c, physics.Box, mathx.Vec3{0.5, 0.5, 0.5})
	}

	w.UpdateTransforms()
	for i := 0; i < opts.DropBalls; i++ {
		g.dropBall()
	}
	return g, nil
}

// addModel loads a glTF file, fits it into a 2-unit box and adds one child
// entity per material under parent.
func (g *Game) addModel(path string, parent scene.ID) error {
	model, err := asset.LoadGLTF(path)
	if err != nil {
		return err
	}
	model.FitToSize(2)

	textures := make([]render.Texture, len(model.Images))
	for i, img := range model.Images {
		if textures[i], err = render.CreateTexture(img, true); err != nil {
			return fmt.Errorf("%s image %d: %w", path, i, err)
		}
	}
	triangles := 0
	for i, part := range model.Parts {
		mesh, err := render.CreateMesh(part.Mesh)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		mat := model.Materials[part.Material]
		r := &scene.Renderable{Mesh: mesh, Color: mat.BaseColor}
		if mat.BaseColorImage >= 0 {
			r.Texture = textures[mat.BaseColorImage]
		}
		g.world.Spawn(fmt.Sprintf("part%d", i), parent).Renderable = r
		triangles += len(part.Mesh.Indices) / 3
	}
	fmt.Printf("loaded %s: %d parts, %d triangles, %d textures\n", path, len(model.Parts), triangles, len(textures))
	return nil
}

// Update advances the game. mouseFree is false while the debug UI has the mouse.
func (g *Game) Update(dt float32, in *input.State, mouseFree bool) {
	if g.scripts != nil {
		g.scripts.Poll() // errors are logged by the host
	}
	g.camera.update(dt, in, mouseFree)
	g.world.Update(dt * g.timeScale)
	g.stepPhysics(dt * g.timeScale)
}

// DebugUI describes the game's debug window.
func (g *Game) DebugUI(b *ui.Builder, s Stats) {
	b.Window("vkgame", 12, 12)
	b.Text("%.0f fps  %.2f ms", s.FPS, s.FrameMS)
	b.Text("%d entities, %d draws", g.world.Len(), s.Draws)
	mode := "orbit (Tab: fly)"
	if g.camera.fly {
		mode = "fly (Tab: orbit)"
	}
	b.Text("camera: %s", mode)
	if g.scripts != nil {
		b.Text("scripts: v%d %s", g.scripts.Version(), strings.Join(g.scripts.Names(), ", "))
	}
	b.Separator()
	b.Slider("sun", &g.sunIntensity, 0, 3)
	b.Slider("ambient", &g.ambientLevel, 0, 3)
	b.Slider("time scale", &g.timeScale, 0, 4)
	b.Checkbox("auto orbit", &g.camera.autoSpin)
	if g.sound != nil {
		b.Slider("volume", &g.volume, 0, 1)
		g.sound.SetVolume(g.volume)
	}
	b.Separator()
	b.Text("physics: %d bodies, %d balls", len(g.phys.Bodies()), len(g.balls))
	if b.Button("drop ball") {
		g.dropBall()
		g.playAtVolume(g.dropSound, mathx.Vec3{0, 6, 0}, 1)
	}
	if b.Button("drop 20") {
		for i := 0; i < 20; i++ {
			g.dropBall()
		}
		g.playAtVolume(g.dropSound, mathx.Vec3{0, 6, 0}, 1)
	}
	if b.Button("clear balls") && len(g.balls) > 0 {
		g.clearBalls()
		g.playAtVolume(g.clearSound, mathx.Vec3{}, 1)
	}
	b.Text("F1: hide this window")
	b.End()
}

// playAtVolume plays a sound panned and attenuated by where pos is relative
// to the camera.
func (g *Game) playAtVolume(s *audio.Sound, pos mathx.Vec3, volume float32) {
	if g.sound == nil {
		return
	}
	view, _ := g.camera.view()
	p := view.TransformPoint(pos) // camera space: +X right, -Z forward
	dist := p.Len()
	pan := float32(0)
	if dist > 0 {
		pan = p[0] / dist
	}
	g.sound.Play(s, volume/(1+0.1*dist), pan)
}

// script returns the named script behaviour, or a no-op without scripts.
func (g *Game) script(name string) scene.Behaviour {
	if g.scripts == nil {
		return func(*scene.World, *scene.Entity, float32) {}
	}
	return g.scripts.Behaviour(name)
}

func logf(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

// CursorLocked reports whether the mouse should be captured (mouse-look).
func (g *Game) CursorLocked() bool { return g.camera.locked }

// Render returns this frame's scene parameters and draw list. The draw list is
// appended to out[:0], so the caller can reuse one slice every frame.
func (g *Game) Render(aspect float32, out []render.DrawCmd) (render.FrameParams, []render.DrawCmd) {
	view, eye := g.camera.view()
	params := render.FrameParams{
		ViewProj:     g.camera.lens.Projection(aspect).Mul(view),
		CameraPos:    eye,
		SunDirection: mathx.Vec3{0.4, 1, 0.3},
		SunColor:     mathx.Vec3{1, 0.95, 0.85}.Scale(g.sunIntensity),
		Ambient:      mathx.Vec3{0.18, 0.2, 0.25}.Scale(g.ambientLevel),
		Clear:        mathx.Hex(0x9cc3e6),
	}
	return params, g.world.AppendDraws(out[:0])
}

// spin rotates an entity around world up at a constant rate (radians/second).
func spin(speed float32) scene.Behaviour {
	return func(_ *scene.World, e *scene.Entity, dt float32) {
		e.Transform.Rotation = mathx.AxisAngle(worldUp, speed*dt).Mul(e.Transform.Rotation).Normalize()
	}
}

// scaledUVs multiplies a mesh's texture coordinates, so textures repeat.
func scaledUVs(m geom.MeshData, repeat float32) geom.MeshData {
	for i := range m.Vertices {
		m.Vertices[i].UV[0] *= repeat
		m.Vertices[i].UV[1] *= repeat
	}
	return m
}
