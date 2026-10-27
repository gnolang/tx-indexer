// Package metrics holds the indexer's Prometheus metrics: those of the
// requests it serves and of its fetch of the chain.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// namespace prefixes every metric of the indexer's own
const namespace = "tx_indexer"

// The APIs a request is served over, the values of the api label
const (
	APIGraphQL = "graphql"
	APIJSONRPC = "jsonrpc"
)

// Status is the outcome of a request, named by the status label. It is a type
// of its own, so a value the metrics do not declare fails to compile rather
// than open a series no alert reads.
type Status int

// The outcomes of a request
const (
	// StatusOK is a request answered without an error
	StatusOK Status = iota

	// StatusError is a request answered with an error other than the query
	// deadline
	StatusError

	// StatusDeadline is a request stopped by the query deadline
	StatusDeadline
)

// String is the status label's value for s
func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusError:
		return "error"
	case StatusDeadline:
		return "deadline"
	default:
		return "Status(" + strconv.Itoa(int(s)) + ")"
	}
}

// FetchStage is a step of the fetch that can fail, named by the stage label.
// It is a type of its own for the same reason as Status.
type FetchStage int

// The steps of the fetch that can fail
const (
	// FetchStageChainHeight is the read of the chain's latest height
	FetchStageChainHeight FetchStage = iota

	// FetchStageRange is the fetch of a range of blocks and their results
	FetchStageRange
)

// String is the stage label's value for s
func (s FetchStage) String() string {
	switch s {
	case FetchStageChainHeight:
		return "chain_height"
	case FetchStageRange:
		return "range"
	default:
		return "FetchStage(" + strconv.Itoa(int(s)) + ")"
	}
}

// durationBuckets span a point read to a scan of the whole chain, which runs
// for minutes when no query deadline bounds it
var durationBuckets = []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 2.5, 5, 10, 30, 60}

// Request is one served request as the metrics record it: a JSON-RPC call, or
// a GraphQL root query field
type Request struct {
	// API is the API the request was served over, APIGraphQL or APIJSONRPC
	API string

	// Operation is the GraphQL root field or the JSON-RPC method
	Operation string

	// Status is the request's outcome
	Status Status

	// Duration is how long the request took to serve
	Duration time.Duration

	// RowsScanned counts the rows read from the storage
	RowsScanned int64

	// RowsReturned counts the rows answered
	RowsReturned int64
}

var _ prometheus.Gatherer = (*Metrics)(nil)

// Metrics are the indexer's Prometheus metrics, on a registry of their own.
// Their labels never carry a client address or a request's arguments: only
// the API, the operation and the status, a set the schema and the registered
// methods bound. A nil Metrics records nothing, so instrumented code runs
// without one.
type Metrics struct {
	registry *prometheus.Registry

	requests     *prometheus.CounterVec
	duration     *prometheus.HistogramVec
	rowsScanned  *prometheus.CounterVec
	rowsReturned *prometheus.CounterVec
	inFlight     *prometheus.GaugeVec

	indexedHeight prometheus.Gauge
	chainHeight   prometheus.Gauge
	fetchErrors   *prometheus.CounterVec
	fetchRetries  prometheus.Counter
}

// New creates the indexer's metrics, with the Go runtime's and the process's.
// The fetch errors start at zero for every stage.
func New() *Metrics {
	operationLabels := []string{"api", "operation"}

	m := &Metrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "requests_total",
				Help:      "Requests served, by API, operation and status (ok, error, deadline).",
			},
			[]string{"api", "operation", "status"},
		),
		duration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "request_duration_seconds",
				Help:      "Time taken to serve a request, by API and operation.",
				Buckets:   durationBuckets,
			},
			operationLabels,
		),
		rowsScanned: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rows_scanned_total",
				Help:      "Rows read from the storage to serve requests, by API and operation.",
			},
			operationLabels,
		),
		rowsReturned: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rows_returned_total",
				Help:      "Rows answered to requests, by API and operation.",
			},
			operationLabels,
		),
		inFlight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "requests_in_flight",
				Help:      "Requests being served, by API and operation.",
			},
			operationLabels,
		),
		indexedHeight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "indexed_height",
			Help:      "The latest block height written to the database.",
		}),
		chainHeight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "chain_height",
			Help:      "The chain's latest block height, as last read from the node.",
		}),
		fetchErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "fetch_errors_total",
				Help:      "Failed reads from the node, by stage (chain_height, range).",
			},
			[]string{"stage"},
		),
		fetchRetries: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "fetch_retries_total",
			Help:      "Block ranges fetched again after their fetch failed.",
		}),
	}

	for _, stage := range []FetchStage{FetchStageChainHeight, FetchStageRange} {
		m.fetchErrors.WithLabelValues(stage.String())
	}

	m.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requests,
		m.duration,
		m.rowsScanned,
		m.rowsReturned,
		m.inFlight,
		m.indexedHeight,
		m.chainHeight,
		m.fetchErrors,
		m.fetchRetries,
	)

	return m
}

// Gather gathers every metric, as a Prometheus scrape reads them
func (m *Metrics) Gather() ([]*dto.MetricFamily, error) {
	return m.registry.Gather()
}

// Handler serves every metric in the Prometheus exposition format
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// RequestStarted counts a request of operation over api in flight
func (m *Metrics) RequestStarted(api, operation string) {
	if m == nil {
		return
	}

	m.inFlight.WithLabelValues(api, operation).Inc()
}

// RequestEnded records a request RequestStarted counted in flight, once it
// has been served
func (m *Metrics) RequestEnded(r Request) {
	if m == nil {
		return
	}

	m.inFlight.WithLabelValues(r.API, r.Operation).Dec()
	m.requests.WithLabelValues(r.API, r.Operation, r.Status.String()).Inc()
	m.duration.WithLabelValues(r.API, r.Operation).Observe(r.Duration.Seconds())
	m.rowsScanned.WithLabelValues(r.API, r.Operation).Add(float64(r.RowsScanned))
	m.rowsReturned.WithLabelValues(r.API, r.Operation).Add(float64(r.RowsReturned))
}

// SetIndexedHeight records the latest block height written to the database
func (m *Metrics) SetIndexedHeight(height uint64) {
	if m == nil {
		return
	}

	m.indexedHeight.Set(float64(height))
}

// SetChainHeight records the chain's latest block height, as just read from
// the node
func (m *Metrics) SetChainHeight(height uint64) {
	if m == nil {
		return
	}

	m.chainHeight.Set(float64(height))
}

// CountFetchError counts a failed read from the node at stage
func (m *Metrics) CountFetchError(stage FetchStage) {
	if m == nil {
		return
	}

	m.fetchErrors.WithLabelValues(stage.String()).Inc()
}

// CountFetchRetry counts a block range fetched again after its fetch failed
func (m *Metrics) CountFetchRetry() {
	if m == nil {
		return
	}

	m.fetchRetries.Inc()
}
