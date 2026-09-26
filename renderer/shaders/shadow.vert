#version 450

// The sun's shadow map: depth only, seen from the sun (see mesh.frag).

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

layout(push_constant) uniform Push {
    mat4 model;
    vec4 color;
    uint texture_index;
    uint flags;
} pc;

layout(location = 0) in vec3 in_position;

void main() {
    gl_Position = frame.light_view_proj * (pc.model * vec4(in_position, 1.0));
}
