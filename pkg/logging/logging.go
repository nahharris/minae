package logging

import (
	"os"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/sirupsen/logrus"
)

var Log *logrus.Logger

// Init initializes the global logger.
func Init(level logrus.Level) {
	Log = logrus.New()
	Log.SetOutput(os.Stdout)
	Log.SetLevel(level)
	Log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
}

// InitRaylibLogger hooks raylib's trace log into our structured logger.
// Must be called after Init() and before rl.InitWindow().
func InitRaylibLogger() {
	rl.SetTraceLogCallback(func(level int, msg string) {
		entry := ForPackage("raylib").WithField("rl_level", level)
		switch rl.TraceLogLevel(level) {
		case rl.LogTrace, rl.LogDebug:
			entry.Debug(msg)
		case rl.LogInfo:
			// Downgrade noisy GPU resource spam to debug.
			if strings.Contains(msg, "VAO:") || strings.Contains(msg, "VBO:") {
				entry.Debug(msg)
				return
			}
			entry.Info(msg)
		case rl.LogWarning:
			entry.Warn(msg)
		case rl.LogError:
			entry.Error(msg)
		case rl.LogFatal:
			entry.Fatal(msg)
		default:
			entry.Info(msg)
		}
	})
}

// ForPackage returns a logger entry with the package field set.
// It initializes the logger with InfoLevel if it hasn't been initialized yet.
func ForPackage(name string) *logrus.Entry {
	if Log == nil {
		Init(logrus.InfoLevel)
	}
	return Log.WithField("pkg", name)
}
