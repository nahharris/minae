package minui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Label is a text component.
type Label struct {
	TextProvider func() string
	FontSize     int
	Color        rl.Color
	bounds       Rect
}

// NewLabel creates a new label with a static text.
func NewLabel(text string) *Label {
	return &Label{
		TextProvider: func() string { return text },
		FontSize:     20,
		Color:        rl.White,
	}
}

// NewReactiveLabel creates a new label with dynamic text.
func NewReactiveLabel(provider func() string) *Label {
	return &Label{
		TextProvider: provider,
		FontSize:     20,
		Color:        rl.White,
	}
}

func (l *Label) ComputeSize(available Size) Size {
	text := l.TextProvider()
	width := rl.MeasureText(text, int32(l.FontSize))
	return Size{Width: float32(width), Height: float32(l.FontSize)}
}

func (l *Label) SetBounds(rect Rect) {
	l.bounds = rect
}

func (l *Label) Draw() {
	rl.DrawText(l.TextProvider(), int32(l.bounds.X), int32(l.bounds.Y), int32(l.FontSize), l.Color)
}

func (l *Label) Update() {
	// Label has no update logic
}

