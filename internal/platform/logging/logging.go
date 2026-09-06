package logging

import (
	"os"

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

// ForPackage returns a logger entry with the package field set.
// It initializes the logger with InfoLevel if it hasn't been initialized yet.
func ForPackage(name string) *logrus.Entry {
	if Log == nil {
		Init(logrus.InfoLevel)
	}
	return Log.WithField("pkg", name)
}
