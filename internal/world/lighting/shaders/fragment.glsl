#version 330

in vec2 fragTexCoord;
in vec4 fragColor;
in vec3 fragNormal;
in vec3 fragPosition;

out vec4 finalColor;

uniform sampler2D texture0;
uniform vec3 lightDir;
uniform vec4 lightColor;
uniform vec4 ambientColor;
uniform vec3 viewPos;

void main()
{
    vec4 texelColor = texture(texture0, fragTexCoord);
    
    // Calculate lighting
    vec3 normal = normalize(fragNormal);
    vec3 light = normalize(-lightDir);
    float diff = max(dot(normal, light), 0.0);
    
    vec4 lighting = ambientColor + (lightColor * diff);
    
    // Apply vertex color (contains skylight in alpha)
    vec4 color = texelColor * fragColor;
    
    finalColor = color * lighting;
}
