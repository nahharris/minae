package config

const (
	GameName    = "Minae"
	GameVersion = "0.1.0"
	ChunkWidth  = 16
	ChunkHeight = 256
)

type GameConfig struct {
	ScreenWidth  int
	ScreenHeight int
	TargetFPS    int
	FOV          float32
	PlayerSpeed  float32
	MouseSens    float32
}

var Current = GameConfig{
	ScreenWidth:  1280,
	ScreenHeight: 720,
	TargetFPS:    60,
	FOV:          60.0,
	PlayerSpeed:  10.0,
	MouseSens:    0.003,
}

