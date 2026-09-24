// Package symbols exposes engine packages to interpreted scripts (yaegi).
//
// The vkgame-engine-*.go files are generated; after changing the exported API
// of scene, mathx or gfx, regenerate them from this directory with:
//
//	go generate
//
// The generated files carry a build tag for the Go version that produced them
// (that version or newer).
package symbols

import "reflect"

//go:generate go run github.com/traefik/yaegi/cmd/yaegi extract -name symbols vkgame/engine/scene vkgame/engine/mathx vkgame/engine/gfx

// Symbols is filled in by the generated files' init functions.
var Symbols = map[string]map[string]reflect.Value{}
