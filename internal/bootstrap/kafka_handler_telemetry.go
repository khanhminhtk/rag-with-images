package bootstrap

import (
	"fmt"
	"time"

	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/infra/monitoring"
	"rag_imagetotext_texttoimage/internal/util"
)

type kafkaHandlerTelemetry struct {
	metrics         *monitoring.Metrics
	logger          util.Logger
	msg             ports.ConsumeMessage
	handlerName     string
	panicLogMessage string
	startedAt       time.Time
}

func newKafkaHandlerTelemetry(metrics *monitoring.Metrics, logger util.Logger, msg ports.ConsumeMessage, handlerName string, panicLogMessage string) *kafkaHandlerTelemetry {
	return &kafkaHandlerTelemetry{
		metrics:         metrics,
		logger:          logger,
		msg:             msg,
		handlerName:     handlerName,
		panicLogMessage: panicLogMessage,
		startedAt:       time.Now(),
	}
}

func (k *kafkaHandlerTelemetry) start() {
	if k == nil || k.metrics == nil {
		return
	}
	k.metrics.InFlight.Inc()
	k.metrics.QueueLength.Set(float64(k.msg.Lag))
}

func (k *kafkaHandlerTelemetry) done() {
	if k == nil || k.metrics == nil {
		return
	}
	k.metrics.InFlight.Dec()
}

func (k *kafkaHandlerTelemetry) observe(status string) {
	if k == nil || k.metrics == nil {
		return
	}
	k.metrics.ProcessedTotal.WithLabelValues(k.msg.Topic, k.handlerName, status).Inc()
	k.metrics.ProcessingTime.WithLabelValues(k.msg.Topic, k.handlerName, status).Observe(time.Since(k.startedAt).Seconds())
}

func (k *kafkaHandlerTelemetry) retry(errorType string) {
	if k == nil || k.metrics == nil {
		return
	}
	k.metrics.RetryTotal.WithLabelValues(k.msg.Topic, k.handlerName, errorType).Inc()
}

func (k *kafkaHandlerTelemetry) recover(handlerErr *error) {
	if k == nil {
		return
	}
	recovered := recover()
	if recovered == nil {
		return
	}

	panicErr := fmt.Errorf("panic recovered: %v", recovered)
	if k.metrics != nil {
		k.metrics.PanicsTotal.WithLabelValues(k.msg.Topic, k.handlerName).Inc()
		k.observe("panic")
	}
	if handlerErr != nil {
		*handlerErr = panicErr
	}
	if k.logger != nil {
		k.logger.Error(k.panicLogMessage, panicErr, "topic", k.msg.Topic, "offset", k.msg.Offset)
	}
}
