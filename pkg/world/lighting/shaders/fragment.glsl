#version 330

// Input vertex attributes (from vertex shader)
in vec3 fragPosition;
in vec2 fragTexCoord;
in vec3 fragNormal;
in vec4 fragColor;

// Input uniform values
uniform sampler2D texture0;
uniform vec4 colDiffuse;

// Custom Uniforms
uniform vec3 lightDir;     // Direction TO the light source
uniform vec4 lightColor;   // Color of the sun/moon light
uniform vec4 ambientColor; // Ambient light color
uniform vec3 viewPos;      // Camera position (for specular, unused for now)

// Output fragment color
out vec4 finalColor;

void main()
{
    // Extract Sky Light Level from Vertex Color Alpha (0.0 - 1.0)
    float skyLight = fragColor.a;

    // Texture and Vertex Color
    // We force alpha to 1.0 for modulation to avoid making dark blocks transparent
    vec4 texelColor = texture(texture0, fragTexCoord);
    vec4 objectColor = texelColor * vec4(fragColor.rgb, 1.0) * colDiffuse;

    // Ambient
    vec3 ambient = ambientColor.rgb * objectColor.rgb;

    // Diffuse
    vec3 norm = normalize(fragNormal);
    vec3 lightDirNormalized = normalize(lightDir);
    float diff = max(dot(norm, lightDirNormalized), 0.0);
    
    // Apply Sky Light attenuation to Diffuse component
    vec3 diffuse = diff * lightColor.rgb * objectColor.rgb * skyLight;

    // Result
    vec3 result = ambient + diffuse;

    // Fog (Simple Linear Fog)
    // float dist = length(viewPos - fragPosition);
    // float fogStart = 20.0;
    // float fogEnd = 100.0;
    // float fogFactor = clamp((fogEnd - dist) / (fogEnd - fogStart), 0.0, 1.0);
    // result = mix(vec3(0.5, 0.5, 0.5), result, fogFactor); // Blend with gray/sky color

    finalColor = vec4(result, texelColor.a);
}

