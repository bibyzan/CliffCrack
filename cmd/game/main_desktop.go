//go:build !android && !ios

package main

// Validation layers come with the Vulkan SDK on development machines.
const validationDefault = true
