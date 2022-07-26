package utils

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func GetLogger() *zap.Logger {
	loggerConfig := zap.Config{
		Level:            zap.NewAtomicLevelAt(zap.DebugLevel),
		Encoding:         "json",
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey:  "message",
			LevelKey:    "level",
			EncodeLevel: zapcore.LowercaseLevelEncoder,
		},
	}
	logger, err := loggerConfig.Build()
	if err != nil {
		panic(err)
	}
	return logger
}
