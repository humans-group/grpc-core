package tracer

import (
	"github.com/mkorolyov/core/config"
	"io"

	"github.com/pkg/errors"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaeger_zap "github.com/uber/jaeger-client-go/log/zap"
	jaegerprometheus "github.com/uber/jaeger-lib/metrics/prometheus"
	"go.uber.org/zap"
)

func InitJaeger(serviceName string, loader config.Loader, l *zap.Logger) (io.Closer,
	error) {
	cfg := jaegercfg.Configuration{}
	samplerConfig := SamplerConfig{}
	loader.MustLoad("Jaeger", &cfg)
	loader.MustLoad("TracerSampler", &samplerConfig)

	metricsFactory := jaegerprometheus.New()

	options := []jaegercfg.Option{
		jaegercfg.Metrics(metricsFactory),
		jaegercfg.Logger(jaeger_zap.NewLogger(l)),
	}

	if samplerConfig.Enabled {
		sampler, err := NewOperationProbabilisticSampler(samplerConfig.Default, samplerConfig.Operations)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create sampler")
		}

		options = append(options, jaegercfg.Sampler(sampler))
	}

	closer, err := cfg.InitGlobalTracer(serviceName, options...)
	if err != nil {
		l.Sugar().Panicf("failed to init jaeger: %v", err)
	}

	return closer, nil
}
