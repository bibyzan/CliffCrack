// Vulkan 1.3 renderer: dynamic rendering + synchronization2, no render passes.
#include "vk_common.h"

#include <VkBootstrap.h>

#include "renderer.h"

#include <cstddef>
#include <cstdio>
#include <cstring>
#include <fstream>
#include <string>
#include <vector>

void* platform_module_handle(); // platform_win32.cpp

namespace {

constexpr uint32_t kFramesInFlight = 2;
constexpr VkFormat kDepthFormat = VK_FORMAT_D32_SFLOAT;
constexpr uint32_t kPushConstantSize = offsetof(RDrawCmd, mesh); // mvp + normal_matrix + color
static_assert(kPushConstantSize == 128, "push constants must fit the 128-byte guaranteed minimum");
static_assert(sizeof(RVertex) == 32, "RVertex layout changed; update the pipeline vertex input");

struct FrameData {
    VkCommandPool   pool = VK_NULL_HANDLE;
    VkCommandBuffer cmd = VK_NULL_HANDLE;
    VkSemaphore     image_acquired = VK_NULL_HANDLE;
    VkFence         in_flight = VK_NULL_HANDLE;
};

struct Buffer {
    VkBuffer      buffer = VK_NULL_HANDLE;
    VmaAllocation allocation = VK_NULL_HANDLE;
};

struct Mesh {
    Buffer   vertices;
    Buffer   indices;
    uint32_t index_count = 0; // 0 = free slot
};

struct Image {
    VkImage       image = VK_NULL_HANDLE;
    VmaAllocation allocation = VK_NULL_HANDLE;
    VkImageView   view = VK_NULL_HANDLE;
};

struct Renderer {
    vkb::Instance instance;
    VkSurfaceKHR  surface = VK_NULL_HANDLE;
    vkb::Device   device;
    VkDevice      dev = VK_NULL_HANDLE;
    VkQueue       queue = VK_NULL_HANDLE;
    uint32_t      queue_family = 0;
    VmaAllocator  allocator = VK_NULL_HANDLE;

    vkb::Swapchain           swapchain;
    std::vector<VkImage>     images;
    std::vector<VkImageView> views;
    std::vector<VkSemaphore> render_done; // one per swapchain image
    bool                     swapchain_dirty = false;
    Image                    depth;       // shared by all frames; barriers serialize use

    // Blocking uploads (mesh creation) use their own command buffer + fence.
    VkCommandPool   upload_pool = VK_NULL_HANDLE;
    VkCommandBuffer upload_cmd = VK_NULL_HANDLE;
    VkFence         upload_fence = VK_NULL_HANDLE;

    std::vector<Mesh>     meshes;     // RMesh handle = index + 1
    std::vector<uint32_t> free_meshes;

    std::string      shader_dir;
    VkFormat         pipeline_format = VK_FORMAT_UNDEFINED;
    VkPipelineLayout pipeline_layout = VK_NULL_HANDLE;
    VkPipeline       pipeline = VK_NULL_HANDLE;

    FrameData frames[kFramesInFlight];
    uint32_t  frame = 0;
    uint32_t  image = 0;
    bool      recording = false;

