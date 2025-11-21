package minui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Box is a simple colored rectangle component.
type Box struct {
	Color  rl.Color
	Width  float32
	Height float32
	bounds Rect
}

func NewBox(width, height float32, color rl.Color) *Box {
	return &Box{
		Width:  width,
		Height: height,
		Color:  color,
	}
}

func (b *Box) ComputeSize(available Size) Size {
	return Size{Width: b.Width, Height: b.Height}
}

func (b *Box) SetBounds(rect Rect) {
	b.bounds = rect
}

func (b *Box) Draw() {
	rl.DrawRectangleRec(b.bounds.ToRaylib(), b.Color)
}

func (b *Box) Update() {
	// Box has no update logic
}

