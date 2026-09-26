// Vulkan 1.3 renderer: dynamic rendering + synchronization2, no render passes.
//
// Descriptor model:
//   set 0  per-frame uniform buffer (camera, sun) — one set per frame in flight
//   set 1  bindless texture table: sampler2D textures[kMaxTextures], indexed by
//          RDrawCmd::texture through push constants. Slot 0 is a white texture.
#include "vk_common.h"

#include <VkBootstrap.h>

#include "renderer.h"
#include "ui.h"

#include <algorithm>
#include <cstddef>
#include <cstdio>
#include <cstring>
#include <fstream>
#include <string>
#include <vector>

// platform_win32.cpp / platform_android.cpp / platform_ios.cpp
VkResult platform_load_vulkan(); // points volk at the Vulkan implementation
bool platform_create_surface(VkInstance instance, void* native_window, VkSurfaceKHR* surface);
void platform_log(const char* message);

namespace {

constexpr uint32_t kFramesInFlight = 2;
constexpr VkFormat kDepthFormat = VK_FORMAT_D32_SFLOAT;
constexpr uint32_t kMaxTextures = 1024; // must match MAX_TEXTURES in mesh.frag

// Push constants are the leading fields of RDrawCmd: model, color, texture, flags.
constexpr uint32_t kPushConstantSize = offsetof(RDrawCmd, mesh);
static_assert(kPushConstantSize <= 128, "push constants must fit the 128-byte guaranteed minimum");
static_assert(offsetof(RDrawCmd, texture) == 80 && offsetof(RDrawCmd, flags) == 84,
              "RDrawCmd layout must match the shader push block");
static_assert(sizeof(RVertex) == 32, "RVertex layout changed; update the pipeline vertex input");

// The per-frame uniform buffer is RFrameParams minus the trailing clear colour
// (std140: mat4 + 5 x vec4).
constexpr VkDeviceSize kFrameUniformSize = offsetof(RFrameParams, clear_color);
static_assert(kFrameUniformSize == 144, "RFrameParams layout must match the shader Frame block");

struct Buffer {
    VkBuffer      buffer = VK_NULL_HANDLE;
    VmaAllocation allocation = VK_NULL_HANDLE;
};

struct Image {
    VkImage       image = VK_NULL_HANDLE;
    VmaAllocation allocation = VK_NULL_HANDLE;
    VkImageView   view = VK_NULL_HANDLE;
};

struct FrameData {
    VkCommandPool   pool = VK_NULL_HANDLE;
    VkCommandBuffer cmd = VK_NULL_HANDLE;
    VkSemaphore     image_acquired = VK_NULL_HANDLE;
    VkFence         in_flight = VK_NULL_HANDLE;
    Buffer          uniforms;
    void*           uniforms_mapped = nullptr;
    VkDescriptorSet frame_set = VK_NULL_HANDLE;
};

struct Mesh {
    Buffer   vertices;
    Buffer   indices;
    uint32_t index_count = 0; // 0 = free slot
};

struct Texture {
    Image image; // image.image == VK_NULL_HANDLE marks a free slot
};

struct Renderer {
    vkb::Instance instance;
    VkSurfaceKHR  surface = VK_NULL_HANDLE;
    vkb::Device   device;
    VkDevice      dev = VK_NULL_HANDLE;
    VkQueue       queue = VK_NULL_HANDLE;
    uint32_t      queue_family = 0;
    VmaAllocator  allocator = VK_NULL_HANDLE;
    float         max_anisotropy = 1.0f;

    vkb::Swapchain           swapchain;
    std::vector<VkImage>     images;
    std::vector<VkImageView> views;
    std::vector<VkSemaphore> render_done; // one per swapchain image
    bool                     swapchain_dirty = false;
    Image                    depth;       // shared by all frames; barriers serialize use

    // Blocking uploads (meshes, textures) use their own command buffer + fence.
    VkCommandPool   upload_pool = VK_NULL_HANDLE;
    VkCommandBuffer upload_cmd = VK_NULL_HANDLE;
    VkFence         upload_fence = VK_NULL_HANDLE;

    VkDescriptorSetLayout frame_set_layout = VK_NULL_HANDLE;
    VkDescriptorSetLayout texture_set_layout = VK_NULL_HANDLE;
    VkDescriptorPool      frame_pool = VK_NULL_HANDLE;
    VkDescriptorPool      texture_pool = VK_NULL_HANDLE;
    VkDescriptorSet       texture_set = VK_NULL_HANDLE;
    VkSampler             sampler = VK_NULL_HANDLE;

    std::vector<Mesh>     meshes;     // RMesh handle = index + 1
    std::vector<uint32_t> free_meshes;
    // Destroyed meshes whose buffers may still be read by frames in flight.
    // Each is freed once every frame numbered below `last_use` has finished.
    struct RetiredMesh {
        Mesh     mesh;
        uint64_t last_use;
    };
    std::vector<RetiredMesh> retired_meshes;
    std::vector<Texture>  textures;   // RTexture handle = index; 0 = white
    std::vector<uint32_t> free_textures;

    std::string      shader_dir;
    VkFormat         pipeline_format = VK_FORMAT_UNDEFINED;
    VkPipelineLayout pipeline_layout = VK_NULL_HANDLE;
    VkPipeline       pipeline = VK_NULL_HANDLE;

    FrameData frames[kFramesInFlight];
    uint32_t  frame = 0;
    uint64_t  submitted = 0; // frames submitted so far; the next frame's number
    uint32_t  image = 0;
    bool      recording = false;
    bool      scene_state_bound = false; // our pipeline/sets/viewport are bound (UI rebinds its own)
    bool      ui_ready = false;

    uint32_t width = 0;
    uint32_t height = 0;
    bool     vsync = true;
    float    ui_scale = 1.0f;

    // Pre-rotation: the swapchain is created in the panel's native
    // orientation and everything is drawn rotated by this transform, so the
    // compositor doesn't have to rotate every frame (Android).
    VkSurfaceTransformFlagBitsKHR transform = VK_SURFACE_TRANSFORM_IDENTITY_BIT_KHR;

