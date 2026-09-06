// Package raylog bridges raylib's trace log into the project's structured logger.
//
// It exists as a separate package so that internal/platform/logging, which is
// imported almost everywhere, stays free of the cgo raylib dependency.
package raylog

import (
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/internal/platform/logging"
)

// Init hooks raylib's trace log into our structured logger.
// Must be called after logging.Init() and before rl.InitWindow().
func Init() {
	rl.SetTraceLogCallback(func(level int, msg string) {
		entry := logging.ForPackage("raylib").WithField("rl_level", level)
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
