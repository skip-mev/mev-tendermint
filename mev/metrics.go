package mev

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

const (
	// MetricsSubsystem is a subsystem shared by all metrics exposed by this
	// package.
	MetricsSubsystem = "mev"
)

// Metrics contains metrics exposed by this package.
type Metrics struct {
	// Whether or not a node is connected to the relay.
	RelayConnected metrics.Gauge

	// SIDECAR METRICS
	// Histogram of sidecar transaction sizes, in bytes.
	SidecarTxSizeBytes metrics.Histogram
	// Size of the sidecar.
	SidecarSize metrics.Gauge

	// Number of MEV bundles received by the sidecar in total.
	NumBundlesTotal metrics.Counter
	// Number of mev bundles received during the last block.
	NumBundlesLastBlock metrics.Gauge

	// Number of mev transactions added in total.
	NumTxsTotal metrics.Counter
	// Number of mev transactions received by sidecar in the last block.
	NumTxsLastBlock metrics.Gauge
}

// PrometheusMetrics returns Metrics build using Prometheus client library.
// Optionally, labels can be provided along with their values ("foo",
// "fooValue").
func PrometheusMetrics(namespace string, labelsAndValues ...string) *Metrics {
	labels := []string{}
	for i := 0; i < len(labelsAndValues); i += 2 {
		labels = append(labels, labelsAndValues[i])
	}
	return &Metrics{
		RelayConnected: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "relay_connected",
			Help:      "Whether or not a node is connected to the mev relay / sentinel. 1 if yes, 0 if no.",
		}, labels).With(labelsAndValues...),
		// SIDECAR METRICS
		SidecarSize: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "size",
			Help:      "Size of the sidecar (number of uncommitted transactions).",
		}, labels).With(labelsAndValues...),
		SidecarTxSizeBytes: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "size_bytes",
			Help:      "MEV transaction sizes in bytes.",
		}, labels).With(labelsAndValues...),
		NumBundlesTotal: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "num_bundles_total",
			Help:      "Number of MEV bundles received by the sidecar in total.",
		}, labels).With(labelsAndValues...),
		NumBundlesLastBlock: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "num_bundles_last_block",
			Help:      "Number of MEV bundles received by the sidecar in the last block.",
		}, labels).With(labelsAndValues...),
		NumTxsTotal: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "num_txs_total",
			Help:      "Number of MEV transactions received to the sidecar in total.",
		}, labels).With(labelsAndValues...),
		NumTxsLastBlock: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "num_txs_last_block",
			Help:      "Number of MEV transactions received to the sidecar in the last block.",
		}, labels).With(labelsAndValues...),
	}
}

// NopMetrics returns no-op Metrics.
func NopMetrics() *Metrics {
	return &Metrics{
		RelayConnected:        discard.NewGauge(),
		// SIDECAR METRICS
		SidecarSize:        discard.NewGauge(),
		SidecarTxSizeBytes: discard.NewHistogram(),
		NumBundlesTotal:     discard.NewCounter(),
		NumBundlesLastBlock: discard.NewGauge(),
		NumTxsTotal:     discard.NewCounter(),
		NumTxsLastBlock: discard.NewGauge(),
	}
}

