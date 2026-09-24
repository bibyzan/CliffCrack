// Package script runs hot-reloadable gameplay scripts: plain Go source files
// interpreted with yaegi. Scripts import engine packages (scene, mathx, gfx)
// under their normal import paths and export behaviours as top-level funcs:
//
//	package scripts
//
//	import "CliffCrack/engine/scene"
//
//	func Bob(w *scene.World, e *scene.Entity, dt float32) { ... }
//
// Game code attaches them by name with Host.Behaviour("Bob"). When a script
// file changes, Poll reloads the whole directory into a fresh interpreter and
// every attached behaviour switches to the new code on its next call. If the
// new code fails to compile, the previous version keeps running.
package script

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"

	"CliffCrack/engine/scene"
	"CliffCrack/engine/script/symbols"
)

// behaviourFunc is the signature scripts export.
type behaviourFunc = func(*scene.World, *scene.Entity, float32)

// Host owns the loaded scripts of one directory.
type Host struct {
	dir      string
	logf     func(format string, args ...any)
	funcs    map[string]behaviourFunc
	failed   map[string]bool // panicked at runtime; skipped until the next reload
	stamp    string          // file names + sizes + mod times at the last load attempt
	lastPoll time.Time
	version  int
}

// PollInterval is the minimum time between checks for changed files.
const PollInterval = 250 * time.Millisecond

// New loads every *.go file in dir. logf receives reload and error messages
// (nil discards them). An error means the initial load failed; the host is
// still returned and will retry when the files change.
func New(dir string, logf func(format string, args ...any)) (*Host, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	h := &Host{dir: dir, logf: logf, funcs: map[string]behaviourFunc{}, failed: map[string]bool{}}
	h.stamp, _ = h.dirStamp()
	return h, h.load()
}

// Version counts successful loads (1 after a clean start).
func (h *Host) Version() int { return h.version }

// Names lists the loaded behaviours, sorted.
func (h *Host) Names() []string {
	names := make([]string, 0, len(h.funcs))
	for n := range h.funcs {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Poll reloads the scripts if any file was added, removed or modified since
// the last load attempt. It checks at most every PollInterval. It returns
// whether a reload succeeded, and the error of a failed one.
func (h *Host) Poll() (bool, error) {
	if time.Since(h.lastPoll) < PollInterval {
		return false, nil
	}
	h.lastPoll = time.Now()
	stamp, err := h.dirStamp()
	if err != nil || stamp == h.stamp {
		return false, err
	}
	h.stamp = stamp
	if err := h.load(); err != nil {
		h.logf("script: reload failed, keeping version %d: %v", h.version, err)
		return false, err
	}
	h.logf("script: reloaded version %d: %s", h.version, strings.Join(h.Names(), ", "))
	return true, nil
}

// Behaviour returns a scene.Behaviour that calls the named script function,
// always using the most recently loaded code. Unknown names do nothing (the
// function may appear in a later reload). A panic disables that behaviour
// until the next reload instead of crashing the game.
func (h *Host) Behaviour(name string) scene.Behaviour {
	return func(w *scene.World, e *scene.Entity, dt float32) {
		f := h.funcs[name]
		if f == nil || h.failed[name] {
			return
		}
		defer func() {
			if r := recover(); r != nil {
				h.failed[name] = true
				h.logf("script: %s panicked on %q, disabled until reload: %v", name, e.Name, r)
			}
		}()
		f(w, e, dt)
	}
}

// load interprets every script file in a fresh interpreter and swaps in the
// exported behaviours only if everything succeeded.
func (h *Host) load() error {
	files, err := filepath.Glob(filepath.Join(h.dir, "*.go"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return fmt.Errorf("no .go files in %s", h.dir)
	}

	// Parse first: cheap syntax errors with positions, and the list of exported funcs.
	fset := token.NewFileSet()
	pkg := ""
	var exported []string
	sources := make([]string, len(files))
	for i, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return err
		}
		if pkg == "" {
			pkg = f.Name.Name
		} else if f.Name.Name != pkg {
			return fmt.Errorf("%s: package %s, expected %s", path, f.Name.Name, pkg)
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.IsExported() {
				exported = append(exported, fn.Name.Name)
			}
		}
		sources[i] = string(src)
	}

	i := interp.New(interp.Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		return err
	}
	if err := i.Use(symbols.Symbols); err != nil {
		return err
	}
	for n, src := range sources {
		if _, err := safeEval(i, src); err != nil {
			return fmt.Errorf("%s: %w", files[n], err)
		}
	}

	funcs := map[string]behaviourFunc{}
	for _, name := range exported {
		v, err := safeEval(i, pkg+"."+name)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		f, ok := v.Interface().(behaviourFunc)
		if !ok {
			continue // exported helper with a different signature
		}
		funcs[name] = f
	}
	h.funcs = funcs
	h.failed = map[string]bool{}
	h.version++
	return nil
}

// safeEval turns interpreter panics (which yaegi can raise on odd input) into errors.
func safeEval(i *interp.Interpreter, src string) (v reflect.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("interpreter panic: %v", r)
		}
	}()
	return i.Eval(src)
}

// dirStamp summarises the script files so changes can be detected cheaply.
func (h *Host) dirStamp() (string, error) {
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue // deleted between ReadDir and Info
			}
			return "", err
		}
		fmt.Fprintf(&b, "%s|%d|%d;", e.Name(), info.Size(), info.ModTime().UnixNano())
	}
	return b.String(), nil
}
