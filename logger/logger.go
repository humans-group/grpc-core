package logger

import (
	"fmt"
	"github.com/mkorolyov/core/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	//prod or dev
	Type string
}

const (
	prod = "prod"
	dev  = "prod"
)

func Init(loader config.Loader) *zap.Logger {
	cfg := &Config{}
	if err := loader.Load("Logger", cfg); err != nil {
		cfg.Type = prod
	}

	var l *zap.Logger
	var err error

	opts := []zap.Option{
		zap.AddStacktrace(zapcore.ErrorLevel),
	}

	switch cfg.Type {
	case "prod":
		l, err = zap.NewProduction(opts...)
	case "dev":
		l, err = zap.NewDevelopment(opts...)
	default:
		panic(fmt.Sprintf("unexpected logger type %s want `prod` or `dev`"))
	}

	if err != nil {
		panic(fmt.Sprintf("failed to build %s logger: %v", cfg.Type, err))
	}

	return l
}
