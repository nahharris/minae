package minui

import (
	"testing"
)

func TestApplyStackLayout_Vertical(t *testing.T) {
	pos := Point{X: 0, Y: 0}
	padding := float32(10)
	spacing := float32(5)
	childrenSizes := []Size{
		{Width: 100, Height: 20},
		{Width: 80, Height: 20},
	}

	rects, totalSize := ApplyStackLayout(
		pos,
		Size{1000, 1000},
		DirectionVertical,
		AlignStart,
		padding,
		spacing,
		childrenSizes,
	)

	// Total height: padding + 20 + spacing + 20 + padding = 10 + 20 + 5 + 20 + 10 = 65
	expectedHeight := float32(65)
	if totalSize.Height != expectedHeight {
		t.Errorf("Expected total height %f, got %f", expectedHeight, totalSize.Height)
	}

	// Max width is 100. Total width: padding + 100 + padding = 120
	expectedWidth := float32(120)
	if totalSize.Width != expectedWidth {
		t.Errorf("Expected total width %f, got %f", expectedWidth, totalSize.Width)
	}

	// Check first rect position
	// X: pos.X + padding = 10
	// Y: pos.Y + padding = 10
	if rects[0].X != 10 || rects[0].Y != 10 {
		t.Errorf("Expected first rect at 10,10, got %f,%f", rects[0].X, rects[0].Y)
	}

	// Check second rect position
	// X: 10 (AlignStart)
	// Y: 10 + 20 (height) + 5 (spacing) = 35
	if rects[1].X != 10 || rects[1].Y != 35 {
		t.Errorf("Expected second rect at 10,35, got %f,%f", rects[1].X, rects[1].Y)
	}
}

func TestApplyStackLayout_Horizontal_Center(t *testing.T) {
	pos := Point{X: 0, Y: 0}
	padding := float32(10)
	spacing := float32(5)
	childrenSizes := []Size{
		{Width: 50, Height: 20}, // Taller
		{Width: 50, Height: 10}, // Shorter
	}

	rects, totalSize := ApplyStackLayout(
		pos,
		Size{1000, 1000},
		DirectionHorizontal,
		AlignCenter,
		padding,
		spacing,
		childrenSizes,
	)

	// Max height is 20. Container height = 10 + 20 + 10 = 40
	if totalSize.Height != 40 {
		t.Errorf("Expected height 40, got %f", totalSize.Height)
	}

	// First rect (Taller, 20) should be centered vertically within max height (20)
	// Y = 10 + (20-20)/2 = 10
	if rects[0].Y != 10 {
		t.Errorf("Expected first rect Y 10, got %f", rects[0].Y)
	}

	// Second rect (Shorter, 10) should be centered vertically within max height (20)
	// Y = 10 + (20-10)/2 = 10 + 5 = 15
	if rects[1].Y != 15 {
		t.Errorf("Expected second rect Y 15, got %f", rects[1].Y)
	}
}

