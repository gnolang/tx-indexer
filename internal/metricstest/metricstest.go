// Package metricstest reads single samples out of a Prometheus gatherer, so a
// test can assert what one served request or fetch recorded.
package metricstest

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

// Find returns the sample of the metric name whose labels are exactly labels,
// or nil if g holds none
func Find(t *testing.T, g prometheus.Gatherer, name string, labels map[string]string) *dto.Metric {
	t.Helper()

	families, err := g.Gather()
	require.NoError(t, err)

	for _, family := range families {
		if family.GetName() != name {
			continue
		}

		for _, sample := range family.GetMetric() {
			if hasLabels(sample, labels) {
				return sample
			}
		}
	}

	return nil
}

// Value returns the value of a counter or gauge sample, or the sample count of
// a histogram one. It fails the test if g holds no such sample.
func Value(t *testing.T, g prometheus.Gatherer, name string, labels map[string]string) float64 {
	t.Helper()

	sample := Find(t, g, name, labels)
	require.NotNilf(t, sample, "no sample %s%v", name, labels)

	switch {
	case sample.GetCounter() != nil:
		return sample.GetCounter().GetValue()
	case sample.GetGauge() != nil:
		return sample.GetGauge().GetValue()
	case sample.GetHistogram() != nil:
		return float64(sample.GetHistogram().GetSampleCount())
	default:
		require.FailNowf(t, "unsupported sample type", "%s%v", name, labels)

		return 0
	}
}

// hasLabels reports whether the labels of sample are exactly labels
func hasLabels(sample *dto.Metric, labels map[string]string) bool {
	pairs := sample.GetLabel()
	if len(pairs) != len(labels) {
		return false
	}

	for _, pair := range pairs {
		value, ok := labels[pair.GetName()]
		if !ok || value != pair.GetValue() {
			return false
		}
	}

	return true
}
