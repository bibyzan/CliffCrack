#version 450

// Must match RDrawCmd in renderer.h.
layout(push_constant) uniform Push {
    mat4 mvp;
    vec4 color;
} pc;

layout(location = 0) out vec4 v_color;

// Built-in equilateral triangle centred on the origin, unit edge length.
const vec2 kPositions[3] = vec2[](
    vec2( 0.0,  0.5773503),
    vec2(-0.5, -0.2886751),
    vec2( 0.5, -0.2886751)
);

const vec3 kColors[3] = vec3[](
    vec3(1.0, 0.3, 0.3),
    vec3(0.3, 1.0, 0.3),
    vec3(0.3, 0.3, 1.0)
);

void main() {
    gl_Position = pc.mvp * vec4(kPositions[gl_VertexIndex], 0.0, 1.0);
    v_color = vec4(kColors[gl_VertexIndex] * pc.color.rgb, pc.color.a);
}
