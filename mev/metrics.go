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
	// Whether or not a node is connected to the sentinel.
	SentinelConnected metrics.Gauge

	// SIDECAR METRICS
	// Size of the sidecar.
	MevBundleMempoolSize metrics.Gauge
	// Histogram of sidecar transaction sizes, in bytes.
	MevTxSizeBytes metrics.Histogram

	// Number of MEV bundles received by the sidecar in total.
	NumBundlesTotal metrics.Counter
	// Number of mev bundles received during the last block.
	NumBundlesLastBlock metrics.Gauge

	// Number of mev transactions added in total.
	NumMevTxsTotal metrics.Counter
	// Number of mev transactions received by sidecar in the last block.
	NumMevTxsLastBlock metrics.Gauge
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
		SentinelConnected: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "sentinel_connected",
			Help:      "Whether or not a node is connected to the mev sentinel. 1 if yes, 0 if no.",
		}, labels).With(labelsAndValues...),
		// SIDECAR METRICS
		MevBundleMempoolSize: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "mempool_size",
			Help:      "Size of the MEV bundle mempool (number of uncommitted transactions).",
		}, labels).With(labelsAndValues...),
		MevTxSizeBytes: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "tx_size_bytes",
			Help:      "MEV transaction sizes in bytes.",
		}, labels).With(labelsAndValues...),
		NumBundlesTotal: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "num_bundles_total",
			Help:      "Number of MEV bundles received in total.",
		}, labels).With(labelsAndValues...),
		NumBundlesLastBlock: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "num_bundles_last_block",
			Help:      "Number of MEV bundles received in the last block.",
		}, labels).With(labelsAndValues...),
		NumMevTxsTotal: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "num_txs_total",
			Help:      "Number of MEV transactions received in total.",
		}, labels).With(labelsAndValues...),
		NumMevTxsLastBlock: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "num_txs_last_block",
			Help:      "Number of MEV transactions received in the last block.",
		}, labels).With(labelsAndValues...),
	}
}

// NopMetrics returns no-op Metrics.
func NopMetrics() *Metrics {
	return &Metrics{
		SentinelConnected: discard.NewGauge(),
		// SIDECAR METRICS
		MevBundleMempoolSize: discard.NewGauge(),
		MevTxSizeBytes:       discard.NewHistogram(),
		NumBundlesTotal:      discard.NewCounter(),
		NumBundlesLastBlock:  discard.NewGauge(),
		NumMevTxsTotal:       discard.NewCounter(),
		NumMevTxsLastBlock:   discard.NewGauge(),
	}
}
