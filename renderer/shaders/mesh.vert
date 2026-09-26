#version 450

// Must match FrameUniforms in renderer.cpp.
layout(set = 0, binding = 0) uniform Frame {
    mat4 view_proj;
    vec4 camera_pos;
    vec4 sun_direction;
    vec4 sun_color;
    vec4 ambient_color;
    vec4 fog_color;
    mat4 light_view_proj;
    vec4 shadow_params;
} frame;

// Must match the leading fields of RDrawCmd in renderer.h.
layout(push_constant) uniform Push {
    mat4 model;
    vec4 color;
    uint texture_index;
    uint flags;
} pc;

layout(location = 0) in vec3 in_position;
layout(location = 1) in vec3 in_normal;
layout(location = 2) in vec2 in_uv;

layout(location = 0) out vec3 v_world_pos;
layout(location = 1) out vec3 v_normal;
layout(location = 2) out vec2 v_uv;

void main() {
    vec4 world = pc.model * vec4(in_position, 1.0);
    gl_Position = frame.view_proj * world;
    v_world_pos = world.xyz;
    // Inverse-transpose keeps normals perpendicular under non-uniform scale.
    v_normal = transpose(inverse(mat3(pc.model))) * in_normal;
    v_uv = in_uv;
}
