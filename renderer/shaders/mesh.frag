#version 450

#define MAX_TEXTURES 1024 // must match kMaxTextures in renderer.cpp

// Must match R_DRAW_* in renderer.h.
#define DRAW_FLAT  1u
#define DRAW_SNOW  2u
#define DRAW_UNLIT 4u
#define DRAW_SKY   8u
#define DRAW_NO_SHADOW 16u

layout(set = 0, binding = 0) uniform Frame {
    mat4 view_proj;
    vec4 camera_pos;
    vec4 sun_direction;
    vec4 sun_color;
    vec4 ambient_color;
    vec4 fog_color; // w = density
    mat4 light_view_proj; // world to the sun's shadow map
    vec4 shadow_params;   // x = shadow map texel (world units), y = 1 with shadows, z = 1 / map size
} frame;

// The sun's depth, seen from the sun (see shadow.vert). Compared here
// rather than by the sampler: not every Metal GPU (the iOS Simulator's)
// has comparing samplers.
layout(set = 0, binding = 1) uniform sampler2D shadow_map;

// Bindless texture table; slot 0 is white.
layout(set = 1, binding = 0) uniform sampler2D textures[MAX_TEXTURES];

layout(push_constant) uniform Push {
    mat4 model;
    vec4 color;
    uint texture_index;
    uint flags;
} pc;

layout(location = 0) in vec3 v_world_pos;
layout(location = 1) in vec3 v_normal;
layout(location = 2) in vec2 v_uv;

layout(location = 0) out vec4 out_color;

// The haze colour looking along dir: the fog colour, warmed towards the sun.
vec3 haze(vec3 dir, vec3 l) {
    float glow = pow(max(dot(dir, l), 0.0), 6.0);
    return frame.fog_color.rgb + frame.sun_color.rgb * glow * 0.12;
}

// How much of the sun reaches this point: 0 in shadow, 1 in the open, soft
// at the edges (a 3x3 bilinear filter over 4x4 texels, from four gathers).
// Pushed off the surface along its normal by about a texel, so a surface
// doesn't shadow itself. Fades out at the edge of the map, beyond which
// nothing is shadowed.
float sunlight(vec3 n) {
    if (frame.shadow_params.y == 0.0) return 1.0;
    vec3 p = v_world_pos + n * (frame.shadow_params.x * 1.5);
    vec4 c = frame.light_view_proj * vec4(p, 1.0);
    float edge = max(abs(c.x), abs(c.y));
    if (edge >= 1.0 || c.z <= 0.0 || c.z >= 1.0) return 1.0;
    float t = frame.shadow_params.z;
    vec2 st = (c.xy * 0.5 + 0.5) / t - 0.5;
    vec2 base = floor(st);
    vec2 f = st - base;
    // Lit (1) or not (0) for texels base-1 .. base+2 on each axis, weighted
    // by how far st is across the middle one.
    vec4 wx = vec4(1.0 - f.x, 1.0, 1.0, f.x);
    vec4 wy = vec4(1.0 - f.y, 1.0, 1.0, f.y);
    float lit = 0.0;
    for (int j = 0; j < 2; ++j) {
        for (int i = 0; i < 2; ++i) {
            // Texels (base + 2i - 1 .. +1, base + 2j - 1 .. +1); gather order
            // is (0,1) (1,1) (1,0) (0,0).
            vec2 at = (base + vec2(2 * i, 2 * j)) * t;
            vec4 d = step(vec4(c.z), textureGather(shadow_map, at, 0));
            lit += d.w * wx[2 * i] * wy[2 * j] + d.z * wx[2 * i + 1] * wy[2 * j] +
                   d.x * wx[2 * i] * wy[2 * j + 1] + d.y * wx[2 * i + 1] * wy[2 * j + 1];
        }
    }
    return mix(lit / 9.0, 1.0, smoothstep(0.8, 1.0, edge));
}

void main() {
    vec3 l = normalize(frame.sun_direction.xyz);
    vec3 to_frag = v_world_pos - frame.camera_pos.xyz;
    float dist = length(to_frag);
    vec3 dir = to_frag / max(dist, 1e-4);

    if ((pc.flags & DRAW_SKY) != 0u) {
        // Horizon haze fading to the draw colour overhead, plus a soft sun.
        float up = clamp(dir.y, 0.0, 1.0);
        vec3 sky = mix(haze(dir, l), pc.color.rgb, pow(up, 0.55));
        float s = max(dot(dir, l), 0.0);
        sky += frame.sun_color.rgb * (pow(s, 900.0) * 3.0 + pow(s, 24.0) * 0.08);
        out_color = vec4(sky, 1.0);
        return;
    }

    vec4 base = texture(textures[pc.texture_index], v_uv) * pc.color;

    vec3 n;
    if ((pc.flags & DRAW_FLAT) != 0u) {
        // One normal per triangle: the plane through neighbouring pixels.
        n = normalize(cross(dFdx(v_world_pos), dFdy(v_world_pos)));
        if (dot(n, dir) > 0.0) n = -n; // face the camera whatever the winding / Y flip
    } else {
        n = normalize(v_normal);
    }

    if ((pc.flags & DRAW_SNOW) != 0u) {
        // Snow settles on faces that point up; steep faces show cool, dark rock.
        vec3 rock = base.rgb * vec3(0.30, 0.29, 0.34);
        base.rgb = mix(rock, base.rgb, smoothstep(0.55, 0.75, n.y));
    }

    vec3 lit;
    if ((pc.flags & DRAW_UNLIT) != 0u) {
        lit = base.rgb;
    } else {
        vec3 v = -dir;
        vec3 h = normalize(l + v);
        float ndl = max(dot(n, l), 0.0);
        float sun = ndl > 0.0 ? sunlight(n) : 0.0;
        float spec = ndl > 0.0 ? pow(max(dot(n, h), 0.0), 64.0) * 0.2 * sun : 0.0;
        ndl *= sun;
        // Hemisphere ambient: full strength facing up, half facing down.
        vec3 ambient = frame.ambient_color.rgb * mix(0.5, 1.0, n.y * 0.5 + 0.5);
        lit = base.rgb * (ambient + frame.sun_color.rgb * ndl) + frame.sun_color.rgb * spec;
    }

    // Aerial perspective: distant surfaces dissolve into the haze.
    float density = frame.fog_color.w;
    if (density > 0.0) {
        float fog = 1.0 - exp(-dist * density);
        lit = mix(lit, haze(dir, l), fog);
    }
    out_color = vec4(lit, base.a);
}
