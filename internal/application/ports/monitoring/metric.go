package monitoring

type MetricsInstrumentor interface {
	RecordKafkaProcess(topic string, status string, errType string)
	RecordModelInferenceTime(modelName string, duration float64)
}

