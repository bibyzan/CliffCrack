// Common Vulkan include. volk must come first: it defines VK_NO_PROTOTYPES and
// provides the global vk* function pointers that everything else uses.
#pragma once

#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#ifndef NOMINMAX
#define NOMINMAX
#endif

#include <volk.h>
#include <vk_mem_alloc.h>
