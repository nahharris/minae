#version 330

in vec3 vertexPosition;
in vec2 vertexTexCoord;
in vec3 vertexNormal;
in vec4 vertexColor;

out vec2 fragTexCoord;
out vec4 fragColor;
out vec3 fragNormal;

uniform mat4 mvp;

void main()
{
    fragTexCoord = vertexTexCoord;
    fragColor = vertexColor;

    // Chunk model matrices are pure translation, so the normal survives the
    // transform unchanged. The usual transpose(inverse(matModel)) is exact but
    // costs a matrix inverse per vertex, every frame, to compute the identity.
    fragNormal = vertexNormal;

    gl_Position = mvp * vec4(vertexPosition, 1.0);
}
