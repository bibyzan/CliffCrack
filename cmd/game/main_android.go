//go:build android

package main

import (
	"fmt"
	"os"

	"CliffCrack/engine/platform"
)

// No validation layers ship on phones and handhelds.
const validationDefault = false

// Android loads this package as a shared library from NativeActivity, so main
// never runs; the activity's game thread calls run through the platform layer.
func init() {
	platform.SetMain(func() {
		if err := run(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		platform.Exit()
	})
}
