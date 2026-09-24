#version 450

// Must match the first 128 bytes of RDrawCmd in renderer.h.
layout(push_constant) uniform Push {
    mat4 mvp;
    vec4 normal_cols[3]; // world-space normal matrix, one column per vec4 (w unused)
    vec4 color;
} pc;

layout(location = 0) in vec3 in_position;
layout(location = 1) in vec3 in_normal;

layout(location = 0) out vec3 v_normal;
layout(location = 1) out vec4 v_color;

void main() {
    mat3 normal_matrix = mat3(pc.normal_cols[0].xyz, pc.normal_cols[1].xyz, pc.normal_cols[2].xyz);
    gl_Position = pc.mvp * vec4(in_position, 1.0);
    v_normal = normal_matrix * in_normal;
    v_color = pc.color;
}
