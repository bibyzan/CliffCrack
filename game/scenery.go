package game

import (
	"image/color"
	"math"
	"math/rand/v2"

	"CliffCrack/engine/geom"
	"CliffCrack/engine/gfx"
	"CliffCrack/engine/mathx"
	"CliffCrack/engine/noise"
	"CliffCrack/engine/render"
)

// Run mode's look: faceted, low-poly snow and rock lit by a low golden sun,
// cool blue shade, and ranges fading into a peach haze with distance
// (aerial perspective). All of it is flat-shaded procedural geometry plus a
// procedural sky; the only texture is the ball's.
var (
	snowColor  = mathx.SRGB(0.94, 0.96, 1.0, 1)
	rockColor  = mathx.SRGB(0.62, 0.63, 0.70, 1)
	crownColor = mathx.SRGB(0.17, 0.36, 0.31, 1)
	trunkColor = mathx.SRGB(0.36, 0.25, 0.19, 1)
	skyZenith  = mathx.SRGB(0.33, 0.50, 0.80, 1)
	hazeColor  = srgb3(0.98, 0.80, 0.71)
	sunColor   = srgb3(1.0, 0.86, 0.68)
	shadeColor = srgb3(0.50, 0.58, 0.78)       // ambient: sky light in the shade
	sunDir     = mathx.Vec3{0.62, 0.26, -0.74} // low, ahead and to the right: long light across the facets
)

const (
	skyRadius  = 2000
	farPlane   = 2600
	fogDensity = 0.0016
)

// backdropRange is one layer of the mountain ranges on the horizon. The
// layers keep a fixed distance ahead of the camera, so they sit at
// "infinity" while the course streams past underneath. Their feet go no
// higher than the course that far ahead: down a steep face the ground falls
// away faster than the camera, and sky would show beneath them.
type backdropRange struct {
	mesh     render.Mesh
	distance float32 // ahead of the camera
	drop     float32 // base below the camera
	color    [4]float32
}

// scenery holds the GPU assets Run mode draws with. Built once, shared by
// every run.
type scenery struct {
	sky     render.Mesh
	ranges  []backdropRange
	rocks   []render.Mesh
	crowns  []render.Mesh
	trunk   render.Mesh
	ball    render.Mesh
	ballTex render.Texture
	shadow  render.Mesh
	chip    render.Mesh // a piece of shattered ball
	snow    render.Mesh // a rolling snowball
	arrow   render.Mesh // the boost pickup: a cone pointing down the run
	orb     render.Mesh // the shield pickup, halos, and the shield around the ball
}

func newScenery() (*scenery, error) {
	sc := &scenery{}
	var err error
	mesh := func(m geom.MeshData) render.Mesh {
		if err != nil {
			return 0
		}
		var h render.Mesh
		h, err = render.CreateMesh(m)
		return h
	}

	dome := geom.Sphere(1, 32, 16)
	dome.FlipWinding()
	sc.sky = mesh(dome)

	field := noise.New(0x5eed)
	for i, layer := range []struct {
		distance, drop, height float32
		tint                   [4]float32
	}{
		{560, 210, 300, mathx.SRGB(0.80, 0.84, 0.95, 1)},
		{860, 320, 440, mathx.SRGB(0.86, 0.87, 0.97, 1)},
		{1200, 440, 620, mathx.SRGB(0.92, 0.91, 0.98, 1)},
	} {
		sc.ranges = append(sc.ranges, backdropRange{
			mesh:     mesh(rangeMesh(field, float64(i)*31.7, layer.height)),
			distance: layer.distance,
			drop:     layer.drop,
			color:    layer.tint,
		})
	}

	rng := rand.New(rand.NewPCG(3, 5))
	for range 5 {
		sc.rocks = append(sc.rocks, mesh(rockMesh(rng)))
	}
	for tiers := 2; tiers <= 4; tiers++ {
		sc.crowns = append(sc.crowns, mesh(crownMesh(tiers)))
	}
	sc.trunk = mesh(geom.Cone(0.22, 1.6, 6))
	sc.ball = mesh(geom.Sphere(1, 32, 16))
	sc.shadow = mesh(geom.Disc(1, 24, false))
	sc.chip = mesh(geom.Icosphere(1, 0))
	sc.snow = mesh(rockMesh(rand.New(rand.NewPCG(9, 9)))) // lumpy, like packed snow
	sc.arrow = mesh(geom.Cone(0.55, 1.3, 12))
	sc.orb = mesh(geom.Icosphere(1, 1))
	if err != nil {
		return nil, err
	}
	sc.ballTex, err = render.CreateTexture(checker(128, 4,
		color.NRGBA{0xf5, 0xf5, 0xf5, 255}, color.NRGBA{0xe0, 0x6a, 0x1b, 255}), true)
	if err != nil {
		return nil, err
	}
	return sc, nil
}

