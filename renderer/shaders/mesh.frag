#version 450

layout(location = 0) in vec3 v_normal;
layout(location = 1) in vec4 v_color;

layout(location = 0) out vec4 out_color;

// Fixed sun light until the renderer gets per-frame uniforms.
const vec3 kLightDir = normalize(vec3(0.4, 1.0, 0.3)); // direction *towards* the light
const float kAmbient = 0.15;

void main() {
    float diffuse = max(dot(normalize(v_normal), kLightDir), 0.0);
    out_color = vec4(v_color.rgb * (kAmbient + (1.0 - kAmbient) * diffuse), v_color.a);
}
