package minui

import rl "github.com/gen2brain/raylib-go/raylib"

// Point represents a 2D point.
type Point struct {
	X, Y float32
}

// Size represents 2D dimensions.
type Size struct {
	Width, Height float32
}

// Rect represents a rectangle.
type Rect struct {
	X, Y, Width, Height float32
}

// ToRaylib converts Rect to rl.Rectangle.
func (r Rect) ToRaylib() rl.Rectangle {
	return rl.Rectangle{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height}
}

// Component is the interface for all UI elements.
type Component interface {
	// ComputeSize calculates the desired size of the component.
	// It can take available size as a hint.
	ComputeSize(available Size) Size

	// SetBounds sets the layout bounds of the component.
	SetBounds(rect Rect)

	// Draw renders the component.
	Draw()

	// Update handles input and updates state.
	Update()
}