// rangeMesh is a strip of mountains 4.8 km wide and 300 m deep along -Z from
// the origin, with its foot on y = 0 and peaks up to height: ridged noise
// for the skyline, lifted into a ridge between the front and back edges and
// jittered so every facet catches the light differently. A skirt hangs
// rangeSkirt metres below its front edge, so nothing shows under it.
func rangeMesh(field *noise.Field, offset float64, height float32) geom.MeshData {
	const cols, rows = 161, 9
	const width, depth = 4800, 300
	return geom.Grid(cols, rows, func(i, j int) mathx.Vec3 {
		x := -width/2 + float32(i)*width/(cols-1)
		if j == 0 {
			return mathx.Vec3{x, -rangeSkirt, 0}
		}
		v := float32(j-1) / (rows - 2)
		z := -v * depth
		skyline := 0.3 + 0.7*float32(field.Ridged(float64(x)/520+offset, offset, 4, 0.55))
		ridge := float32(math.Pow(math.Sin(math.Pi*float64(v)), 0.6))
		jitter := float32(field.At(float64(x)/37+offset, float64(j)*1.7)) * 0.08
		return mathx.Vec3{x, height * skyline * (ridge + jitter*ridge), z}
	})
}

// rockMesh is a lumpy boulder: a coarse icosphere with its corners pushed
// in and out.
func rockMesh(rng *rand.Rand) geom.MeshData {
	m := geom.Icosphere(1, 1)
	for i := range m.Vertices {
		p := m.Vertices[i].Position
		m.Vertices[i].Position = p.Scale(0.8 + 0.4*rng.Float32())
	}
	m.ComputeNormals()
	return m
}

// crownMesh is a pine's foliage: stacked cones, narrowing upwards.
func crownMesh(tiers int) geom.MeshData {
	var m geom.MeshData
	y := float32(0.7)
	for t := 0; t < tiers; t++ {
		f := 1 - float32(t)/float32(tiers+1)
		cone := geom.Cone(1.35*f, 1.9*f+0.4, 7)
		for i := range cone.Vertices {
			cone.Vertices[i].Position[1] += y
		}
		m.Append(cone)
		y += 1.0 * f
	}
	return m
}

func srgb3(r, g, b float32) mathx.Vec3 {
	c := mathx.SRGB(r, g, b, 1)
	return mathx.Vec3{c[0], c[1], c[2]}
}

// runFrameParams is the Run mode's camera, sun, ambient and haze.
func runFrameParams(viewProj mathx.Mat4, eye mathx.Vec3, sun float32) render.FrameParams {
	return render.FrameParams{
		ViewProj:     viewProj,
		CameraPos:    eye,
		SunDirection: sunDir,
		SunColor:     sunColor.Scale(0.9 * sun),
		Ambient:      shadeColor.Scale(0.42),
		FogColor:     hazeColor,
		FogDensity:   fogDensity,
		Clear:        [4]float32{hazeColor[0], hazeColor[1], hazeColor[2], 1},
	}
}

// rangeSkirt is how far below its foot each range hangs a curtain.
const rangeSkirt = 700

// appendBackdrop draws the sky and the distant ranges around the camera.
// ground (nil for none) is the course's height the given distance ahead.
func (sc *scenery) appendBackdrop(out []render.DrawCmd, eye mathx.Vec3, ground func(ahead float32) float32) []render.DrawCmd {
	out = append(out, render.DrawCmd{
		Model: mathx.Translate(eye[0], eye[1], eye[2]).Mul(mathx.Scale(skyRadius, skyRadius, skyRadius)),
		Color: skyZenith,
		Flags: gfx.DrawSky,
		Mesh:  sc.sky,
	})
	for _, r := range sc.ranges {
		foot := eye[1] - r.drop
		if ground != nil {
			foot = min(foot, ground(r.distance)-60)
		}
		out = append(out, render.DrawCmd{
			Model: mathx.Translate(eye[0], foot, eye[2]-r.distance),
			Color: r.color,
			Flags: gfx.DrawFlat | gfx.DrawSnow,
			Mesh:  r.mesh,
		})
	}
	return out
}
