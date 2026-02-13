package atlas

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/internal/blocks"
)

const DefaultTileSize = 16

// UV is a normalized rectangle in texture coordinate space.
type UV struct {
	U0, V0 float32
	U1, V1 float32
}

// Atlas is a runtime-built texture atlas plus UV lookup by texture key.
type Atlas struct {
	Texture  rl.Texture2D
	TileSize int
	uv       map[string]UV
}

// UV returns the UV rectangle for a given texture key.
func (a *Atlas) UV(key string) (UV, bool) {
	if a == nil {
		return UV{}, false
	}
	uv, ok := a.uv[key]
	return uv, ok
}

// Build constructs a texture atlas from all registered block models. It loads
// PNGs from `<dataFolder>/textures/blocks/<key>.png` and falls back to a solid
// color tile when a PNG is missing.
//
// This must be called after the raylib window/context is initialized.
func Build(dataFolder string) (*Atlas, error) {
	return BuildWithTileSize(dataFolder, DefaultTileSize)
}

// BuildWithTileSize is like Build but allows specifying the atlas tile size.
func BuildWithTileSize(dataFolder string, tileSize int) (*Atlas, error) {
	if tileSize <= 0 {
		return nil, fmt.Errorf("invalid tileSize: %d", tileSize)
	}

	keys, fallbackByKey := collectTextureKeysAndFallbacks()
	if len(keys) == 0 {
		return nil, errors.New("no textures referenced by registered block models")
	}

	cols := int(math.Ceil(math.Sqrt(float64(len(keys)))))
	if cols < 1 {
		cols = 1
	}
	rows := int(math.Ceil(float64(len(keys)) / float64(cols)))
	if rows < 1 {
		rows = 1
	}

	atlasW := cols * tileSize
	atlasH := rows * tileSize

	atlasImg := rl.GenImageColor(atlasW, atlasH, color.RGBA{R: 0, G: 0, B: 0, A: 0})
	if atlasImg == nil {
		return nil, errors.New("failed to create atlas image")
	}

	uv := make(map[string]UV, len(keys))

	for i, key := range keys {
		x := (i % cols) * tileSize
		y := (i / cols) * tileSize

		img := loadTileImage(dataFolder, key, tileSize, fallbackByKey[key])
		if img == nil {
			rl.UnloadImage(atlasImg)
			return nil, fmt.Errorf("failed to load/generate tile image for %q", key)
		}

		src := rl.Rectangle{X: 0, Y: 0, Width: float32(img.Width), Height: float32(img.Height)}
		dst := rl.Rectangle{X: float32(x), Y: float32(y), Width: float32(tileSize), Height: float32(tileSize)}
		rl.ImageDraw(atlasImg, img, src, dst, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		rl.UnloadImage(img)

		// Use a half-texel inset on each side to avoid sampling bleeding between tiles.
		halfU := float32(0.5) / float32(atlasW)
		halfV := float32(0.5) / float32(atlasH)

		uv[key] = UV{
			U0: float32(x)/float32(atlasW) + halfU,
			V0: float32(y)/float32(atlasH) + halfV,
			U1: float32(x+tileSize)/float32(atlasW) - halfU,
			V1: float32(y+tileSize)/float32(atlasH) - halfV,
		}
	}

	tex := rl.LoadTextureFromImage(atlasImg)
	rl.UnloadImage(atlasImg)

	if tex.ID == 0 {
		return nil, errors.New("failed to create atlas texture")
	}

	rl.SetTextureFilter(tex, rl.FilterPoint)
	rl.SetTextureWrap(tex, rl.WrapClamp)

	return &Atlas{
		Texture:  tex,
		TileSize: tileSize,
		uv:       uv,
	}, nil
}

func collectTextureKeysAndFallbacks() ([]string, map[string]color.RGBA) {
	allBlocks := blocks.GetAll()

	byID := make(map[string]color.RGBA, len(allBlocks))
	for _, b := range allBlocks {
		if b == nil {
			continue
		}
		c := rl.GetColor(uint(b.Color))
		byID[b.ID] = color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A}
	}

	set := make(map[string]struct{})
	fallback := make(map[string]color.RGBA)

	for _, b := range allBlocks {
		if b == nil || b.Model == nil {
			continue
		}

		ownerColor := byID[b.ID]
		for _, key := range b.Model.Textures() {
			if key == "" {
				continue
			}
			if _, ok := set[key]; ok {
				continue
			}
			set[key] = struct{}{}

			if c, ok := byID[key]; ok {
				fallback[key] = c
			} else {
				fallback[key] = ownerColor
			}
		}
	}

	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys, fallback
}

func loadTileImage(dataFolder, key string, tileSize int, fallback color.RGBA) *rl.Image {
	path := texturePathForKey(dataFolder, key)

	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		img := rl.LoadImage(path)
		if img == nil {
			return rl.GenImageColor(tileSize, tileSize, fallback)
		}
		if img.Width != int32(tileSize) || img.Height != int32(tileSize) {
			rl.ImageResizeNN(img, int32(tileSize), int32(tileSize))
		}
		return img
	}

	return rl.GenImageColor(tileSize, tileSize, fallback)
}

func texturePathForKey(dataFolder, key string) string {
	keyParts := strings.Split(key, "/")
	// key is expected to be "namespace/name" (potentially with deeper paths).
	// Directories are all but the last segment; the filename is the last segment + ".png".
	if len(keyParts) > 0 {
		keyParts = keyParts[:len(keyParts)-1]
	}
	parts := append([]string{dataFolder, "textures", "blocks"}, keyParts...)
	return filepath.Join(append(parts, keyFileName(key))...)
}

func keyFileName(key string) string {
	// Keep filenames stable: last segment of key + ".png"
	// Example: "minae/grass_top" -> "grass_top.png"
	last := key
	if idx := strings.LastIndexByte(key, '/'); idx >= 0 {
		last = key[idx+1:]
	}
	return last + ".png"
}