    uint32_t width = 0;
    uint32_t height = 0;
    bool     vsync = true;
};

Renderer*   g = nullptr;
std::string g_error;

bool fail(std::string msg) {
    g_error = std::move(msg);
    std::fprintf(stderr, "[renderer] %s\n", g_error.c_str());
    return false;
}

#define VK_TRY(expr)                                                                   \
    do {                                                                               \
        VkResult vk_try_result_ = (expr);                                              \
        if (vk_try_result_ != VK_SUCCESS)                                              \
            return fail(std::string(#expr) + " failed: VkResult " +                    \
                        std::to_string(static_cast<int>(vk_try_result_)));              \
    } while (0)

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
    if (g->pipeline_layout) vkDestroyPipelineLayout(g->dev, g->pipeline_layout, nullptr);
    g->pipeline = VK_NULL_HANDLE;
    g->pipeline_layout = VK_NULL_HANDLE;
}

bool create_pipeline(VkFormat color_format) {
    destroy_pipeline();

    VkPushConstantRange push{};
    push.stageFlags = VK_SHADER_STAGE_VERTEX_BIT;
    push.size = kPushConstantSize;

    VkPipelineLayoutCreateInfo layout_info{VK_STRUCTURE_TYPE_PIPELINE_LAYOUT_CREATE_INFO};
    layout_info.pushConstantRangeCount = 1;
    layout_info.pPushConstantRanges = &push;
    VK_TRY(vkCreatePipelineLayout(g->dev, &layout_info, nullptr, &g->pipeline_layout));

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
    // uv (offset 24) stays in the vertex layout but isn't bound until textures land.
    const VkVertexInputAttributeDescription vertex_attributes[] = {
        {0, 0, VK_FORMAT_R32G32B32_SFLOAT, offsetof(RVertex, position)},
        {1, 0, VK_FORMAT_R32G32B32_SFLOAT, offsetof(RVertex, normal)},
    };
    VkPipelineVertexInputStateCreateInfo vertex_input{VK_STRUCTURE_TYPE_PIPELINE_VERTEX_INPUT_STATE_CREATE_INFO};
    vertex_input.vertexBindingDescriptionCount = 1;
    vertex_input.pVertexBindingDescriptions = &vertex_binding;
    vertex_input.vertexAttributeDescriptionCount = 2;
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

void destroy_image(Image& img) {
    if (img.view) vkDestroyImageView(g->dev, img.view, nullptr);
    if (img.image) vmaDestroyImage(g->allocator, img.image, img.allocation);
    img = {};
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
    void*  mapped = nullptr;
    if (!create_buffer(size, VK_BUFFER_USAGE_TRANSFER_SRC_BIT,
                       VMA_ALLOCATION_CREATE_HOST_ACCESS_SEQUENTIAL_WRITE_BIT | VMA_ALLOCATION_CREATE_MAPPED_BIT,
                       staging, &mapped)) {
        return false;
    }
    std::memcpy(mapped, data, static_cast<size_t>(size));
    vmaFlushAllocation(g->allocator, staging.allocation, 0, VK_WHOLE_SIZE);

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

void destroy_swapchain_resources() {
    for (VkSemaphore s : g->render_done) vkDestroySemaphore(g->dev, s, nullptr);
    g->render_done.clear();
    if (!g->views.empty()) g->swapchain.destroy_image_views(g->views);
    g->views.clear();
    g->images.clear();
}

bool create_swapchain() {
    vkb::SwapchainBuilder builder{g->device, g->surface};
    auto built = builder
                     .set_desired_format({VK_FORMAT_B8G8R8A8_SRGB, VK_COLOR_SPACE_SRGB_NONLINEAR_KHR})
                     .set_desired_present_mode(g->vsync ? VK_PRESENT_MODE_FIFO_KHR : VK_PRESENT_MODE_MAILBOX_KHR)
                     .set_desired_extent(g->width, g->height)
                     .set_old_swapchain(g->swapchain)
                     .build();
    if (!built.has_value()) return fail("swapchain creation failed: " + built.error().message());

    destroy_swapchain_resources();
    vkb::destroy_swapchain(g->swapchain); // no-op on the first call
    g->swapchain = built.value();

    std::printf("[renderer] swapchain %ux%u format %d colorspace %d present mode %d\n",
                g->swapchain.extent.width, g->swapchain.extent.height, static_cast<int>(g->swapchain.image_format),
                static_cast<int>(g->swapchain.color_space), static_cast<int>(g->swapchain.present_mode));

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

void image_barrier(VkCommandBuffer cmd, VkImage image, VkImageAspectFlags aspect,
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
    barrier.subresourceRange = {aspect, 0, 1, 0, 1};

    VkDependencyInfo dep{VK_STRUCTURE_TYPE_DEPENDENCY_INFO};
    dep.imageMemoryBarrierCount = 1;
    dep.pImageMemoryBarriers = &barrier;
    vkCmdPipelineBarrier2(cmd, &dep);
}

bool init(const RInitDesc& desc) {
    VK_TRY(volkInitialize());

    vkb::InstanceBuilder inst_builder(vkGetInstanceProcAddr);
    inst_builder.set_app_name("vkgame").require_api_version(1, 3, 0);
    if (desc.enable_validation) {
        inst_builder.request_validation_layers().use_default_debug_messenger();
    }
    auto inst = inst_builder.build();
    if (!inst.has_value()) return fail("instance creation failed: " + inst.error().message());
    g->instance = inst.value();
    volkLoadInstance(g->instance.instance);

    VkWin32SurfaceCreateInfoKHR surface_info{VK_STRUCTURE_TYPE_WIN32_SURFACE_CREATE_INFO_KHR};
    surface_info.hinstance = static_cast<HINSTANCE>(platform_module_handle());
    surface_info.hwnd = static_cast<HWND>(desc.native_window);
    VK_TRY(vkCreateWin32SurfaceKHR(g->instance.instance, &surface_info, nullptr, &g->surface));

    VkPhysicalDeviceVulkan13Features features13{VK_STRUCTURE_TYPE_PHYSICAL_DEVICE_VULKAN_1_3_FEATURES};
    features13.dynamicRendering = VK_TRUE;
    features13.synchronization2 = VK_TRUE;

    auto phys = vkb::PhysicalDeviceSelector(g->instance, g->surface)
                    .set_minimum_version(1, 3)
                    .set_required_features_13(features13)
                    .select();
    if (!phys.has_value()) return fail("no suitable GPU: " + phys.error().message());
    std::printf("[renderer] GPU: %s\n", phys.value().name.c_str());

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
    return create_swapchain(); // also builds the pipeline for the swapchain format
}

void destroy_all() {
    if (g->dev) {
        vkDeviceWaitIdle(g->dev);
        for (FrameData& f : g->frames) {
            if (f.in_flight) vkDestroyFence(g->dev, f.in_flight, nullptr);
            if (f.image_acquired) vkDestroySemaphore(g->dev, f.image_acquired, nullptr);
            if (f.pool) vkDestroyCommandPool(g->dev, f.pool, nullptr);
        }
        if (g->upload_fence) vkDestroyFence(g->dev, g->upload_fence, nullptr);
        if (g->upload_pool) vkDestroyCommandPool(g->dev, g->upload_pool, nullptr);
        destroy_pipeline();
        destroy_swapchain_resources();
        vkb::destroy_swapchain(g->swapchain);
        if (g->allocator) {
            for (Mesh& mesh : g->meshes) destroy_mesh(mesh);
            destroy_image(g->depth);
            vmaDestroyAllocator(g->allocator);
        }
        vkb::destroy_device(g->device);
    }
    if (g->surface) vkb::destroy_surface(g->instance, g->surface);
    if (g->instance.instance) vkb::destroy_instance(g->instance);
}

} // namespace

extern "C" {

int32_t r_init(const RInitDesc* desc) {
    if (g) return fail("r_init called twice"), 0;
    if (!desc || !desc->native_window) return fail("r_init: missing native window"), 0;

    g = new Renderer();
    g->width = desc->width;
    g->height = desc->height;
    g->vsync = desc->vsync != 0;
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

void r_resize(uint32_t width, uint32_t height) {
    if (!g) return;
    if (width == g->width && height == g->height) return;
    g->width = width;
    g->height = height;
    g->swapchain_dirty = true;
}

int32_t r_begin_frame(const float clear_color[4]) {
    if (!g || g->recording) return 0;
    if (g->width == 0 || g->height == 0) return 0; // minimized

    if (g->swapchain_dirty && !recreate_swapchain()) return 0;

    FrameData& f = g->frames[g->frame];
    vkWaitForFences(g->dev, 1, &f.in_flight, VK_TRUE, UINT64_MAX);

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

    vkResetFences(g->dev, 1, &f.in_flight);
    vkResetCommandPool(g->dev, f.pool, 0);

    VkCommandBufferBeginInfo begin{VK_STRUCTURE_TYPE_COMMAND_BUFFER_BEGIN_INFO};
    begin.flags = VK_COMMAND_BUFFER_USAGE_ONE_TIME_SUBMIT_BIT;
    vkBeginCommandBuffer(f.cmd, &begin);

    image_barrier(f.cmd, g->images[g->image], VK_IMAGE_ASPECT_COLOR_BIT,
                  VK_IMAGE_LAYOUT_UNDEFINED, VK_IMAGE_LAYOUT_COLOR_ATTACHMENT_OPTIMAL,
                  VK_PIPELINE_STAGE_2_COLOR_ATTACHMENT_OUTPUT_BIT, 0,
                  VK_PIPELINE_STAGE_2_COLOR_ATTACHMENT_OUTPUT_BIT, VK_ACCESS_2_COLOR_ATTACHMENT_WRITE_BIT);

    // The depth image is shared by both frames in flight: wait for the previous
    // frame's depth writes before this frame clears it.
    constexpr VkPipelineStageFlags2 kDepthStages =
        VK_PIPELINE_STAGE_2_EARLY_FRAGMENT_TESTS_BIT | VK_PIPELINE_STAGE_2_LATE_FRAGMENT_TESTS_BIT;
    image_barrier(f.cmd, g->depth.image, VK_IMAGE_ASPECT_DEPTH_BIT,
                  VK_IMAGE_LAYOUT_UNDEFINED, VK_IMAGE_LAYOUT_DEPTH_ATTACHMENT_OPTIMAL,
                  kDepthStages, VK_ACCESS_2_DEPTH_STENCIL_ATTACHMENT_WRITE_BIT,
                  kDepthStages, VK_ACCESS_2_DEPTH_STENCIL_ATTACHMENT_READ_BIT |
                                    VK_ACCESS_2_DEPTH_STENCIL_ATTACHMENT_WRITE_BIT);

    VkRenderingAttachmentInfo color{VK_STRUCTURE_TYPE_RENDERING_ATTACHMENT_INFO};
    color.imageView = g->views[g->image];
    color.imageLayout = VK_IMAGE_LAYOUT_COLOR_ATTACHMENT_OPTIMAL;
    color.loadOp = VK_ATTACHMENT_LOAD_OP_CLEAR;
    color.storeOp = VK_ATTACHMENT_STORE_OP_STORE;
    if (clear_color) {
        for (int i = 0; i < 4; ++i) color.clearValue.color.float32[i] = clear_color[i];
    }

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

    VkViewport viewport{0.0f, 0.0f, static_cast<float>(extent.width), static_cast<float>(extent.height), 0.0f, 1.0f};
    VkRect2D scissor{{0, 0}, extent};
    vkCmdSetViewport(f.cmd, 0, 1, &viewport);
    vkCmdSetScissor(f.cmd, 0, 1, &scissor);
    vkCmdBindPipeline(f.cmd, VK_PIPELINE_BIND_POINT_GRAPHICS, g->pipeline);

    g->recording = true;
    return 1;
}

void r_draw(const RDrawCmd* cmds, uint32_t count) {
    if (!g || !g->recording || !cmds) return;
    VkCommandBuffer cmd = g->frames[g->frame].cmd;
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
        vkCmdPushConstants(cmd, g->pipeline_layout, VK_SHADER_STAGE_VERTEX_BIT, 0, kPushConstantSize, &cmds[i]);
        vkCmdDrawIndexed(cmd, mesh->index_count, 1, 0, 0, 0);
    }
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
    vkDeviceWaitIdle(g->dev); // it may still be referenced by frames in flight
    destroy_mesh(*mesh);
    g->free_meshes.push_back(handle - 1);
}

void r_end_frame(void) {
    if (!g || !g->recording) return;
    g->recording = false;

    FrameData& f = g->frames[g->frame];
    vkCmdEndRendering(f.cmd);

    image_barrier(f.cmd, g->images[g->image], VK_IMAGE_ASPECT_COLOR_BIT,
                  VK_IMAGE_LAYOUT_COLOR_ATTACHMENT_OPTIMAL, VK_IMAGE_LAYOUT_PRESENT_SRC_KHR,
                  VK_PIPELINE_STAGE_2_COLOR_ATTACHMENT_OUTPUT_BIT, VK_ACCESS_2_COLOR_ATTACHMENT_WRITE_BIT,
                  VK_PIPELINE_STAGE_2_BOTTOM_OF_PIPE_BIT, 0);
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

void r_shutdown(void) {
    if (!g) return;
    destroy_all();
    delete g;
    g = nullptr;
}

} // extern "C"
