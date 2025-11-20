package main

import (
	"fmt"
	"runtime"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/player"
	"github.com/nahharris/minae/pkg/world"
)

func init() {
	// Lock the main thread for OpenGL (Raylib requirement)
	runtime.LockOSThread()
}

func main() {
	// Initialize Window
	rl.InitWindow(1280, 720, "Minae - Go Minecraft Clone")
	defer rl.CloseWindow()

	rl.SetTargetFPS(60)
	rl.DisableCursor() // Lock cursor for FPS control

	// Initialize Player
	// Start at 0, 40, 0 to be above the ground (since we generated up to y=32)
	p := player.NewPlayer(rl.NewVector3(0, 40, 0))

	// Initialize World
	w := world.NewWorld()
	w.GenerateFixedGrid()

	// Generate Meshes
	chunkMeshes := make(map[world.ChunkCoord]*rl.Mesh)
	chunkMaterials := make(map[world.ChunkCoord]rl.Material)

	for coord, chunk := range w.Chunks {
		mesh := world.GenerateChunkMesh(chunk, w)
		if mesh != nil {
			chunkMeshes[coord] = mesh
			// We need a material to draw the mesh
			mat := rl.LoadMaterialDefault()
			chunkMaterials[coord] = mat
		}
	}

	// Main Game Loop
	for !rl.WindowShouldClose() {
		// Update
		dt := rl.GetFrameTime()
		p.Update(dt)

		// Draw
		rl.BeginDrawing()
		rl.ClearBackground(rl.SkyBlue)

		rl.BeginMode3D(p.Camera)

		// Draw Chunks
		for coord, mesh := range chunkMeshes {
			// Position logic:
			// Chunks are at ChunkX * 16, 0, ChunkZ * 16
			// BUT, my mesh generation used local coordinates (0..15) + global check.
			// Wait, my mesh generation used local coordinates for vertices: `fx, fy, fz := float32(x), float32(y), float32(z)`
			// So the mesh is local to 0,0,0.
			// I need to translate it to the chunk's position.
			
			pos := rl.NewVector3(float32(coord.X*world.ChunkWidth), 0, float32(coord.Z*world.ChunkWidth))
			
			rl.DrawMesh(*mesh, chunkMaterials[coord], rl.MatrixTranslate(pos.X, pos.Y, pos.Z))
            
            // Debug: Draw Chunk Bounds
            rl.DrawCubeWires(rl.Vector3Add(pos, rl.NewVector3(8, 128, 8)), 16, 256, 16, rl.Red)
		}
        
        // Draw Grid for reference
        rl.DrawGrid(100, 1.0)

		rl.EndMode3D()

		// UI / Debug
		rl.DrawFPS(10, 10)
		rl.DrawText(fmt.Sprintf("Pos: %.2f, %.2f, %.2f", p.Camera.Position.X, p.Camera.Position.Y, p.Camera.Position.Z), 10, 40, 20, rl.White)

		rl.EndDrawing()
	}

	// Cleanup Meshes
	for _, mesh := range chunkMeshes {
		rl.UnloadMesh(mesh)
	}
}

