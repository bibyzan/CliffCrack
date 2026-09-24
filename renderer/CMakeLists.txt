cmake_minimum_required(VERSION 3.25)
project(cliffcrack_renderer LANGUAGES C CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

if(NOT WIN32)
    message(FATAL_ERROR "Only the Win32 surface is implemented so far.")
endif()

# ---------------------------------------------------------------------------
# Dependencies (fetched at configure time, pinned to matching Vulkan SDK tags).
# The Vulkan loader is NOT linked: volk loads vulkan-1.dll (from the GPU driver)
# at runtime, so any compiler works without the SDK's import library.
# ---------------------------------------------------------------------------
include(FetchContent)

FetchContent_Declare(vulkan_headers
    GIT_REPOSITORY https://github.com/KhronosGroup/Vulkan-Headers.git
    GIT_TAG        vulkan-sdk-1.4.357.0
    GIT_SHALLOW    TRUE
    SYSTEM)
FetchContent_Declare(volk
    GIT_REPOSITORY https://github.com/zeux/volk.git
    GIT_TAG        vulkan-sdk-1.4.357.0
    GIT_SHALLOW    TRUE
    SYSTEM)
FetchContent_Declare(vk_bootstrap
    GIT_REPOSITORY https://github.com/charles-lunarg/vk-bootstrap.git
    GIT_TAG        v1.4.357
    GIT_SHALLOW    TRUE
    SYSTEM)
FetchContent_Declare(vma
    GIT_REPOSITORY https://github.com/GPUOpen-LibrariesAndSDKs/VulkanMemoryAllocator.git
    GIT_TAG        v3.3.0
    GIT_SHALLOW    TRUE
    SYSTEM)
FetchContent_Declare(imgui
    GIT_REPOSITORY https://github.com/ocornut/imgui.git
    GIT_TAG        v1.92.9
    GIT_SHALLOW    TRUE
    SYSTEM)

FetchContent_MakeAvailable(vulkan_headers)

# Point volk at the fetched headers so every library agrees on one header version.
set(VULKAN_HEADERS_INSTALL_DIR "${vulkan_headers_SOURCE_DIR}" CACHE PATH "" FORCE)
set(VOLK_STATIC_DEFINES "VK_USE_PLATFORM_WIN32_KHR" CACHE STRING "" FORCE)
set(VK_BOOTSTRAP_TEST OFF CACHE BOOL "" FORCE)

FetchContent_MakeAvailable(volk vk_bootstrap vma imgui)

# Dear ImGui ships no CMake project. Its Vulkan backend gets its function
# pointers from volk at runtime (ImGui_ImplVulkan_LoadFunctions), so it is
# compiled without prototypes and never links the loader.
add_library(imgui STATIC
    ${imgui_SOURCE_DIR}/imgui.cpp
    ${imgui_SOURCE_DIR}/imgui_draw.cpp
    ${imgui_SOURCE_DIR}/imgui_tables.cpp
    ${imgui_SOURCE_DIR}/imgui_widgets.cpp
    ${imgui_SOURCE_DIR}/backends/imgui_impl_vulkan.cpp)
target_include_directories(imgui SYSTEM PUBLIC ${imgui_SOURCE_DIR} ${imgui_SOURCE_DIR}/backends)
target_compile_definitions(imgui PUBLIC IMGUI_IMPL_VULKAN_NO_PROTOTYPES IMGUI_DISABLE_OBSOLETE_FUNCTIONS)
target_link_libraries(imgui PUBLIC Vulkan::Headers)

# ---------------------------------------------------------------------------
# Renderer DLL
# ---------------------------------------------------------------------------
add_library(renderer SHARED
    src/renderer.cpp
    src/ui.cpp
    src/ui.h
    src/vma_impl.cpp
    src/platform_win32.cpp
    src/vk_common.h
    include/renderer.h)

target_include_directories(renderer PUBLIC include)
target_compile_definitions(renderer PRIVATE
    RENDERER_BUILD
    VMA_STATIC_VULKAN_FUNCTIONS=0
    VMA_DYNAMIC_VULKAN_FUNCTIONS=1)
target_link_libraries(renderer PRIVATE
    volk::volk
    vk-bootstrap::vk-bootstrap
    GPUOpen::VulkanMemoryAllocator
    imgui)

# Output renderer.dll (+ import lib) into build/bin with no "lib" prefix, so the
# Go side can link with -lrenderer and the exe can sit next to the DLL.
set(VKGAME_BIN_DIR "${CMAKE_BINARY_DIR}/bin")
set_target_properties(renderer PROPERTIES
    PREFIX ""
    IMPORT_PREFIX ""
    RUNTIME_OUTPUT_DIRECTORY "${VKGAME_BIN_DIR}"
    LIBRARY_OUTPUT_DIRECTORY "${VKGAME_BIN_DIR}"
    ARCHIVE_OUTPUT_DIRECTORY "${VKGAME_BIN_DIR}")
foreach(cfg IN LISTS CMAKE_CONFIGURATION_TYPES)
    string(TOUPPER "${cfg}" CFG)
    set_target_properties(renderer PROPERTIES
        RUNTIME_OUTPUT_DIRECTORY_${CFG} "${VKGAME_BIN_DIR}"
        LIBRARY_OUTPUT_DIRECTORY_${CFG} "${VKGAME_BIN_DIR}"
        ARCHIVE_OUTPUT_DIRECTORY_${CFG} "${VKGAME_BIN_DIR}")
endforeach()

if(MSVC)
    target_compile_options(renderer PRIVATE /W4)
else()
    # Vulkan structs are initialised as `T info{VK_STRUCTURE_TYPE_...}` on purpose.
    target_compile_options(renderer PRIVATE -Wall -Wextra -Wno-missing-field-initializers)
endif()

if(MINGW)
    # Bake libgcc/libstdc++/winpthread into the DLL so no MinGW DLLs ship with it.
    target_link_options(renderer PRIVATE -static)
endif()

# ---------------------------------------------------------------------------
# Shaders: GLSL -> SPIR-V with glslc (ships with the Vulkan SDK)
# ---------------------------------------------------------------------------
find_program(GLSLC glslc HINTS "$ENV{VULKAN_SDK}/Bin" "$ENV{VULKAN_SDK}/bin" REQUIRED)

set(SHADER_SOURCES
    shaders/mesh.vert
    shaders/mesh.frag)

set(SHADER_OUT_DIR "${VKGAME_BIN_DIR}/shaders")
set(SPIRV_OUTPUTS "")
foreach(src IN LISTS SHADER_SOURCES)
    get_filename_component(name "${src}" NAME)
    set(out "${SHADER_OUT_DIR}/${name}.spv")
    add_custom_command(
        OUTPUT  "${out}"
        COMMAND "${CMAKE_COMMAND}" -E make_directory "${SHADER_OUT_DIR}"
        COMMAND "${GLSLC}" --target-env=vulkan1.3 -O -o "${out}" "${CMAKE_CURRENT_SOURCE_DIR}/${src}"
        DEPENDS "${CMAKE_CURRENT_SOURCE_DIR}/${src}"
        COMMENT "glslc ${name}"
        VERBATIM)
    list(APPEND SPIRV_OUTPUTS "${out}")
endforeach()

add_custom_target(shaders ALL DEPENDS ${SPIRV_OUTPUTS})
add_dependencies(renderer shaders)
