package minui

// LayoutDirection defines how children are stacked.
type LayoutDirection int

const (
	DirectionVertical LayoutDirection = iota
	DirectionHorizontal
)

// Alignment defines how children are aligned within the cross axis.
type Alignment int

const (
	AlignStart Alignment = iota
	AlignCenter
	AlignEnd
)

// ApplyStackLayout calculates positions for children based on direction and alignment.
// This is a pure function for testability, returning the bounds for each child and the total size used.
// It assumes children have already computed their desired sizes.
func ApplyStackLayout(
	pos Point,
	availableSize Size,
	direction LayoutDirection,
	alignment Alignment,
	padding float32,
	spacing float32,
	childrenSizes []Size,
) ([]Rect, Size) {
	count := len(childrenSizes)
	if count == 0 {
		return []Rect{}, Size{Width: padding * 2, Height: padding * 2}
	}

	rects := make([]Rect, count)

	currentX := pos.X + padding
	currentY := pos.Y + padding

	var maxCross float32
	var totalMain float32

	// First pass: Calculate total main axis size and max cross axis size
	for _, size := range childrenSizes {
		if direction == DirectionVertical {
			totalMain += size.Height
			if size.Width > maxCross {
				maxCross = size.Width
			}
		} else {
			totalMain += size.Width
			if size.Height > maxCross {
				maxCross = size.Height
			}
		}
	}

	// Add spacing
	if count > 1 {
		totalMain += float32(count-1) * spacing
	}

	// Determine final container size (at least enough to fit content + padding)
	containerWidth := maxCross + padding*2
	containerHeight := totalMain + padding*2

	if direction == DirectionHorizontal {
		containerWidth = totalMain + padding*2
		containerHeight = maxCross + padding*2
	}

	// Second pass: Position children
	for i, size := range childrenSizes {
		var x, y float32

		if direction == DirectionVertical {
			// Vertical Stack
			y = currentY

			// Alignment on Cross Axis (Horizontal)
			switch alignment {
			case AlignStart:
				x = currentX
			case AlignCenter:
				x = currentX + (maxCross-size.Width)/2
			case AlignEnd:
				x = currentX + (maxCross - size.Width)
			}

			rects[i] = Rect{X: x, Y: y, Width: size.Width, Height: size.Height}
			currentY += size.Height + spacing

		} else {
			// Horizontal Stack
			x = currentX

			// Alignment on Cross Axis (Vertical)
			switch alignment {
			case AlignStart:
				y = currentY
			case AlignCenter:
				y = currentY + (maxCross-size.Height)/2
			case AlignEnd:
				y = currentY + (maxCross - size.Height)
			}

			rects[i] = Rect{X: x, Y: y, Width: size.Width, Height: size.Height}
			currentX += size.Width + spacing
		}
	}

	return rects, Size{Width: containerWidth, Height: containerHeight}
}
