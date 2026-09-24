// Win32 bits kept out of renderer.cpp so it doesn't pull in <windows.h>.
#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>

void* platform_module_handle() {
    return GetModuleHandleW(nullptr);
}