    // Frame capture (r_capture_next_frame / r_read_capture).
    bool         capture_requested = false;
    bool         capture_pending = false; // copy recorded, not yet read
    uint32_t     capture_frame = 0;       // frames[] slot whose fence guards the copy
    uint32_t     capture_width = 0;
    uint32_t     capture_height = 0;
    VkFormat     capture_format = VK_FORMAT_UNDEFINED;
    Buffer       capture_buffer;
    void*        capture_mapped = nullptr;
    VkDeviceSize capture_capacity = 0;
};

Renderer*   g = nullptr;
std::string g_error;

bool fail(std::string msg) {
    g_error = std::move(msg);
    platform_log(("[renderer] " + g_error).c_str());
    return false;
}

bool rotated_sideways(VkSurfaceTransformFlagBitsKHR t) {
    return t == VK_SURFACE_TRANSFORM_ROTATE_90_BIT_KHR || t == VK_SURFACE_TRANSFORM_ROTATE_270_BIT_KHR;
}

// The swapchain extent as the player sees it (see r_display_size).
VkExtent2D display_extent() {
    VkExtent2D e = g->swapchain.extent;
    if (rotated_sideways(g->transform)) std::swap(e.width, e.height);
    return e;
}

// Folds the pre-rotation into a column-major view-projection matrix: clip
// space is turned so the upright image lands correctly in the rotated swapchain.
void pre_rotate(float m[16], VkSurfaceTransformFlagBitsKHR t) {
    float c = 1, s = 0; // rotation about clip-space Z
    switch (t) {
    case VK_SURFACE_TRANSFORM_ROTATE_90_BIT_KHR:  c = 0;  s = 1;  break;
    case VK_SURFACE_TRANSFORM_ROTATE_180_BIT_KHR: c = -1; s = 0;  break;
    case VK_SURFACE_TRANSFORM_ROTATE_270_BIT_KHR: c = 0;  s = -1; break;
    default: return;
    }
    for (int col = 0; col < 4; ++col) {
        float* v = m + col * 4;
        const float x = v[0], y = v[1];
        v[0] = c * x - s * y;
        v[1] = s * x + c * y;
    }
}

#define VK_TRY(expr)                                                                   \
    do {                                                                               \
        VkResult vk_try_result_ = (expr);                                              \
        if (vk_try_result_ != VK_SUCCESS)                                              \
            return fail(std::string(#expr) + " failed: VkResult " +                    \
                        std::to_string(static_cast<int>(vk_try_result_)));              \
    } while (0)

// ---------------------------------------------------------------------------
// Shaders and pipeline
// ---------------------------------------------------------------------------

bool read_spirv(const std::string& path, std::vector<uint32_t>& out) {
    std::ifstream file(path, std::ios::binary | std::ios::ate);
    if (!file) return fail("cannot open shader: " + path);
    const auto size = static_cast<size_t>(file.tellg());
    if (size == 0 || size % 4 != 0) return fail("invalid SPIR-V file: " + path);
    out.resize(size / 4);
    file.seekg(0);
    file.read(reinterpret_cast<char*>(out.data()), static_cast<std::streamsize>(size));
    return true;
}

bool create_shader_module(const std::string& name, VkShaderModule& out) {
    std::vector<uint32_t> code;
    if (!read_spirv(g->shader_dir + "/" + name, code)) return false;
    VkShaderModuleCreateInfo info{VK_STRUCTURE_TYPE_SHADER_MODULE_CREATE_INFO};
    info.codeSize = code.size() * sizeof(uint32_t);
    info.pCode = code.data();
    VK_TRY(vkCreateShaderModule(g->dev, &info, nullptr, &out));
    return true;
}

void destroy_pipeline() {
    if (g->pipeline) vkDestroyPipeline(g->dev, g->pipeline, nullptr);
    g->pipeline = VK_NULL_HANDLE;
    g->pipeline_format = VK_FORMAT_UNDEFINED;
}

bool create_pipeline(VkFormat color_format) {
    destroy_pipeline();

    VkShaderModule vert = VK_NULL_HANDLE, frag = VK_NULL_HANDLE;
    if (!create_shader_module("mesh.vert.spv", vert)) return false;
    if (!create_shader_module("mesh.frag.spv", frag)) {
        vkDestroyShaderModule(g->dev, vert, nullptr);
        return false;
    }

    VkPipelineShaderStageCreateInfo stages[2]{};
    stages[0].sType = VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO;
    stages[0].stage = VK_SHADER_STAGE_VERTEX_BIT;
    stages[0].module = vert;
    stages[0].pName = "main";
    stages[1].sType = VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO;
    stages[1].stage = VK_SHADER_STAGE_FRAGMENT_BIT;
    stages[1].module = frag;
    stages[1].pName = "main";

    VkVertexInputBindingDescription vertex_binding{0, sizeof(RVertex), VK_VERTEX_INPUT_RATE_VERTEX};
    const VkVertexInputAttributeDescription vertex_attributes[] = {
        {0, 0, VK_FORMAT_R32G32B32_SFLOAT, offsetof(RVertex, position)},
        {1, 0, VK_FORMAT_R32G32B32_SFLOAT, offsetof(RVertex, normal)},
        {2, 0, VK_FORMAT_R32G32_SFLOAT, offsetof(RVertex, uv)},
    };
    VkPipelineVertexInputStateCreateInfo vertex_input{VK_STRUCTURE_TYPE_PIPELINE_VERTEX_INPUT_STATE_CREATE_INFO};
    vertex_input.vertexBindingDescriptionCount = 1;
    vertex_input.pVertexBindingDescriptions = &vertex_binding;
    vertex_input.vertexAttributeDescriptionCount = 3;
    vertex_input.pVertexAttributeDescriptions = vertex_attributes;

    VkPipelineInputAssemblyStateCreateInfo input_assembly{VK_STRUCTURE_TYPE_PIPELINE_INPUT_ASSEMBLY_STATE_CREATE_INFO};
    input_assembly.topology = VK_PRIMITIVE_TOPOLOGY_TRIANGLE_LIST;

    VkPipelineViewportStateCreateInfo viewport{VK_STRUCTURE_TYPE_PIPELINE_VIEWPORT_STATE_CREATE_INFO};
    viewport.viewportCount = 1;
    viewport.scissorCount = 1;

    VkPipelineRasterizationStateCreateInfo raster{VK_STRUCTURE_TYPE_PIPELINE_RASTERIZATION_STATE_CREATE_INFO};
    raster.polygonMode = VK_POLYGON_MODE_FILL;
    // glTF winding (CCW = front). mathx projections flip Y, so on-screen winding is preserved.
    raster.cullMode = VK_CULL_MODE_BACK_BIT;
    raster.frontFace = VK_FRONT_FACE_COUNTER_CLOCKWISE;
    raster.lineWidth = 1.0f;

    VkPipelineMultisampleStateCreateInfo multisample{VK_STRUCTURE_TYPE_PIPELINE_MULTISAMPLE_STATE_CREATE_INFO};
    multisample.rasterizationSamples = VK_SAMPLE_COUNT_1_BIT;

    VkPipelineColorBlendAttachmentState blend_attachment{};
    blend_attachment.blendEnable = VK_TRUE;
    blend_attachment.srcColorBlendFactor = VK_BLEND_FACTOR_SRC_ALPHA;
    blend_attachment.dstColorBlendFactor = VK_BLEND_FACTOR_ONE_MINUS_SRC_ALPHA;
    blend_attachment.colorBlendOp = VK_BLEND_OP_ADD;
    blend_attachment.srcAlphaBlendFactor = VK_BLEND_FACTOR_ONE;
    blend_attachment.dstAlphaBlendFactor = VK_BLEND_FACTOR_ONE_MINUS_SRC_ALPHA;
    blend_attachment.alphaBlendOp = VK_BLEND_OP_ADD;
    blend_attachment.colorWriteMask = VK_COLOR_COMPONENT_R_BIT | VK_COLOR_COMPONENT_G_BIT |
                                      VK_COLOR_COMPONENT_B_BIT | VK_COLOR_COMPONENT_A_BIT;

    VkPipelineColorBlendStateCreateInfo blend{VK_STRUCTURE_TYPE_PIPELINE_COLOR_BLEND_STATE_CREATE_INFO};
    blend.attachmentCount = 1;
    blend.pAttachments = &blend_attachment;

    VkPipelineDepthStencilStateCreateInfo depth{VK_STRUCTURE_TYPE_PIPELINE_DEPTH_STENCIL_STATE_CREATE_INFO};
    depth.depthTestEnable = VK_TRUE;
    depth.depthWriteEnable = VK_TRUE;
    depth.depthCompareOp = VK_COMPARE_OP_LESS;

    const VkDynamicState dynamic_states[] = {VK_DYNAMIC_STATE_VIEWPORT, VK_DYNAMIC_STATE_SCISSOR};
    VkPipelineDynamicStateCreateInfo dynamic{VK_STRUCTURE_TYPE_PIPELINE_DYNAMIC_STATE_CREATE_INFO};
    dynamic.dynamicStateCount = 2;
    dynamic.pDynamicStates = dynamic_states;

    // Dynamic rendering: attachment formats are declared here instead of a VkRenderPass.
    VkPipelineRenderingCreateInfo rendering{VK_STRUCTURE_TYPE_PIPELINE_RENDERING_CREATE_INFO};
    rendering.colorAttachmentCount = 1;
    rendering.pColorAttachmentFormats = &color_format;
    rendering.depthAttachmentFormat = kDepthFormat;

    VkGraphicsPipelineCreateInfo info{VK_STRUCTURE_TYPE_GRAPHICS_PIPELINE_CREATE_INFO};
    info.pNext = &rendering;
    info.stageCount = 2;
    info.pStages = stages;
    info.pVertexInputState = &vertex_input;
    info.pInputAssemblyState = &input_assembly;
    info.pViewportState = &viewport;
    info.pRasterizationState = &raster;
    info.pMultisampleState = &multisample;
    info.pDepthStencilState = &depth;
    info.pColorBlendState = &blend;
    info.pDynamicState = &dynamic;
    info.layout = g->pipeline_layout;

    const VkResult result = vkCreateGraphicsPipelines(g->dev, VK_NULL_HANDLE, 1, &info, nullptr, &g->pipeline);
    vkDestroyShaderModule(g->dev, vert, nullptr);
    vkDestroyShaderModule(g->dev, frag, nullptr);
    VK_TRY(result);

    g->pipeline_format = color_format;
    return true;
}

// ---------------------------------------------------------------------------
// Memory: buffers, images, blocking uploads
// ---------------------------------------------------------------------------

void destroy_image(Image& img) {
    if (img.view) vkDestroyImageView(g->dev, img.view, nullptr);
    if (img.image) vmaDestroyImage(g->allocator, img.image, img.allocation);
    img = {};
}

void destroy_buffer(Buffer& b) {
    if (b.buffer) vmaDestroyBuffer(g->allocator, b.buffer, b.allocation);
    b = {};
}

bool create_buffer(VkDeviceSize size, VkBufferUsageFlags usage, VmaAllocationCreateFlags flags,
                   Buffer& out, void** mapped = nullptr) {
    VkBufferCreateInfo info{VK_STRUCTURE_TYPE_BUFFER_CREATE_INFO};
    info.size = size;
    info.usage = usage;

    VmaAllocationCreateInfo alloc{};
    alloc.usage = VMA_MEMORY_USAGE_AUTO;
    alloc.flags = flags;

    VmaAllocationInfo alloc_info{};
    VK_TRY(vmaCreateBuffer(g->allocator, &info, &alloc, &out.buffer, &out.allocation, &alloc_info));
    if (mapped) *mapped = alloc_info.pMappedData;
    return true;
}

// Host-visible, persistently mapped buffer filled with `data`.
bool create_staging(const void* data, VkDeviceSize size, Buffer& out) {
    void* mapped = nullptr;
    if (!create_buffer(size, VK_BUFFER_USAGE_TRANSFER_SRC_BIT,
                       VMA_ALLOCATION_CREATE_HOST_ACCESS_SEQUENTIAL_WRITE_BIT | VMA_ALLOCATION_CREATE_MAPPED_BIT,
                       out, &mapped)) {
        return false;
    }
    std::memcpy(mapped, data, static_cast<size_t>(size));
    vmaFlushAllocation(g->allocator, out.allocation, 0, VK_WHOLE_SIZE);
    return true;
}

// Records `record` into the upload command buffer, submits it and waits.
template <typename F>
bool submit_and_wait(F&& record) {
    VK_TRY(vkResetCommandPool(g->dev, g->upload_pool, 0));
    VkCommandBufferBeginInfo begin{VK_STRUCTURE_TYPE_COMMAND_BUFFER_BEGIN_INFO};
    begin.flags = VK_COMMAND_BUFFER_USAGE_ONE_TIME_SUBMIT_BIT;
    VK_TRY(vkBeginCommandBuffer(g->upload_cmd, &begin));
    record(g->upload_cmd);
    VK_TRY(vkEndCommandBuffer(g->upload_cmd));

    VkCommandBufferSubmitInfo cmd_info{VK_STRUCTURE_TYPE_COMMAND_BUFFER_SUBMIT_INFO};
    cmd_info.commandBuffer = g->upload_cmd;
    VkSubmitInfo2 submit{VK_STRUCTURE_TYPE_SUBMIT_INFO_2};
    submit.commandBufferInfoCount = 1;
    submit.pCommandBufferInfos = &cmd_info;
    VK_TRY(vkQueueSubmit2(g->queue, 1, &submit, g->upload_fence));
    VK_TRY(vkWaitForFences(g->dev, 1, &g->upload_fence, VK_TRUE, UINT64_MAX));
    VK_TRY(vkResetFences(g->dev, 1, &g->upload_fence));
    return true;
}

// Creates a device-local buffer and fills it through a staging buffer (blocking).
bool upload_buffer(const void* data, VkDeviceSize size, VkBufferUsageFlags usage, Buffer& out) {
    Buffer staging;
    if (!create_staging(data, size, staging)) return false;

    bool ok = create_buffer(size, usage | VK_BUFFER_USAGE_TRANSFER_DST_BIT, 0, out);
    if (ok) {
        ok = submit_and_wait([&](VkCommandBuffer cmd) {
            VkBufferCopy region{0, 0, size};
            vkCmdCopyBuffer(cmd, staging.buffer, out.buffer, 1, &region);

            // Make the copy visible to vertex/index fetches in later submissions.
            VkMemoryBarrier2 barrier{VK_STRUCTURE_TYPE_MEMORY_BARRIER_2};
            barrier.srcStageMask = VK_PIPELINE_STAGE_2_COPY_BIT;
            barrier.srcAccessMask = VK_ACCESS_2_TRANSFER_WRITE_BIT;
            barrier.dstStageMask = VK_PIPELINE_STAGE_2_VERTEX_INPUT_BIT;
            barrier.dstAccessMask = VK_ACCESS_2_VERTEX_ATTRIBUTE_READ_BIT | VK_ACCESS_2_INDEX_READ_BIT;
            VkDependencyInfo dep{VK_STRUCTURE_TYPE_DEPENDENCY_INFO};
            dep.memoryBarrierCount = 1;
            dep.pMemoryBarriers = &barrier;
            vkCmdPipelineBarrier2(cmd, &dep);
        });
        if (!ok) destroy_buffer(out);
    }
    destroy_buffer(staging);
    return ok;
}

void image_barrier(VkCommandBuffer cmd, VkImage image, VkImageSubresourceRange range,
                   VkImageLayout old_layout, VkImageLayout new_layout,
                   VkPipelineStageFlags2 src_stage, VkAccessFlags2 src_access,
                   VkPipelineStageFlags2 dst_stage, VkAccessFlags2 dst_access) {
    VkImageMemoryBarrier2 barrier{VK_STRUCTURE_TYPE_IMAGE_MEMORY_BARRIER_2};
    barrier.srcStageMask = src_stage;
    barrier.srcAccessMask = src_access;
    barrier.dstStageMask = dst_stage;
    barrier.dstAccessMask = dst_access;
    barrier.oldLayout = old_layout;
    barrier.newLayout = new_layout;
    barrier.srcQueueFamilyIndex = VK_QUEUE_FAMILY_IGNORED;
    barrier.dstQueueFamilyIndex = VK_QUEUE_FAMILY_IGNORED;
    barrier.image = image;
    barrier.subresourceRange = range;

    VkDependencyInfo dep{VK_STRUCTURE_TYPE_DEPENDENCY_INFO};
    dep.imageMemoryBarrierCount = 1;
    dep.pImageMemoryBarriers = &barrier;
    vkCmdPipelineBarrier2(cmd, &dep);
}

VkImageSubresourceRange color_mips(uint32_t base, uint32_t count) {
    return {VK_IMAGE_ASPECT_COLOR_BIT, base, count, 0, 1};
}

bool create_depth(VkExtent2D extent) {
    destroy_image(g->depth);

    VkImageCreateInfo info{VK_STRUCTURE_TYPE_IMAGE_CREATE_INFO};
    info.imageType = VK_IMAGE_TYPE_2D;
    info.format = kDepthFormat;
    info.extent = {extent.width, extent.height, 1};
    info.mipLevels = 1;
    info.arrayLayers = 1;
    info.samples = VK_SAMPLE_COUNT_1_BIT;
    info.tiling = VK_IMAGE_TILING_OPTIMAL;
    info.usage = VK_IMAGE_USAGE_DEPTH_STENCIL_ATTACHMENT_BIT;
    info.initialLayout = VK_IMAGE_LAYOUT_UNDEFINED;

    VmaAllocationCreateInfo alloc{};
    alloc.usage = VMA_MEMORY_USAGE_AUTO;
    alloc.flags = VMA_ALLOCATION_CREATE_DEDICATED_MEMORY_BIT;
    VK_TRY(vmaCreateImage(g->allocator, &info, &alloc, &g->depth.image, &g->depth.allocation, nullptr));

    VkImageViewCreateInfo view{VK_STRUCTURE_TYPE_IMAGE_VIEW_CREATE_INFO};
    view.image = g->depth.image;
    view.viewType = VK_IMAGE_VIEW_TYPE_2D;
    view.format = kDepthFormat;
    view.subresourceRange = {VK_IMAGE_ASPECT_DEPTH_BIT, 0, 1, 0, 1};
    VK_TRY(vkCreateImageView(g->dev, &view, nullptr, &g->depth.view));
    return true;
}

// ---------------------------------------------------------------------------
// Meshes
// ---------------------------------------------------------------------------

Mesh* lookup_mesh(RMesh handle) {
    if (handle == 0 || handle > g->meshes.size()) return nullptr;
    Mesh& mesh = g->meshes[handle - 1];
    return mesh.index_count ? &mesh : nullptr;
}

void destroy_mesh(Mesh& mesh) {
    destroy_buffer(mesh.vertices);
    destroy_buffer(mesh.indices);
    mesh.index_count = 0;
}

// Frees retired meshes that no unfinished frame can reference. Frames numbered
// below `finished` are known to be complete.
void release_retired_meshes(uint64_t finished) {
    auto& retired = g->retired_meshes;
    auto  done = [finished](Renderer::RetiredMesh& r) { return r.last_use <= finished; };
    for (auto& r : retired) {
        if (done(r)) destroy_mesh(r.mesh);
    }
    retired.erase(std::remove_if(retired.begin(), retired.end(), done), retired.end());
}

// ---------------------------------------------------------------------------
// Textures
// ---------------------------------------------------------------------------

bool texture_alive(RTexture handle) {
    return handle < g->textures.size() && g->textures[handle].image.image != VK_NULL_HANDLE;
}

void write_texture_descriptor(uint32_t slot, VkImageView view) {
    VkDescriptorImageInfo image_info{g->sampler, view, VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL};
    VkWriteDescriptorSet write{VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET};
    write.dstSet = g->texture_set;
    write.dstBinding = 0;
    write.dstArrayElement = slot;
    write.descriptorCount = 1;
    write.descriptorType = VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER;
    write.pImageInfo = &image_info;
    vkUpdateDescriptorSets(g->dev, 1, &write, 0, nullptr);
}

bool can_generate_mips(VkFormat format) {
    VkFormatProperties props{};
    vkGetPhysicalDeviceFormatProperties(g->device.physical_device.physical_device, format, &props);
    const VkFormatFeatureFlags needed = VK_FORMAT_FEATURE_BLIT_SRC_BIT | VK_FORMAT_FEATURE_BLIT_DST_BIT |
                                        VK_FORMAT_FEATURE_SAMPLED_IMAGE_FILTER_LINEAR_BIT;
    return (props.optimalTilingFeatures & needed) == needed;
}

uint32_t mip_count(uint32_t width, uint32_t height) {
    uint32_t levels = 1;
    for (uint32_t size = std::max(width, height); size > 1; size /= 2) ++levels;
    return levels;
}

// Uploads RGBA8 pixels into a sampled image and downsamples a full mip chain
// with linear blits. Leaves every mip in SHADER_READ_ONLY_OPTIMAL.
bool create_texture_image(const uint8_t* rgba, uint32_t width, uint32_t height, VkFormat format, Image& out) {
    const uint32_t levels = can_generate_mips(format) ? mip_count(width, height) : 1;

    VkImageCreateInfo info{VK_STRUCTURE_TYPE_IMAGE_CREATE_INFO};
    info.imageType = VK_IMAGE_TYPE_2D;
    info.format = format;
    info.extent = {width, height, 1};
    info.mipLevels = levels;
    info.arrayLayers = 1;
    info.samples = VK_SAMPLE_COUNT_1_BIT;
    info.tiling = VK_IMAGE_TILING_OPTIMAL;
    info.usage = VK_IMAGE_USAGE_SAMPLED_BIT | VK_IMAGE_USAGE_TRANSFER_DST_BIT | VK_IMAGE_USAGE_TRANSFER_SRC_BIT;
    info.initialLayout = VK_IMAGE_LAYOUT_UNDEFINED;

    VmaAllocationCreateInfo alloc{};
    alloc.usage = VMA_MEMORY_USAGE_AUTO;
    VK_TRY(vmaCreateImage(g->allocator, &info, &alloc, &out.image, &out.allocation, nullptr));

    Buffer staging;
    if (!create_staging(rgba, VkDeviceSize{width} * height * 4, staging)) {
        destroy_image(out);
        return false;
    }

    const bool uploaded = submit_and_wait([&](VkCommandBuffer cmd) {
        constexpr VkPipelineStageFlags2 kTransfer = VK_PIPELINE_STAGE_2_ALL_TRANSFER_BIT;
        constexpr VkPipelineStageFlags2 kSample = VK_PIPELINE_STAGE_2_FRAGMENT_SHADER_BIT;

        image_barrier(cmd, out.image, color_mips(0, levels),
                      VK_IMAGE_LAYOUT_UNDEFINED, VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL,
                      VK_PIPELINE_STAGE_2_NONE, 0, kTransfer, VK_ACCESS_2_TRANSFER_WRITE_BIT);

        VkBufferImageCopy copy{};
        copy.imageSubresource = {VK_IMAGE_ASPECT_COLOR_BIT, 0, 0, 1};
        copy.imageExtent = {width, height, 1};
        vkCmdCopyBufferToImage(cmd, staging.buffer, out.image, VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL, 1, &copy);

        int32_t w = static_cast<int32_t>(width), h = static_cast<int32_t>(height);
        for (uint32_t level = 1; level < levels; ++level) {
            // Previous level: written by copy/blit -> read by this blit.
            image_barrier(cmd, out.image, color_mips(level - 1, 1),
                          VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL, VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL,
                          kTransfer, VK_ACCESS_2_TRANSFER_WRITE_BIT, kTransfer, VK_ACCESS_2_TRANSFER_READ_BIT);

            const int32_t nw = std::max(w / 2, 1), nh = std::max(h / 2, 1);
            VkImageBlit blit{};
            blit.srcSubresource = {VK_IMAGE_ASPECT_COLOR_BIT, level - 1, 0, 1};
            blit.srcOffsets[1] = {w, h, 1};
            blit.dstSubresource = {VK_IMAGE_ASPECT_COLOR_BIT, level, 0, 1};
            blit.dstOffsets[1] = {nw, nh, 1};
            vkCmdBlitImage(cmd, out.image, VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL,
                           out.image, VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL, 1, &blit, VK_FILTER_LINEAR);

            image_barrier(cmd, out.image, color_mips(level - 1, 1),
                          VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL, VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL,
                          kTransfer, VK_ACCESS_2_TRANSFER_READ_BIT, kSample, VK_ACCESS_2_SHADER_SAMPLED_READ_BIT);
            w = nw;
            h = nh;
        }
        image_barrier(cmd, out.image, color_mips(levels - 1, 1),
                      VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL, VK_IMAGE_LAYOUT_SHADER_READ_ONLY_OPTIMAL,
                      kTransfer, VK_ACCESS_2_TRANSFER_WRITE_BIT, kSample, VK_ACCESS_2_SHADER_SAMPLED_READ_BIT);
    });
    destroy_buffer(staging);
    if (!uploaded) {
        destroy_image(out);
        return false;
    }

    VkImageViewCreateInfo view{VK_STRUCTURE_TYPE_IMAGE_VIEW_CREATE_INFO};
    view.image = out.image;
    view.viewType = VK_IMAGE_VIEW_TYPE_2D;
    view.format = format;
    view.subresourceRange = color_mips(0, levels);
    if (vkCreateImageView(g->dev, &view, nullptr, &out.view) != VK_SUCCESS) {
        destroy_image(out);
        return fail("texture view creation failed");
    }
    return true;
}

// ---------------------------------------------------------------------------
// Descriptors, uniforms, sampler, pipeline layout
// ---------------------------------------------------------------------------

bool create_descriptors() {
    // set 0: per-frame uniforms
    VkDescriptorSetLayoutBinding frame_binding{};
    frame_binding.binding = 0;
    frame_binding.descriptorType = VK_DESCRIPTOR_TYPE_UNIFORM_BUFFER;
    frame_binding.descriptorCount = 1;
    frame_binding.stageFlags = VK_SHADER_STAGE_VERTEX_BIT | VK_SHADER_STAGE_FRAGMENT_BIT;
    VkDescriptorSetLayoutCreateInfo frame_layout{VK_STRUCTURE_TYPE_DESCRIPTOR_SET_LAYOUT_CREATE_INFO};
    frame_layout.bindingCount = 1;
    frame_layout.pBindings = &frame_binding;
    VK_TRY(vkCreateDescriptorSetLayout(g->dev, &frame_layout, nullptr, &g->frame_set_layout));

    // set 1: bindless textures. Partially bound (unused slots may be empty) and
    // update-after-bind (new textures can be written while frames are in flight).
    VkDescriptorSetLayoutBinding texture_binding{};
    texture_binding.binding = 0;
    texture_binding.descriptorType = VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER;
    texture_binding.descriptorCount = kMaxTextures;
    texture_binding.stageFlags = VK_SHADER_STAGE_FRAGMENT_BIT;
    const VkDescriptorBindingFlags texture_flags = VK_DESCRIPTOR_BINDING_PARTIALLY_BOUND_BIT |
                                                   VK_DESCRIPTOR_BINDING_UPDATE_AFTER_BIND_BIT |
                                                   VK_DESCRIPTOR_BINDING_UPDATE_UNUSED_WHILE_PENDING_BIT;
    VkDescriptorSetLayoutBindingFlagsCreateInfo binding_flags{
        VK_STRUCTURE_TYPE_DESCRIPTOR_SET_LAYOUT_BINDING_FLAGS_CREATE_INFO};
    binding_flags.bindingCount = 1;
    binding_flags.pBindingFlags = &texture_flags;
    VkDescriptorSetLayoutCreateInfo texture_layout{VK_STRUCTURE_TYPE_DESCRIPTOR_SET_LAYOUT_CREATE_INFO};
    texture_layout.pNext = &binding_flags;
    texture_layout.flags = VK_DESCRIPTOR_SET_LAYOUT_CREATE_UPDATE_AFTER_BIND_POOL_BIT;
    texture_layout.bindingCount = 1;
    texture_layout.pBindings = &texture_binding;
    VK_TRY(vkCreateDescriptorSetLayout(g->dev, &texture_layout, nullptr, &g->texture_set_layout));

    VkDescriptorPoolSize frame_pool_size{VK_DESCRIPTOR_TYPE_UNIFORM_BUFFER, kFramesInFlight};
    VkDescriptorPoolCreateInfo frame_pool{VK_STRUCTURE_TYPE_DESCRIPTOR_POOL_CREATE_INFO};
    frame_pool.maxSets = kFramesInFlight;
    frame_pool.poolSizeCount = 1;
    frame_pool.pPoolSizes = &frame_pool_size;
    VK_TRY(vkCreateDescriptorPool(g->dev, &frame_pool, nullptr, &g->frame_pool));

    VkDescriptorPoolSize texture_pool_size{VK_DESCRIPTOR_TYPE_COMBINED_IMAGE_SAMPLER, kMaxTextures};
    VkDescriptorPoolCreateInfo texture_pool{VK_STRUCTURE_TYPE_DESCRIPTOR_POOL_CREATE_INFO};
    texture_pool.flags = VK_DESCRIPTOR_POOL_CREATE_UPDATE_AFTER_BIND_BIT;
    texture_pool.maxSets = 1;
    texture_pool.poolSizeCount = 1;
    texture_pool.pPoolSizes = &texture_pool_size;
    VK_TRY(vkCreateDescriptorPool(g->dev, &texture_pool, nullptr, &g->texture_pool));

    VkDescriptorSetAllocateInfo texture_alloc{VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO};
    texture_alloc.descriptorPool = g->texture_pool;
    texture_alloc.descriptorSetCount = 1;
    texture_alloc.pSetLayouts = &g->texture_set_layout;
    VK_TRY(vkAllocateDescriptorSets(g->dev, &texture_alloc, &g->texture_set));

    for (FrameData& f : g->frames) {
        if (!create_buffer(kFrameUniformSize, VK_BUFFER_USAGE_UNIFORM_BUFFER_BIT,
                           VMA_ALLOCATION_CREATE_HOST_ACCESS_SEQUENTIAL_WRITE_BIT | VMA_ALLOCATION_CREATE_MAPPED_BIT,
                           f.uniforms, &f.uniforms_mapped)) {
            return false;
        }
        VkDescriptorSetAllocateInfo alloc{VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO};
        alloc.descriptorPool = g->frame_pool;
        alloc.descriptorSetCount = 1;
        alloc.pSetLayouts = &g->frame_set_layout;
        VK_TRY(vkAllocateDescriptorSets(g->dev, &alloc, &f.frame_set));

        VkDescriptorBufferInfo buffer_info{f.uniforms.buffer, 0, kFrameUniformSize};
        VkWriteDescriptorSet write{VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET};
        write.dstSet = f.frame_set;
        write.dstBinding = 0;
        write.descriptorCount = 1;
        write.descriptorType = VK_DESCRIPTOR_TYPE_UNIFORM_BUFFER;
        write.pBufferInfo = &buffer_info;
        vkUpdateDescriptorSets(g->dev, 1, &write, 0, nullptr);
    }

    VkSamplerCreateInfo sampler{VK_STRUCTURE_TYPE_SAMPLER_CREATE_INFO};
    sampler.magFilter = VK_FILTER_LINEAR;
    sampler.minFilter = VK_FILTER_LINEAR;
    sampler.mipmapMode = VK_SAMPLER_MIPMAP_MODE_LINEAR;
    sampler.addressModeU = VK_SAMPLER_ADDRESS_MODE_REPEAT;
    sampler.addressModeV = VK_SAMPLER_ADDRESS_MODE_REPEAT;
    sampler.addressModeW = VK_SAMPLER_ADDRESS_MODE_REPEAT;
    sampler.anisotropyEnable = VK_TRUE;
    sampler.maxAnisotropy = g->max_anisotropy;
    sampler.maxLod = VK_LOD_CLAMP_NONE;
    VK_TRY(vkCreateSampler(g->dev, &sampler, nullptr, &g->sampler));

    const VkDescriptorSetLayout set_layouts[] = {g->frame_set_layout, g->texture_set_layout};
    VkPushConstantRange push{};
    push.stageFlags = VK_SHADER_STAGE_VERTEX_BIT | VK_SHADER_STAGE_FRAGMENT_BIT;
    push.size = kPushConstantSize;
    VkPipelineLayoutCreateInfo layout_info{VK_STRUCTURE_TYPE_PIPELINE_LAYOUT_CREATE_INFO};
    layout_info.setLayoutCount = 2;
    layout_info.pSetLayouts = set_layouts;
    layout_info.pushConstantRangeCount = 1;
    layout_info.pPushConstantRanges = &push;
    VK_TRY(vkCreatePipelineLayout(g->dev, &layout_info, nullptr, &g->pipeline_layout));

    // Slot 0: 1x1 white, so untextured draws just multiply by 1.
    const uint8_t white[4] = {255, 255, 255, 255};
    Texture tex;
    if (!create_texture_image(white, 1, 1, VK_FORMAT_R8G8B8A8_UNORM, tex.image)) return false;
    g->textures.push_back(tex);
    write_texture_descriptor(0, tex.image.view);
    return true;
}

// ---------------------------------------------------------------------------
// Swapchain and per-frame objects
// ---------------------------------------------------------------------------

void destroy_swapchain_resources() {
    for (VkSemaphore s : g->render_done) vkDestroySemaphore(g->dev, s, nullptr);
    g->render_done.clear();
    if (!g->views.empty()) g->swapchain.destroy_image_views(g->views);
    g->views.clear();
    g->images.clear();
}

bool create_swapchain() {
    // Draw in the surface's native orientation (see Renderer::transform).
    VkSurfaceCapabilitiesKHR caps{};
    VK_TRY(vkGetPhysicalDeviceSurfaceCapabilitiesKHR(g->device.physical_device.physical_device, g->surface, &caps));
    g->transform = caps.currentTransform;
    if (!(caps.supportedTransforms & g->transform)) g->transform = VK_SURFACE_TRANSFORM_IDENTITY_BIT_KHR;

    vkb::SwapchainBuilder builder{g->device, g->surface};
    auto built = builder
                     .set_desired_format({VK_FORMAT_B8G8R8A8_SRGB, VK_COLOR_SPACE_SRGB_NONLINEAR_KHR})
                     .add_fallback_format({VK_FORMAT_R8G8B8A8_SRGB, VK_COLOR_SPACE_SRGB_NONLINEAR_KHR})
                     .set_pre_transform_flags(g->transform)
                     .set_composite_alpha_flags(caps.supportedCompositeAlpha & VK_COMPOSITE_ALPHA_OPAQUE_BIT_KHR
                                                    ? VK_COMPOSITE_ALPHA_OPAQUE_BIT_KHR
                                                    : VK_COMPOSITE_ALPHA_INHERIT_BIT_KHR)
                     .set_desired_present_mode(g->vsync ? VK_PRESENT_MODE_FIFO_KHR : VK_PRESENT_MODE_MAILBOX_KHR)
                     .set_desired_extent(g->width, g->height)
                     .add_image_usage_flags(VK_IMAGE_USAGE_TRANSFER_SRC_BIT) // frame capture
                     .set_old_swapchain(g->swapchain)
                     .build();
    if (!built.has_value()) return fail("swapchain creation failed: " + built.error().message());

    destroy_swapchain_resources();
    vkb::destroy_swapchain(g->swapchain); // no-op on the first call
    g->swapchain = built.value();

    char msg[160];
    std::snprintf(msg, sizeof msg, "[renderer] swapchain %ux%u format %d colorspace %d present mode %d transform %d",
                  g->swapchain.extent.width, g->swapchain.extent.height, static_cast<int>(g->swapchain.image_format),
                  static_cast<int>(g->swapchain.color_space), static_cast<int>(g->swapchain.present_mode),
                  static_cast<int>(g->transform));
    platform_log(msg);

    auto images = g->swapchain.get_images();
    auto views = g->swapchain.get_image_views();
    if (!images.has_value() || !views.has_value()) return fail("failed to get swapchain images");
    g->images = images.value();
    g->views = views.value();

    VkSemaphoreCreateInfo sem_info{VK_STRUCTURE_TYPE_SEMAPHORE_CREATE_INFO};
    g->render_done.resize(g->images.size(), VK_NULL_HANDLE);
    for (VkSemaphore& s : g->render_done) VK_TRY(vkCreateSemaphore(g->dev, &sem_info, nullptr, &s));

    if (!create_depth(g->swapchain.extent)) return false;

    if (g->swapchain.image_format != g->pipeline_format) {
        if (!create_pipeline(g->swapchain.image_format)) return false;
        if (g->ui_ready) ui_set_formats(g->swapchain.image_format, kDepthFormat);
    }

    g->swapchain_dirty = false;
    return true;
}

bool recreate_swapchain() {
    vkDeviceWaitIdle(g->dev);
    return create_swapchain();
}

bool create_frames() {
    VkCommandPoolCreateInfo pool_info{VK_STRUCTURE_TYPE_COMMAND_POOL_CREATE_INFO};
    pool_info.flags = VK_COMMAND_POOL_CREATE_TRANSIENT_BIT;
    pool_info.queueFamilyIndex = g->queue_family;

    VkSemaphoreCreateInfo sem_info{VK_STRUCTURE_TYPE_SEMAPHORE_CREATE_INFO};
    VkFenceCreateInfo fence_info{VK_STRUCTURE_TYPE_FENCE_CREATE_INFO};
    fence_info.flags = VK_FENCE_CREATE_SIGNALED_BIT;

    for (FrameData& f : g->frames) {
        VK_TRY(vkCreateCommandPool(g->dev, &pool_info, nullptr, &f.pool));
        VkCommandBufferAllocateInfo alloc{VK_STRUCTURE_TYPE_COMMAND_BUFFER_ALLOCATE_INFO};
        alloc.commandPool = f.pool;
        alloc.level = VK_COMMAND_BUFFER_LEVEL_PRIMARY;
        alloc.commandBufferCount = 1;
        VK_TRY(vkAllocateCommandBuffers(g->dev, &alloc, &f.cmd));
        VK_TRY(vkCreateSemaphore(g->dev, &sem_info, nullptr, &f.image_acquired));
        VK_TRY(vkCreateFence(g->dev, &fence_info, nullptr, &f.in_flight));
    }

    VK_TRY(vkCreateCommandPool(g->dev, &pool_info, nullptr, &g->upload_pool));
    VkCommandBufferAllocateInfo alloc{VK_STRUCTURE_TYPE_COMMAND_BUFFER_ALLOCATE_INFO};
    alloc.commandPool = g->upload_pool;
    alloc.level = VK_COMMAND_BUFFER_LEVEL_PRIMARY;
    alloc.commandBufferCount = 1;
    VK_TRY(vkAllocateCommandBuffers(g->dev, &alloc, &g->upload_cmd));
    VkFenceCreateInfo upload_fence_info{VK_STRUCTURE_TYPE_FENCE_CREATE_INFO};
    VK_TRY(vkCreateFence(g->dev, &upload_fence_info, nullptr, &g->upload_fence));
    return true;
}

// ---------------------------------------------------------------------------
// Frame capture
// ---------------------------------------------------------------------------

// Records a copy of the current swapchain image into the host-readable capture
// buffer and leaves the image in PRESENT_SRC. Returns false (recording nothing)
// if the buffer can't be allocated, so the caller does the normal transition.
bool record_capture(VkCommandBuffer cmd) {
    const VkExtent2D   extent = g->swapchain.extent;
    const VkDeviceSize size = VkDeviceSize{extent.width} * extent.height * 4;
    if (size > g->capture_capacity) {
        if (g->capture_buffer.buffer) vkDeviceWaitIdle(g->dev); // an older copy may still target it
        destroy_buffer(g->capture_buffer);
        g->capture_capacity = 0;
        if (!create_buffer(size, VK_BUFFER_USAGE_TRANSFER_DST_BIT,
                           VMA_ALLOCATION_CREATE_HOST_ACCESS_RANDOM_BIT | VMA_ALLOCATION_CREATE_MAPPED_BIT,
                           g->capture_buffer, &g->capture_mapped)) {
            return false;
        }
        g->capture_capacity = size;
    }

    VkImage image = g->images[g->image];
    image_barrier(cmd, image, color_mips(0, 1),
                  VK_IMAGE_LAYOUT_COLOR_ATTACHMENT_OPTIMAL, VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL,
                  VK_PIPELINE_STAGE_2_COLOR_ATTACHMENT_OUTPUT_BIT, VK_ACCESS_2_COLOR_ATTACHMENT_WRITE_BIT,
                  VK_PIPELINE_STAGE_2_COPY_BIT, VK_ACCESS_2_TRANSFER_READ_BIT);

    VkBufferImageCopy region{};
    region.imageSubresource = {VK_IMAGE_ASPECT_COLOR_BIT, 0, 0, 1};
    region.imageExtent = {extent.width, extent.height, 1};
    vkCmdCopyImageToBuffer(cmd, image, VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL, g->capture_buffer.buffer, 1, &region);

    image_barrier(cmd, image, color_mips(0, 1),
                  VK_IMAGE_LAYOUT_TRANSFER_SRC_OPTIMAL, VK_IMAGE_LAYOUT_PRESENT_SRC_KHR,
                  VK_PIPELINE_STAGE_2_COPY_BIT, VK_ACCESS_2_TRANSFER_READ_BIT,
                  VK_PIPELINE_STAGE_2_BOTTOM_OF_PIPE_BIT, 0);

    VkMemoryBarrier2 to_host{VK_STRUCTURE_TYPE_MEMORY_BARRIER_2};
    to_host.srcStageMask = VK_PIPELINE_STAGE_2_COPY_BIT;
    to_host.srcAccessMask = VK_ACCESS_2_TRANSFER_WRITE_BIT;
    to_host.dstStageMask = VK_PIPELINE_STAGE_2_HOST_BIT;
    to_host.dstAccessMask = VK_ACCESS_2_HOST_READ_BIT;
    VkDependencyInfo dep{VK_STRUCTURE_TYPE_DEPENDENCY_INFO};
    dep.memoryBarrierCount = 1;
    dep.pMemoryBarriers = &to_host;
    vkCmdPipelineBarrier2(cmd, &dep);

    g->capture_pending = true;
    g->capture_frame = g->frame;
    g->capture_width = extent.width;
    g->capture_height = extent.height;
    g->capture_format = g->swapchain.image_format;
    return true;
}

// ---------------------------------------------------------------------------
// Init / shutdown
// ---------------------------------------------------------------------------

// Pre-rotation needs the swapchain in the surface's native orientation, but
// Android reports currentExtent in the window's orientation (1920x1080 on a
// portrait 1080x1920 panel used in landscape, with a ROTATE_90 transform).
// vk-bootstrap always uses currentExtent, so it gets its surface capabilities
// through this wrapper, which swaps the extents for 90/270-degree transforms.
PFN_vkGetPhysicalDeviceSurfaceCapabilitiesKHR g_real_surface_caps = nullptr;

VKAPI_ATTR VkResult VKAPI_CALL native_surface_caps(VkPhysicalDevice device, VkSurfaceKHR surface,
                                                   VkSurfaceCapabilitiesKHR* caps) {
    const VkResult result = g_real_surface_caps(device, surface, caps);
    if (result == VK_SUCCESS && rotated_sideways(caps->currentTransform)) {
        std::swap(caps->currentExtent.width, caps->currentExtent.height);
        std::swap(caps->minImageExtent.width, caps->minImageExtent.height);
        std::swap(caps->maxImageExtent.width, caps->maxImageExtent.height);
    }
    return result;
}

VKAPI_ATTR PFN_vkVoidFunction VKAPI_CALL bootstrap_proc_addr(VkInstance instance, const char* name) {
    if (instance && std::strcmp(name, "vkGetPhysicalDeviceSurfaceCapabilitiesKHR") == 0) {
        g_real_surface_caps = reinterpret_cast<PFN_vkGetPhysicalDeviceSurfaceCapabilitiesKHR>(
            vkGetInstanceProcAddr(instance, name));
        return reinterpret_cast<PFN_vkVoidFunction>(native_surface_caps);
    }
    return vkGetInstanceProcAddr(instance, name);
}

// The first GPU's features (VK_NULL_HANDLE if there's none).
VkPhysicalDevice first_gpu_features(const vkb::Instance& instance, VkPhysicalDeviceFeatures2* f,
                                    VkPhysicalDeviceVulkan12Features* f12, VkPhysicalDeviceVulkan13Features* f13) {
    uint32_t         count = 1;
    VkPhysicalDevice gpu = VK_NULL_HANDLE;
    if (vkEnumeratePhysicalDevices(instance.instance, &count, &gpu) < 0 || !gpu) return VK_NULL_HANDLE;
    *f13 = {VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_VULKAN_1_3_FEATURES};
    *f12 = {VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_VULKAN_1_2_FEATURES, f13};
    *f = {VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_FEATURES_2, f12};
    vkGetPhysicalDeviceFeatures2(gpu, f);
    return gpu;
}

// Lists the required features the first GPU lacks, for the error message
// when none is suitable.
std::string missing_features(const vkb::Instance& instance) {
    VkPhysicalDeviceFeatures2        f;
    VkPhysicalDeviceVulkan12Features f12;
    VkPhysicalDeviceVulkan13Features f13;
    VkPhysicalDevice                 gpu = first_gpu_features(instance, &f, &f12, &f13);
    if (!gpu) return "";
    VkPhysicalDeviceProperties props{};
    vkGetPhysicalDeviceProperties(gpu, &props);
    std::string out = " (" + std::string(props.deviceName) + ", Vulkan " +
                      std::to_string(VK_API_VERSION_MAJOR(props.apiVersion)) + "." +
                      std::to_string(VK_API_VERSION_MINOR(props.apiVersion)) + "; missing:";
    auto need = [&](VkBool32 have, const char* name) {
        if (!have) out += std::string(" ") + name;
    };
    need(f.features.samplerAnisotropy, "samplerAnisotropy");
    need(f.features.shaderSampledImageArrayDynamicIndexing, "shaderSampledImageArrayDynamicIndexing");
    need(f12.descriptorBindingPartiallyBound, "descriptorBindingPartiallyBound");
    need(f12.descriptorBindingSampledImageUpdateAfterBind, "descriptorBindingSampledImageUpdateAfterBind");
    need(f12.descriptorBindingUpdateUnusedWhilePending, "descriptorBindingUpdateUnusedWhilePending");
    need(f13.dynamicRendering, "dynamicRendering");
    need(f13.synchronization2, "synchronization2");
    return out + ")";
}

bool init(const RInitDesc& desc) {
    VK_TRY(platform_load_vulkan());

    vkb::InstanceBuilder inst_builder(bootstrap_proc_addr);
    inst_builder.set_app_name("Cliff Crack").require_api_version(1, 3, 0);
    if (desc.enable_validation) {
        inst_builder.request_validation_layers().use_default_debug_messenger();
    }
    auto inst = inst_builder.build();
    if (!inst.has_value()) return fail("instance creation failed: " + inst.error().message());
    g->instance = inst.value();
    volkLoadInstance(g->instance.instance);

    if (!platform_create_surface(g->instance.instance, desc.native_window, &g->surface)) {
        return fail("surface creation failed");
    }

    VkPhysicalDeviceFeatures features{};
    features.samplerAnisotropy = VK_TRUE;
    features.shaderSampledImageArrayDynamicIndexing = VK_TRUE;
#ifdef __APPLE__
    // MoltenVK doesn't claim dynamic texture-array indexing for older Metal
    // GPU families (the iOS Simulator's among them), but indexing the Metal
    // texture array with a push constant, as mesh.frag does, works on all of
    // them. Newer iPhones (A13 on) do claim it.
    {
        VkPhysicalDeviceFeatures2        have;
        VkPhysicalDeviceVulkan12Features have12;
        VkPhysicalDeviceVulkan13Features have13;
        if (first_gpu_features(g->instance, &have, &have12, &have13) &&
            !have.features.shaderSampledImageArrayDynamicIndexing) {
            features.shaderSampledImageArrayDynamicIndexing = VK_FALSE;
            platform_log("[renderer] GPU doesn't claim dynamic texture indexing; relying on Metal's");
        }
    }
#endif

    VkPhysicalDeviceVulkan12Features features12{VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_VULKAN_1_2_FEATURES};
    features12.descriptorBindingPartiallyBound = VK_TRUE;
    features12.descriptorBindingSampledImageUpdateAfterBind = VK_TRUE;
    features12.descriptorBindingUpdateUnusedWhilePending = VK_TRUE;

    VkPhysicalDeviceVulkan13Features features13{VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_VULKAN_1_3_FEATURES};
    features13.dynamicRendering = VK_TRUE;
    features13.synchronization2 = VK_TRUE;

    auto phys = vkb::PhysicalDeviceSelector(g->instance, g->surface)
                    .set_minimum_version(1, 3)
                    .set_required_features(features)
                    .set_required_features_12(features12)
                    .set_required_features_13(features13)
                    .select();
    if (!phys.has_value()) return fail("no suitable GPU: " + phys.error().message() + missing_features(g->instance));
    platform_log(("[renderer] GPU: " + phys.value().name).c_str());
    g->max_anisotropy = std::min(16.0f, phys.value().properties.limits.maxSamplerAnisotropy);

    auto device = vkb::DeviceBuilder(phys.value()).build();
    if (!device.has_value()) return fail("device creation failed: " + device.error().message());
    g->device = device.value();
    g->dev = g->device.device;
    volkLoadDevice(g->dev);

    auto queue = g->device.get_queue(vkb::QueueType::graphics);
    auto family = g->device.get_queue_index(vkb::QueueType::graphics);
    if (!queue.has_value() || !family.has_value()) return fail("no graphics queue");
    g->queue = queue.value();
    g->queue_family = family.value();

    VmaVulkanFunctions vma_funcs{};
    vma_funcs.vkGetInstanceProcAddr = vkGetInstanceProcAddr;
    vma_funcs.vkGetDeviceProcAddr = vkGetDeviceProcAddr;
    VmaAllocatorCreateInfo vma_info{};
    vma_info.vulkanApiVersion = VK_API_VERSION_1_3;
    vma_info.instance = g->instance.instance;
    vma_info.physicalDevice = g->device.physical_device.physical_device;
    vma_info.device = g->dev;
    vma_info.pVulkanFunctions = &vma_funcs;
    VK_TRY(vmaCreateAllocator(&vma_info, &g->allocator));

    if (!create_frames()) return false;
    if (!create_descriptors()) return false;
    if (!create_swapchain()) return false; // also builds the pipeline for the swapchain format

    UiInitInfo ui{};
    ui.instance = g->instance.instance;
    ui.physical_device = g->device.physical_device.physical_device;
    ui.device = g->dev;
    ui.queue_family = g->queue_family;
    ui.queue = g->queue;
    ui.image_count = static_cast<uint32_t>(g->images.size());
    ui.color_format = g->swapchain.image_format;
    ui.depth_format = kDepthFormat;
    ui.scale = g->ui_scale;
    std::string ui_error;
    g->ui_ready = ui_init(ui, &ui_error);
    if (!g->ui_ready) platform_log(("[renderer] debug UI disabled: " + ui_error).c_str());
    return true;
}

// Binds the scene pipeline, descriptor sets and viewport for the current frame.
void bind_scene_state(VkCommandBuffer cmd) {
    const VkExtent2D extent = g->swapchain.extent;
    VkViewport viewport{0.0f, 0.0f, static_cast<float>(extent.width), static_cast<float>(extent.height), 0.0f, 1.0f};
    VkRect2D   scissor{{0, 0}, extent};
    vkCmdSetViewport(cmd, 0, 1, &viewport);
    vkCmdSetScissor(cmd, 0, 1, &scissor);
    vkCmdBindPipeline(cmd, VK_PIPELINE_BIND_POINT_GRAPHICS, g->pipeline);
    const VkDescriptorSet sets[] = {g->frames[g->frame].frame_set, g->texture_set};
    vkCmdBindDescriptorSets(cmd, VK_PIPELINE_BIND_POINT_GRAPHICS, g->pipeline_layout, 0, 2, sets, 0, nullptr);
    g->scene_state_bound = true;
}

void destroy_all() {
    if (g->dev) {
        vkDeviceWaitIdle(g->dev);
        if (g->ui_ready) ui_shutdown();
        for (FrameData& f : g->frames) {
            if (f.in_flight) vkDestroyFence(g->dev, f.in_flight, nullptr);
            if (f.image_acquired) vkDestroySemaphore(g->dev, f.image_acquired, nullptr);
            if (f.pool) vkDestroyCommandPool(g->dev, f.pool, nullptr);
        }
        if (g->upload_fence) vkDestroyFence(g->dev, g->upload_fence, nullptr);
        if (g->upload_pool) vkDestroyCommandPool(g->dev, g->upload_pool, nullptr);
        destroy_pipeline();
        if (g->pipeline_layout) vkDestroyPipelineLayout(g->dev, g->pipeline_layout, nullptr);
        if (g->sampler) vkDestroySampler(g->dev, g->sampler, nullptr);
        if (g->frame_pool) vkDestroyDescriptorPool(g->dev, g->frame_pool, nullptr);
        if (g->texture_pool) vkDestroyDescriptorPool(g->dev, g->texture_pool, nullptr);
        if (g->frame_set_layout) vkDestroyDescriptorSetLayout(g->dev, g->frame_set_layout, nullptr);
        if (g->texture_set_layout) vkDestroyDescriptorSetLayout(g->dev, g->texture_set_layout, nullptr);
        destroy_swapchain_resources();
        vkb::destroy_swapchain(g->swapchain);
        if (g->allocator) {
            for (FrameData& f : g->frames) destroy_buffer(f.uniforms);
            destroy_buffer(g->capture_buffer);
            for (Mesh& mesh : g->meshes) destroy_mesh(mesh);
            for (auto& r : g->retired_meshes) destroy_mesh(r.mesh);
            for (Texture& tex : g->textures) destroy_image(tex.image);
            destroy_image(g->depth);
            vmaDestroyAllocator(g->allocator);
        }
        vkb::destroy_device(g->device);
    }
    if (g->surface) vkb::destroy_surface(g->instance, g->surface);
    if (g->instance.instance) vkb::destroy_instance(g->instance);
}

} // namespace

// ---------------------------------------------------------------------------
// C API
// ---------------------------------------------------------------------------

extern "C" {

int32_t r_init(const RInitDesc* desc) {
    if (g) return fail("r_init called twice"), 0;
    if (!desc || !desc->native_window) return fail("r_init: missing native window"), 0;

    g = new Renderer();
    g->width = desc->width;
    g->height = desc->height;
    g->vsync = desc->vsync != 0;
    g->ui_scale = desc->ui_scale > 0.0f ? desc->ui_scale : 1.0f;
    g->shader_dir = desc->shader_dir ? desc->shader_dir : "shaders";

    if (!init(*desc)) {
        destroy_all();
        delete g;
        g = nullptr;
        return 0;
    }
    return 1;
}

const char* r_last_error(void) {
    return g_error.c_str();
}

void r_set_window(void* native_window) {
    if (!g) return;
    vkDeviceWaitIdle(g->dev);
    destroy_swapchain_resources();
    vkb::destroy_swapchain(g->swapchain);
    g->swapchain = vkb::Swapchain{};
    if (g->surface) vkb::destroy_surface(g->instance, g->surface);
    g->surface = VK_NULL_HANDLE;
    if (!native_window) return;
    if (!platform_create_surface(g->instance.instance, native_window, &g->surface)) {
        fail("r_set_window: surface creation failed");
        return;
    }
    VkBool32 supported = VK_FALSE;
    vkGetPhysicalDeviceSurfaceSupportKHR(g->device.physical_device.physical_device, g->queue_family, g->surface,
                                         &supported);
    if (!supported) {
        fail("r_set_window: the GPU can't present to the new window");
        return;
    }
    if (!create_swapchain()) fail("r_set_window: " + g_error);
}

void r_display_size(uint32_t* width, uint32_t* height) {
    VkExtent2D e{0, 0};
    if (g && g->swapchain.swapchain) e = display_extent();
    if (width) *width = e.width;
    if (height) *height = e.height;
}

void r_resize(uint32_t width, uint32_t height) {
    if (!g) return;
    if (width == g->width && height == g->height) return;
    g->width = width;
    g->height = height;
    g->swapchain_dirty = true;
}

RMesh r_create_mesh(const RVertex* vertices, uint32_t vertex_count,
                    const uint32_t* indices, uint32_t index_count) {
    if (!g) return fail("r_create_mesh: renderer not initialized"), 0;
    if (!vertices || !indices || vertex_count == 0 || index_count == 0 || index_count % 3 != 0) {
        return fail("r_create_mesh: need vertices and a non-empty triangle list"), 0;
    }
    for (uint32_t i = 0; i < index_count; ++i) {
        if (indices[i] >= vertex_count) return fail("r_create_mesh: index out of range"), 0;
    }

    Mesh mesh;
    if (!upload_buffer(vertices, VkDeviceSize{sizeof(RVertex)} * vertex_count,
                       VK_BUFFER_USAGE_VERTEX_BUFFER_BIT, mesh.vertices)) {
        return 0;
    }
    if (!upload_buffer(indices, VkDeviceSize{sizeof(uint32_t)} * index_count,
                       VK_BUFFER_USAGE_INDEX_BUFFER_BIT, mesh.indices)) {
        destroy_buffer(mesh.vertices);
        return 0;
    }
    mesh.index_count = index_count;

    uint32_t slot;
    if (!g->free_meshes.empty()) {
        slot = g->free_meshes.back();
        g->free_meshes.pop_back();
        g->meshes[slot] = mesh;
    } else {
        slot = static_cast<uint32_t>(g->meshes.size());
        g->meshes.push_back(mesh);
    }
    return slot + 1;
}

void r_destroy_mesh(RMesh handle) {
    if (!g) return;
    Mesh* mesh = lookup_mesh(handle);
    if (!mesh) return;
    // Frames up to and including the one being recorded may still draw it.
    const uint64_t last_use = g->submitted + (g->recording ? 1 : 0);
    g->retired_meshes.push_back({*mesh, last_use});
    *mesh = Mesh{};
    g->free_meshes.push_back(handle - 1);
}

RTexture r_create_texture(const uint8_t* rgba, uint32_t width, uint32_t height, uint32_t flags) {
    if (!g) return fail("r_create_texture: renderer not initialized"), 0;
    if (!rgba || width == 0 || height == 0) return fail("r_create_texture: empty image"), 0;

    uint32_t slot;
    if (!g->free_textures.empty()) {
        slot = g->free_textures.back();
    } else if (g->textures.size() < kMaxTextures) {
        slot = static_cast<uint32_t>(g->textures.size());
    } else {
        return fail("r_create_texture: texture table full (" + std::to_string(kMaxTextures) + ")"), 0;
    }

    const VkFormat format = (flags & R_TEXTURE_SRGB) ? VK_FORMAT_R8G8B8A8_SRGB : VK_FORMAT_R8G8B8A8_UNORM;
    Texture tex;
    if (!create_texture_image(rgba, width, height, format, tex.image)) return 0;

    if (slot == g->textures.size()) {
        g->textures.push_back(tex);
    } else {
        g->free_textures.pop_back();
        g->textures[slot] = tex;
    }
    write_texture_descriptor(slot, tex.image.view);
    return slot;
}

void r_destroy_texture(RTexture handle) {
    if (!g || handle == 0 || !texture_alive(handle)) return;
    vkDeviceWaitIdle(g->dev); // it may still be referenced by frames in flight
    write_texture_descriptor(handle, g->textures[0].image.view); // never leave a dangling view
    destroy_image(g->textures[handle].image);
    g->free_textures.push_back(handle);
}

int32_t r_begin_frame(const RFrameParams* params) {
    if (!g || g->recording || !params) return 0;
    if (g->width == 0 || g->height == 0) return 0; // minimized
    if (!g->surface) return 0;                       // no window (Android, in the background)

    if (g->swapchain_dirty && !recreate_swapchain()) return 0;

    FrameData& f = g->frames[g->frame];
    vkWaitForFences(g->dev, 1, &f.in_flight, VK_TRUE, UINT64_MAX);
    // This slot's fence belonged to frame (submitted - kFramesInFlight), and the
    // other slots' fences were waited on before it, so everything older is done.
    if (g->submitted >= kFramesInFlight - 1) release_retired_meshes(g->submitted - (kFramesInFlight - 1));

    VkResult acquired = vkAcquireNextImageKHR(g->dev, g->swapchain.swapchain, UINT64_MAX,
                                              f.image_acquired, VK_NULL_HANDLE, &g->image);
    if (acquired == VK_ERROR_OUT_OF_DATE_KHR) {
        g->swapchain_dirty = true;
        return 0;
    }
    if (acquired == VK_SUBOPTIMAL_KHR) {
        g->swapchain_dirty = true; // still usable this frame; rebuild after present
    } else if (acquired != VK_SUCCESS) {
        fail("vkAcquireNextImageKHR failed: VkResult " + std::to_string(static_cast<int>(acquired)));
        return 0;
    }

    // Safe to overwrite: this frame's fence says the GPU is done with its uniforms.
    RFrameParams uniforms = *params;
    pre_rotate(uniforms.view_proj, g->transform);
    std::memcpy(f.uniforms_mapped, &uniforms, kFrameUniformSize);
    vmaFlushAllocation(g->allocator, f.uniforms.allocation, 0, VK_WHOLE_SIZE);

    vkResetFences(g->dev, 1, &f.in_flight);
    vkResetCommandPool(g->dev, f.pool, 0);

    VkCommandBufferBeginInfo begin{VK_STRUCTURE_TYPE_COMMAND_BUFFER_BEGIN_INFO};
    begin.flags = VK_COMMAND_BUFFER_USAGE_ONE_TIME_SUBMIT_BIT;
    vkBeginCommandBuffer(f.cmd, &begin);

    image_barrier(f.cmd, g->images[g->image], color_mips(0, 1),
                  VK_IMAGE_LAYOUT_UNDEFINED, VK_IMAGE_LAYOUT_COLOR_ATTACHMENT_OPTIMAL,
                  VK_PIPELINE_STAGE_2_COLOR_ATTACHMENT_OUTPUT_BIT, 0,
                  VK_PIPELINE_STAGE_2_COLOR_ATTACHMENT_OUTPUT_BIT, VK_ACCESS_2_COLOR_ATTACHMENT_WRITE_BIT);

    // The depth image is shared by both frames in flight: wait for the previous
    // frame's depth writes before this frame clears it.
    constexpr VkPipelineStageFlags2 kDepthStages =
        VK_PIPELINE_STAGE_2_EARLY_FRAGMENT_TESTS_BIT | VK_PIPELINE_STAGE_2_LATE_FRAGMENT_TESTS_BIT;
    image_barrier(f.cmd, g->depth.image, {VK_IMAGE_ASPECT_DEPTH_BIT, 0, 1, 0, 1},
                  VK_IMAGE_LAYOUT_UNDEFINED, VK_IMAGE_LAYOUT_DEPTH_ATTACHMENT_OPTIMAL,
                  kDepthStages, VK_ACCESS_2_DEPTH_STENCIL_ATTACHMENT_WRITE_BIT,
                  kDepthStages, VK_ACCESS_2_DEPTH_STENCIL_ATTACHMENT_READ_BIT |
                                    VK_ACCESS_2_DEPTH_STENCIL_ATTACHMENT_WRITE_BIT);

    VkRenderingAttachmentInfo color{VK_STRUCTURE_TYPE_RENDERING_ATTACHMENT_INFO};
    color.imageView = g->views[g->image];
    color.imageLayout = VK_IMAGE_LAYOUT_COLOR_ATTACHMENT_OPTIMAL;
    color.loadOp = VK_ATTACHMENT_LOAD_OP_CLEAR;
    color.storeOp = VK_ATTACHMENT_STORE_OP_STORE;
    for (int i = 0; i < 4; ++i) color.clearValue.color.float32[i] = params->clear_color[i];

    VkRenderingAttachmentInfo depth{VK_STRUCTURE_TYPE_RENDERING_ATTACHMENT_INFO};
    depth.imageView = g->depth.view;
    depth.imageLayout = VK_IMAGE_LAYOUT_DEPTH_ATTACHMENT_OPTIMAL;
    depth.loadOp = VK_ATTACHMENT_LOAD_OP_CLEAR;
    depth.storeOp = VK_ATTACHMENT_STORE_OP_DONT_CARE;
    depth.clearValue.depthStencil.depth = 1.0f;

    const VkExtent2D extent = g->swapchain.extent;
    VkRenderingInfo rendering{VK_STRUCTURE_TYPE_RENDERING_INFO};
    rendering.renderArea = {{0, 0}, extent};
    rendering.layerCount = 1;
    rendering.colorAttachmentCount = 1;
    rendering.pColorAttachments = &color;
    rendering.pDepthAttachment = &depth;
    vkCmdBeginRendering(f.cmd, &rendering);
    bind_scene_state(f.cmd);

    g->recording = true;
    return 1;
}

void r_draw(const RDrawCmd* cmds, uint32_t count) {
    if (!g || !g->recording || !cmds) return;
    VkCommandBuffer cmd = g->frames[g->frame].cmd;
    if (!g->scene_state_bound) bind_scene_state(cmd); // r_ui ran earlier this frame
    const Mesh*     bound = nullptr;
    for (uint32_t i = 0; i < count; ++i) {
        const Mesh* mesh = lookup_mesh(cmds[i].mesh);
        if (!mesh) continue;
        if (mesh != bound) {
            const VkDeviceSize offset = 0;
            vkCmdBindVertexBuffers(cmd, 0, 1, &mesh->vertices.buffer, &offset);
            vkCmdBindIndexBuffer(cmd, mesh->indices.buffer, 0, VK_INDEX_TYPE_UINT32);
            bound = mesh;
        }
        RDrawCmd push = cmds[i];
        if (!texture_alive(push.texture)) push.texture = 0; // stale handle: fall back to white
        vkCmdPushConstants(cmd, g->pipeline_layout, VK_SHADER_STAGE_VERTEX_BIT | VK_SHADER_STAGE_FRAGMENT_BIT,
                           0, kPushConstantSize, &push);
        vkCmdDrawIndexed(cmd, mesh->index_count, 1, 0, 0, 0);
    }
}

void r_ui(const RUIInput* input, RUICmd* cmds, uint32_t count,
          const char* text, uint32_t text_length, RUIOutput* out) {
    if (out) *out = {};
    if (!g || !g->recording || !g->ui_ready || !input) return;
    if (count > 0 && !cmds) return;
    ui_frame(g->frames[g->frame].cmd, display_extent(), g->transform, *input, cmds, count,
             text ? text : "", text ? text_length : 0, out);
    g->scene_state_bound = false; // ImGui bound its own pipeline, sets and viewport
}

void r_end_frame(void) {
    if (!g || !g->recording) return;
    g->recording = false;

    FrameData& f = g->frames[g->frame];
    vkCmdEndRendering(f.cmd);

    const bool captured = g->capture_requested && record_capture(f.cmd);
    if (captured) g->capture_requested = false;
    if (!captured) {
        image_barrier(f.cmd, g->images[g->image], color_mips(0, 1),
                      VK_IMAGE_LAYOUT_COLOR_ATTACHMENT_OPTIMAL, VK_IMAGE_LAYOUT_PRESENT_SRC_KHR,
                      VK_PIPELINE_STAGE_2_COLOR_ATTACHMENT_OUTPUT_BIT, VK_ACCESS_2_COLOR_ATTACHMENT_WRITE_BIT,
                      VK_PIPELINE_STAGE_2_BOTTOM_OF_PIPE_BIT, 0);
    }
    vkEndCommandBuffer(f.cmd);

    VkSemaphore render_done = g->render_done[g->image];

    VkSemaphoreSubmitInfo wait{VK_STRUCTURE_TYPE_SEMAPHORE_SUBMIT_INFO};
    wait.semaphore = f.image_acquired;
    wait.stageMask = VK_PIPELINE_STAGE_2_COLOR_ATTACHMENT_OUTPUT_BIT;
    VkSemaphoreSubmitInfo signal{VK_STRUCTURE_TYPE_SEMAPHORE_SUBMIT_INFO};
    signal.semaphore = render_done;
    signal.stageMask = VK_PIPELINE_STAGE_2_ALL_GRAPHICS_BIT;
    VkCommandBufferSubmitInfo cmd_info{VK_STRUCTURE_TYPE_COMMAND_BUFFER_SUBMIT_INFO};
    cmd_info.commandBuffer = f.cmd;

    VkSubmitInfo2 submit{VK_STRUCTURE_TYPE_SUBMIT_INFO_2};
    submit.waitSemaphoreInfoCount = 1;
    submit.pWaitSemaphoreInfos = &wait;
    submit.commandBufferInfoCount = 1;
    submit.pCommandBufferInfos = &cmd_info;
    submit.signalSemaphoreInfoCount = 1;
    submit.pSignalSemaphoreInfos = &signal;
    VkResult submitted = vkQueueSubmit2(g->queue, 1, &submit, f.in_flight);
    if (submitted != VK_SUCCESS) {
        fail("vkQueueSubmit2 failed: VkResult " + std::to_string(static_cast<int>(submitted)));
    }
    ++g->submitted;

    VkSwapchainKHR swapchain = g->swapchain.swapchain;
    VkPresentInfoKHR present{VK_STRUCTURE_TYPE_PRESENT_INFO_KHR};
    present.waitSemaphoreCount = 1;
    present.pWaitSemaphores = &render_done;
    present.swapchainCount = 1;
    present.pSwapchains = &swapchain;
    present.pImageIndices = &g->image;
    VkResult presented = vkQueuePresentKHR(g->queue, &present);
    if (presented == VK_ERROR_OUT_OF_DATE_KHR || presented == VK_SUBOPTIMAL_KHR) {
        g->swapchain_dirty = true;
    } else if (presented != VK_SUCCESS) {
        fail("vkQueuePresentKHR failed: VkResult " + std::to_string(static_cast<int>(presented)));
    }

    g->frame = (g->frame + 1) % kFramesInFlight;
}

void r_capture_next_frame(void) {
    if (g) g->capture_requested = true;
}

int32_t r_read_capture(uint8_t* rgba, uint32_t capacity, uint32_t* width, uint32_t* height) {
    if (!g || !g->capture_pending) return fail("r_read_capture: no captured frame"), 0;
    if (width) *width = g->capture_width;
    if (height) *height = g->capture_height;
    if (!rgba) return 1;

    const uint64_t pixels = uint64_t{g->capture_width} * g->capture_height;
    if (capacity < pixels * 4) return fail("r_read_capture: buffer too small"), 0;

    vkWaitForFences(g->dev, 1, &g->frames[g->capture_frame].in_flight, VK_TRUE, UINT64_MAX);
    vmaInvalidateAllocation(g->allocator, g->capture_buffer.allocation, 0, VK_WHOLE_SIZE);

    const bool bgra = g->capture_format == VK_FORMAT_B8G8R8A8_SRGB || g->capture_format == VK_FORMAT_B8G8R8A8_UNORM;
    const auto* src = static_cast<const uint8_t*>(g->capture_mapped);
    for (uint64_t i = 0; i < pixels; ++i) {
        const uint8_t* s = src + i * 4;
        uint8_t*       d = rgba + i * 4;
        d[0] = bgra ? s[2] : s[0];
        d[1] = s[1];
        d[2] = bgra ? s[0] : s[2];
        d[3] = 255; // swapchain alpha is meaningless for a screenshot
    }
    g->capture_pending = false;
    return 1;
}

void r_shutdown(void) {
    if (!g) return;
    destroy_all();
    delete g;
    g = nullptr;
}

} // extern "C"
