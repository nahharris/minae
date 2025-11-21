package minui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Panel is a container that stacks its children.
type Panel struct {
	Children        []Component
	Direction       LayoutDirection
	Alignment       Alignment
	Padding         float32
	Spacing         float32
	BackgroundColor rl.Color
	
	// If FixedSize is set > 0, it forces that size. Otherwise fits content.
	FixedSize Size
	
	bounds Rect
}

func NewPanel() *Panel {
	return &Panel{
		Children:        make([]Component, 0),
		Direction:       DirectionVertical,
		Alignment:       AlignStart,
		Padding:         10,
		Spacing:         10,
		BackgroundColor: rl.Blank,
	}
}

func (p *Panel) AddChild(c Component) {
	p.Children = append(p.Children, c)
}

func (p *Panel) ComputeSize(available Size) Size {
	// Compute sizes for all children
	childSizes := make([]Size, len(p.Children))
	for i, c := range p.Children {
		childSizes[i] = c.ComputeSize(available)
	}

	// Use layout logic to determine required size
	// passing dummy pos, as we only care about total size here
	_, size := ApplyStackLayout(
		Point{0, 0}, 
		available, 
		p.Direction, 
		p.Alignment, 
		p.Padding, 
		p.Spacing, 
		childSizes,
	)

	if p.FixedSize.Width > 0 {
		size.Width = p.FixedSize.Width
	}
	if p.FixedSize.Height > 0 {
		size.Height = p.FixedSize.Height
	}

	return size
}

func (p *Panel) SetBounds(rect Rect) {
	p.bounds = rect
	
	// Now we need to layout children within this rect
	childSizes := make([]Size, len(p.Children))
	for i, c := range p.Children {
		// Re-compute size? Or cache? For now re-compute or we need to store it.
		// In a robust system we'd cache measure.
		// Here we just ask again, assuming it's cheap.
		childSizes[i] = c.ComputeSize(Size{Width: rect.Width, Height: rect.Height})
	}

	childRects, _ := ApplyStackLayout(
		Point{X: rect.X, Y: rect.Y},
		Size{Width: rect.Width, Height: rect.Height},
		p.Direction,
		p.Alignment,
		p.Padding,
		p.Spacing,
		childSizes,
	)

	for i, c := range p.Children {
		if i < len(childRects) {
			c.SetBounds(childRects[i])
		}
	}
}

func (p *Panel) Update() {
	for _, c := range p.Children {
		c.Update()
	}
}

func (p *Panel) Draw() {
	if p.BackgroundColor.A > 0 {
		rl.DrawRectangleRec(p.bounds.ToRaylib(), p.BackgroundColor)
	}
	for _, c := range p.Children {
		c.Draw()
	}
}

