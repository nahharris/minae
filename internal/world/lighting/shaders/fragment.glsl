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

// blockAOStrength is how much of the ambient-occlusion darkening applies to
// block light. 1.0 treats a torch exactly like skylight; 0.0 exempts it
// entirely. Half keeps some corner depth in torch-lit spaces without the
// hard cross a 2x2 pocket produces at full strength.
const float blockAOStrength = 0.5;

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

    // Ambient occlusion applies in full to skylight and at reduced strength to
    // block light.
    //
    // AO approximates how much of the distant sky a corner can see, which is
    // exactly what skylight is. A torch a block away is not occluded by the
    // same geometry in the same way, and applying full AO to it makes small
    // enclosed spaces read badly: in a 2x2 pocket every corner touches two
    // walls and is forced to the darkest level, leaving only the shared centre
    // bright and drawing a hard cross across the floor.
    //
    // Dropping AO from block light entirely would remove that artifact but
    // also flatten every torch-lit cave, so it is halved rather than removed.
    vec3 litBySky   = skyTint * sky * ao;
    vec3 litByBlock = blockTint * block * mix(1.0, ao, blockAOStrength);

    vec3 light = max(litBySky, litByBlock);
    light *= faceBias(normalize(fragNormal));
    light = max(light, vec3(minAmbient));

    finalColor = vec4(texelColor.rgb * light, texelColor.a * fragColor.a);
}
