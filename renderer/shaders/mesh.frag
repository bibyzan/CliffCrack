#version 450

#define MAX_TEXTURES 1024 // must match kMaxTextures in renderer.cpp

layout(set = 0, binding = 0) uniform Frame {
    mat4 view_proj;
    vec4 camera_pos;
    vec4 sun_direction;
    vec4 sun_color;
    vec4 ambient_color;
} frame;

// Bindless texture table; slot 0 is white.
layout(set = 1, binding = 0) uniform sampler2D textures[MAX_TEXTURES];

layout(push_constant) uniform Push {
    mat4 model;
    vec4 color;
    uint texture_index;
} pc;

layout(location = 0) in vec3 v_world_pos;
layout(location = 1) in vec3 v_normal;
layout(location = 2) in vec2 v_uv;

layout(location = 0) out vec4 out_color;

void main() {
    vec4 base = texture(textures[pc.texture_index], v_uv) * pc.color;

    vec3 n = normalize(v_normal);
    vec3 l = normalize(frame.sun_direction.xyz);
    vec3 v = normalize(frame.camera_pos.xyz - v_world_pos);
    vec3 h = normalize(l + v);

    float ndl = max(dot(n, l), 0.0);
    float spec = ndl > 0.0 ? pow(max(dot(n, h), 0.0), 64.0) * 0.2 : 0.0;

    // Hemisphere ambient: full strength facing up, half facing down.
    vec3 ambient = frame.ambient_color.rgb * mix(0.5, 1.0, n.y * 0.5 + 0.5);

    vec3 lit = base.rgb * (ambient + frame.sun_color.rgb * ndl) + frame.sun_color.rgb * spec;
    out_color = vec4(lit, base.a);
}
