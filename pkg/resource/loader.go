package resource

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/blocks"
	"github.com/nahharris/minae/pkg/config"
	"github.com/nahharris/minae/pkg/logging"
	"github.com/nahharris/minae/pkg/render/atlas"
	"github.com/nahharris/minae/pkg/world/lighting"
	"github.com/sirupsen/logrus"
)

// Resources holds all loaded game resources.
type Resources struct {
	Atlas    *atlas.Atlas
	Shader   rl.Shader
	Material rl.Material
}

// Loader orchestrates resource loading.
type Loader struct {
	DataFolder string
	Log        *logrus.Entry
}

// NewLoader creates a new Loader instance.
func NewLoader(dataFolder string) *Loader {
	return &Loader{
		DataFolder: dataFolder,
		Log:        logging.ForPackage("resource"),
	}
}

// LoadConfig loads the game configuration.
func (l *Loader) LoadConfig() error {
	l.Log.Info("Loading configuration...")
	return config.Load(l.DataFolder)
}

// LoadBlocks loads block definitions.
func (l *Loader) LoadBlocks() error {
	l.Log.Info("Loading blocks...")
	return blocks.Load(l.DataFolder)
}

// LoadRenderResources loads the texture atlas, shaders, and materials.
// Must be called after Raylib window initialization.
func (l *Loader) LoadRenderResources() (*Resources, error) {
	l.Log.Info("Loading render resources...")

	// 1. Load Shader
	shader := rl.LoadShaderFromMemory(lighting.VsCode, lighting.FsCode)
	
	// Set standard shader locations if they aren't automatically set
	// Raylib usually handles standard names, but we verify or set custom ones later in SceneRenderer
	
	// 2. Create Material
	mat := rl.LoadMaterialDefault()
	mat.Shader = shader
	
	// 3. Build Atlas
	var texAtlas *atlas.Atlas
	if l.DataFolder != "" {
		a, err := atlas.Build(l.DataFolder)
		if err != nil {
			l.Log.Warnf("Failed to build texture atlas: %v. Falling back to default textures.", err)
		} else {
			texAtlas = a
			mat.GetMap(int32(rl.MapDiffuse)).Texture = a.Texture
			l.Log.Infof("Texture atlas built with tile size %d", a.TileSize)
		}
	}

	return &Resources{
		Atlas:    texAtlas,
		Shader:   shader,
		Material: mat,
	}, nil
}

// Unload releases all loaded resources.
func (l *Loader) Unload(res *Resources) {
	l.Log.Info("Unloading resources...")
	if res == nil {
		return
	}

	rl.UnloadShader(res.Shader)
	if res.Atlas != nil && res.Atlas.Texture.ID != 0 {
		rl.UnloadTexture(res.Atlas.Texture)
	}
}

