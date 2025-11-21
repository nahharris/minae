package lighting

var VsCode = `
#version 330

// Input vertex attributes
in vec3 vertexPosition;
in vec2 vertexTexCoord;
in vec3 vertexNormal;
in vec4 vertexColor;

// Input uniform values
uniform mat4 mvp;
uniform mat4 matModel;
uniform mat4 matNormal;

// Output vertex attributes (to fragment shader)
out vec3 fragPosition;
out vec2 fragTexCoord;
out vec3 fragNormal;
out vec4 fragColor;

void main()
{
    // Send vertex attributes to fragment shader
    fragPosition = vec3(matModel * vec4(vertexPosition, 1.0));
    fragTexCoord = vertexTexCoord;
    fragColor = vertexColor;
    
    // Calculate normal in world space
    // Using matNormal (transpose inverse of model matrix) is correct for non-uniform scaling,
    // but usually just matModel rotational part is enough for uniform scaling.
    // Raylib provides matNormal.
    fragNormal = normalize(vec3(matNormal * vec4(vertexNormal, 1.0)));

    // Calculate final vertex position
    gl_Position = mvp * vec4(vertexPosition, 1.0);
}
`

var FsCode = `
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
    // Texture and Vertex Color
    vec4 texelColor = texture(texture0, fragTexCoord);
    vec4 objectColor = texelColor * fragColor * colDiffuse;

    // Ambient
    vec3 ambient = ambientColor.rgb * objectColor.rgb;

    // Diffuse
    vec3 norm = normalize(fragNormal);
    vec3 lightDirNormalized = normalize(lightDir);
    float diff = max(dot(norm, lightDirNormalized), 0.0);
    vec3 diffuse = diff * lightColor.rgb * objectColor.rgb;

    // Result
    vec3 result = ambient + diffuse;

    // Fog (Simple Linear Fog)
    // float dist = length(viewPos - fragPosition);
    // float fogStart = 20.0;
    // float fogEnd = 100.0;
    // float fogFactor = clamp((fogEnd - dist) / (fogEnd - fogStart), 0.0, 1.0);
    // result = mix(vec3(0.5, 0.5, 0.5), result, fogFactor); // Blend with gray/sky color

    finalColor = vec4(result, objectColor.a);
}
`
