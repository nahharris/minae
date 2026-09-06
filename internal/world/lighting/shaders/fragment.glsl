#version 330

in vec2 fragTexCoord;
in vec4 fragColor;
in vec3 fragNormal;

out vec4 finalColor;

uniform sampler2D texture0;

// skyTint is the colour of daylight right now: deep blue at midnight, orange
// at sunrise, warm white at noon. It multiplies baked skylight only, so
// enclosed spaces correctly ignore the time of day.
uniform vec3 skyTint;

// blockTint is the colour of light emitted by blocks (torches, glowstone).
// Reserved: nothing writes the block-light channel yet, so this currently has
// no effect.
uniform vec3 blockTint;

// minAmbient keeps a sealed cave readable rather than pure black. Purely a
// playability floor, not a physical term.
uniform float minAmbient;

// Per-face brightness. Voxel faces are axis-aligned, so a constant step per
// orientation is what gives the world its sense of depth. This replaces the
// old N-dot-L term, which produced no light on top faces at any hour because
// the sun vector was horizontal.
//
// It lives here rather than baked into vertex colours so it stays tunable
// without regenerating every chunk mesh.
float faceBias(vec3 n)
{
    if (n.y > 0.5)  return 1.00;   // top
    if (n.y < -0.5) return 0.50;   // bottom
    if (abs(n.x) > 0.5) return 0.80;   // +X / -X
    return 0.60;                       // +Z / -Z
}

void main()
{
    vec4 texelColor = texture(texture0, fragTexCoord);

    // Vertex colour is a packed light payload, not a tint:
    //   r = skylight 0..1, g = block light 0..1, b = ambient occlusion 0..1.
    // Alpha is opacity and is deliberately NOT light. Encoding light in alpha
    // is what made dark blocks render see-through instead of dark.
    float sky   = fragColor.r;
    float block = fragColor.g;
    float ao    = fragColor.b;

    vec3 light = max(skyTint * sky, blockTint * block);
    light *= faceBias(normalize(fragNormal)) * ao;
    light = max(light, vec3(minAmbient));

    finalColor = vec4(texelColor.rgb * light, texelColor.a * fragColor.a);
}
