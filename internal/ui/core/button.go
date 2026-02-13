package minui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Button is a clickable component.
type Button struct {
	Text    string
	OnClick func()
	Width   float32
	Height  float32

	// Styles
	NormalColor rl.Color
	HoverColor  rl.Color
	TextColor   rl.Color

	bounds    Rect
	isHovered bool
}

func NewButton(text string, onClick func()) *Button {
	return &Button{
		Text:        text,
		OnClick:     onClick,
		Width:       160,
		Height:      40,
		NormalColor: rl.DarkGray,
		HoverColor:  rl.Gray,
		TextColor:   rl.White,
	}
}

func (b *Button) ComputeSize(available Size) Size {
	return Size{Width: b.Width, Height: b.Height}
}

func (b *Button) SetBounds(rect Rect) {
	b.bounds = rect
}

func (b *Button) Update() {
	mousePos := rl.GetMousePosition()
	b.isHovered = rl.CheckCollisionPointRec(mousePos, b.bounds.ToRaylib())

	if b.isHovered && rl.IsMouseButtonReleased(rl.MouseLeftButton) {
		if b.OnClick != nil {
			b.OnClick()
		}
	}
}

func (b *Button) Draw() {
	color := b.NormalColor
	if b.isHovered {
		color = b.HoverColor
	}

	rl.DrawRectangleRec(b.bounds.ToRaylib(), color)

	// Center text
	fontSize := int32(20)
	textWidth := rl.MeasureText(b.Text, fontSize)
	textX := int32(b.bounds.X) + (int32(b.bounds.Width)-textWidth)/2
	textY := int32(b.bounds.Y) + (int32(b.bounds.Height)-int32(fontSize))/2

	rl.DrawText(b.Text, textX, textY, fontSize, b.TextColor)
}
