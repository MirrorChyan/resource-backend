package logger

import (
	"os"
	"path/filepath"

	"github.com/MirrorChyan/resource-backend/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	level = zap.NewAtomicLevelAt(zap.InfoLevel)
)

func SetLevel(l string) {
	level.SetLevel(getLevel(l))
}

func New() *zap.Logger {
	SetLevel(config.CFG.Log.Level)
	config.SetLogLevelChangeListener(SetLevel)
	var (
		encoder = getConsoleEncoder()
		core    = zapcore.NewCore(
			encoder,
			zapcore.AddSync(os.Stdout),
			level,
		)
	)

	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
}

func getLevel(l string) zapcore.Level {
	switch l {
	case "debug":
		return zap.DebugLevel
	case "info":
		return zap.InfoLevel
	case "warn":
		return zap.WarnLevel
	case "error":
		return zap.ErrorLevel
	case "fatal":
		return zap.FatalLevel
	default:
		return zap.InfoLevel
	}
}

func getLumberjackLogger(conf *config.Config) (lumberjack.Logger, error) {
	exePath, err := os.Getwd()
	if err != nil {
		return lumberjack.Logger{}, err
	}
	exeDir := exePath
	logPath := filepath.Join(exeDir, "debug", "log.jsonl")
	return lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    conf.Log.MaxSize,
		MaxBackups: conf.Log.MaxBackups,
		MaxAge:     conf.Log.MaxAge,
		Compress:   conf.Log.Compress,
	}, nil
}

func getConsoleEncoder() zapcore.Encoder {
	conf := zap.NewProductionEncoderConfig()
	conf.TimeKey = "time"
	conf.EncodeTime = zapcore.ISO8601TimeEncoder
	return zapcore.NewConsoleEncoder(conf)
}
