//go:build ios

package main

import (
	"fmt"
	"os"

	"CliffCrack/engine/platform"
)

// No validation layers ship on phones.
const validationDefault = false

// iOS links this package into the app as a static library, so main never
// runs; UIKit starts the app and the platform layer calls run on the game's
// own thread.
func init() {
	platform.SetMain(func() {
		if err := run(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		platform.Exit()
	})
}
