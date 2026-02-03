package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

// New creates a new logger with the specified level and format
func New(level, format string) *logrus.Logger {
	log := logrus.New()
	log.SetOutput(os.Stdout)

	// Set level
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		lvl = logrus.InfoLevel
	}
	log.SetLevel(lvl)

	// Set format
	if format == "json" {
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	} else {
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}

	return log
}
