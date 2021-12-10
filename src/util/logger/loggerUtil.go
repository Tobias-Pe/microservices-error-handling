package logger

import (
	"github.com/sirupsen/logrus"
	"os"
)

func InitLogger() *logrus.Logger {
	logger := logrus.New()
	logger.Out = os.Stdout
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		ForceColors:      true,
		DisableColors:    false,
		DisableTimestamp: false,
		FullTimestamp:    true,
		TimestampFormat:  "15:04:05",
		DisableSorting:   false,
		SortingFunc:      nil,
		PadLevelText:     true,
		QuoteEmptyFields: true,
	})
	return logger
}
