package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func NewLogger(logFilename string, level zapcore.Level, stdout bool) *zap.Logger {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.FullCallerEncoder,
	}

	hook := lumberjack.Logger{
		Filename:   logFilename,
		MaxSize:    512, //MB
		MaxBackups: 1000,
		MaxAge:     70,
		Compress:   true,
	}

	atomicLevel := zap.NewAtomicLevel()
	atomicLevel.SetLevel(level)

	writeSyncer := []zapcore.WriteSyncer{
		zapcore.AddSync(&hook),
	}

	if stdout {
		writeSyncer = append(writeSyncer, zapcore.Lock(os.Stdout))
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.NewMultiWriteSyncer(writeSyncer...),
		atomicLevel,
	)

	logger := zap.New(core)

	return logger
}
