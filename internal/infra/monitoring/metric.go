package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	InFlight       prometheus.Gauge
	QueueLength    prometheus.Gauge
	ProcessedTotal *prometheus.CounterVec
	RetryTotal     *prometheus.CounterVec
	PanicsTotal    *prometheus.CounterVec
	ProcessingTime *prometheus.HistogramVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		InFlight: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "kafka_task_inflight",
			Help: "Current number of tasks being processed.",
		}),
		QueueLength: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "kafka_task_queue_length",
			Help: "Current internal task queue length.",
		}),
		ProcessedTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "kafka_task_processed_total",
			Help: "Total processed tasks by topic, handler and status.",
		}, []string{"topic", "handler", "status"}),
		RetryTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "kafka_task_retry_total",
			Help: "Total retried tasks by topic, handler and error type.",
		}, []string{"topic", "handler", "error_type"}),
		PanicsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "kafka_task_panics_total",
			Help: "Total panics recovered by topic and handler.",
		}, []string{"topic", "handler"}),
		ProcessingTime: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "kafka_task_processing_seconds",
			Help:    "Task processing duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"topic", "handler", "status"}),
	}
}
