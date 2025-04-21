package logger

import (
	"github.com/MirrorChyan/resource-backend/internal/config"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	level = zap.NewAtomicLevelAt(zap.InfoLevel)
)

func init() {
	config.RegisterKeyListener(config.KeyListener{
		Key: "log.level",
		Listener: func(l any) {
			val, ok := l.(string)
			if !ok {
				return
			}
			SetLevel(val)
		},
	})
}

func SetLevel(l string) {
	level.SetLevel(getLevel(l))
}

func New() *zap.Logger {
	SetLevel(config.GConfig.Log.Level)
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

func getConsoleEncoder() zapcore.Encoder {
	conf := zap.NewProductionEncoderConfig()
	conf.TimeKey = "time"
	conf.EncodeTime = zapcore.ISO8601TimeEncoder
	return zapcore.NewConsoleEncoder(conf)
}
