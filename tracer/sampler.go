package tracer

import (
	"github.com/pkg/errors"
	"github.com/uber/jaeger-client-go"
)

// SamplerConfig holds configuration for OperationProbabilisticSampler.
type SamplerConfig struct {
	// Enabled if sampler enabled.
	Enabled bool
	// Default is default probabilistic rate that will be used if there is override for concrete operations.
	Default float64
	// Operations is per operation probabilistic rate override.
	Operations map[string]float64
}

// OperationProbabilisticSampler is an implementation of jaeger.Sampler interface that allows to specify sampling rate for operations.
// If no concrete rate is set for operation then default one is used.
type OperationProbabilisticSampler struct {
	samplers       map[string]jaeger.Sampler
	defaultSampler jaeger.Sampler
}

// NewOperationProbabilisticSampler creates new OperationProbabilisticSampler.
func NewOperationProbabilisticSampler(defaultRate float64, rates map[string]float64) (*OperationProbabilisticSampler, error) {
	samplers := make(map[string]jaeger.Sampler, len(rates))
	for operation, rate := range rates {
		sampler, err := jaeger.NewProbabilisticSampler(rate)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create sampler for operation %q", operation)
		}

		samplers[operation] = sampler
	}

	defSampler, err := jaeger.NewProbabilisticSampler(defaultRate)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create default sampler")
	}

	return &OperationProbabilisticSampler{
		defaultSampler: defSampler,
		samplers:       samplers,
	}, nil
}

// IsSampled decides whether a trace with given `id` and `operation`
// should be sampled. This function will also return the tags that
// can be used to identify the type of sampling that was applied to
// the root span. Most simple samplers would return two tags,
// sampler.type and sampler.param, similar to those used in the Configuration
func (s *OperationProbabilisticSampler) IsSampled(id jaeger.TraceID, operation string) (sampled bool, tags []jaeger.Tag) {
	if sampler, exists := s.samplers[operation]; exists {
		return sampler.IsSampled(id, operation)
	}

	return s.defaultSampler.IsSampled(id, operation)
}

// Close does a clean shutdown of the sampler, stopping any background
// go-routines it may have started.
func (s *OperationProbabilisticSampler) Close() {
	s.defaultSampler.Close()
	for _, sampler := range s.samplers {
		sampler.Close()
	}
}

// Equal checks if the `other` sampler is functionally equivalent
// to this sampler.
func (s *OperationProbabilisticSampler) Equal(other jaeger.Sampler) bool {
	return false
}
