package mempool

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	"github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

const (
	// MetricsSubsystem is a subsystem shared by all metrics exposed by this
	// package.
	MetricsSubsystem        = "mempool"
	SidecarMetricsSubsystem = "sidecar"
)

// Metrics contains metrics exposed by this package.
// see MetricsProvider for descriptions.
type Metrics struct {
	// Size of the mempool.
	Size metrics.Gauge

	// Histogram of transaction sizes, in bytes.
	TxSizeBytes metrics.Histogram

	// Number of failed transactions.
	FailedTxs metrics.Counter

	// RejectedTxs defines the number of rejected transactions. These are
	// transactions that passed CheckTx but failed to make it into the mempool
	// due to resource limits, e.g. mempool is full and no lower priority
	// transactions exist in the mempool.
	RejectedTxs metrics.Counter

	// EvictedTxs defines the number of evicted transactions. These are valid
	// transactions that passed CheckTx and existed in the mempool but were later
	// evicted to make room for higher priority valid transactions that passed
	// CheckTx.
	EvictedTxs metrics.Counter

	// Number of times transactions are rechecked in the mempool.
	RecheckTimes metrics.Counter

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
		Size: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "size",
			Help:      "Size of the mempool (number of uncommitted transactions).",
		}, labels).With(labelsAndValues...),

		TxSizeBytes: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "tx_size_bytes",
			Help:      "Transaction sizes in bytes.",
			Buckets:   stdprometheus.ExponentialBuckets(1, 3, 17),
		}, labels).With(labelsAndValues...),

		FailedTxs: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "failed_txs",
			Help:      "Number of failed transactions.",
		}, labels).With(labelsAndValues...),

		RejectedTxs: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "rejected_txs",
			Help:      "Number of rejected transactions.",
		}, labels).With(labelsAndValues...),

		EvictedTxs: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "evicted_txs",
			Help:      "Number of evicted transactions.",
		}, labels).With(labelsAndValues...),

		RecheckTimes: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "recheck_times",
			Help:      "Number of times transactions are rechecked in the mempool.",
		}, labels).With(labelsAndValues...),

		// SIDECAR METRICS
		SidecarSize: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: SidecarMetricsSubsystem,
			Name:      "size",
			Help:      "Size of the sidecar (number of uncommitted transactions).",
		}, labels).With(labelsAndValues...),
		SidecarTxSizeBytes: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: SidecarMetricsSubsystem,
			Name:      "size_bytes",
			Help:      "MEV transaction sizes in bytes.",
		}, labels).With(labelsAndValues...),

		NumBundlesTotal: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: SidecarMetricsSubsystem,
			Name:      "num_bundles_total",
			Help:      "Number of MEV bundles received by the sidecar in total.",
		}, labels).With(labelsAndValues...),
		NumBundlesLastBlock: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: SidecarMetricsSubsystem,
			Name:      "num_bundles_last_block",
			Help:      "Number of MEV bundles received by the sidecar in the last block.",
		}, labels).With(labelsAndValues...),

		NumMevTxsTotal: prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: SidecarMetricsSubsystem,
			Name:      "num_mev_txs_total",
			Help:      "Number of MEV transactions received to the sidecar in total.",
		}, labels).With(labelsAndValues...),
		NumMevTxsLastBlock: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: SidecarMetricsSubsystem,
			Name:      "num_mev_txs_last_block",
			Help:      "Number of MEV transactions received to the sidecar in the last block.",
		}, labels).With(labelsAndValues...),
	}
}

// NopMetrics returns no-op Metrics.
func NopMetrics() *Metrics {
	return &Metrics{
		Size:         discard.NewGauge(),
		TxSizeBytes:  discard.NewHistogram(),
		FailedTxs:    discard.NewCounter(),
		RejectedTxs:  discard.NewCounter(),
		EvictedTxs:   discard.NewCounter(),
		RecheckTimes: discard.NewCounter(),

		// SIDECAR METRICS
		SidecarSize:        discard.NewGauge(),
		SidecarTxSizeBytes: discard.NewHistogram(),

		NumBundlesTotal:     discard.NewCounter(),
		NumBundlesLastBlock: discard.NewGauge(),

		NumMevTxsTotal:     discard.NewCounter(),
		NumMevTxsLastBlock: discard.NewGauge(),
	}
}
