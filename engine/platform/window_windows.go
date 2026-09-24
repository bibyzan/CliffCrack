package platform

import "unsafe"

// NativeHandle returns the HWND the renderer creates its Vulkan surface from.
func (w *Window) NativeHandle() unsafe.Pointer {
	return unsafe.Pointer(w.win.GetWin32Window())
}
