package trainingfile

import (
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
)

type trainingFileUseCase struct {
	logger         util.Logger
	KafkaPublisher ports.KafkaPublisher
	KafkaConsumer  ports.KafkaConsumer
	Config         util.Config
	ragPointWriter ports.RagPointWriter
}

func NewTrainingFileUseCase(
	logger util.Logger,
	publisher ports.KafkaPublisher,
	consumer ports.KafkaConsumer,
	Config util.Config,
	ragPointWriter ...ports.RagPointWriter,
) ports.TrainingFileUseCase {
	var writer ports.RagPointWriter
	if len(ragPointWriter) > 0 {
		writer = ragPointWriter[0]
	}

	return &trainingFileUseCase{
		logger:         logger,
		KafkaPublisher: publisher,
		KafkaConsumer:  consumer,
		Config:         Config,
		ragPointWriter: writer,
	}
}
