package trainingfile

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"rag_imagetotext_texttoimage/internal/application/dtos"
	pb "rag_imagetotext_texttoimage/proto"
)

const (
	defaultTrainingBatchSize = 20
	maxTrainingBatchSize     = 200
)

func (uc *trainingFileUseCase) UploadVectorDB(ctx context.Context, req *dtos.UploadVectorDBRequest) (dtos.UploadVectorDBResult, error) {
	startedAt := time.Now()
	result := dtos.UploadVectorDBResult{
		Success:        false,
		InsertedPoints: 0,
		SkippedPoints:  0,
		LatencyMs:      0,
	}

	if req == nil {
		err := errors.New("request is nil")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.UploadVectorDB invalid request", err)
		return result, err
	}

	collectionName := strings.TrimSpace(req.CollectionName)
	if collectionName == "" {
		err := errors.New("collection_name is required")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.UploadVectorDB missing collection_name", err)
		return result, err
	}

	if len(req.Points) == 0 {
		err := errors.New("points is required")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.UploadVectorDB missing points", err)
		return result, err
	}

	batchSize := uc.resolveTrainingBatchSize(req.BatchSize)

	conn, ragClient, err := uc.newRAGServiceClient(ctx)
	if err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.UploadVectorDB rag grpc connection failed", err)
		return result, err
	}
	defer conn.Close()

	if err := uc.ensureRAGCollection(ctx, ragClient, collectionName, req.Points); err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.UploadVectorDB ensure collection failed", err, "collection_name", collectionName)
		return result, err
	}

	batch := make([]*pb.Point, 0, batchSize)
	inserted := 0
	skipped := 0
	batchIndex := 0

	flushBatch := func() error {
		if len(batch) == 0 {
			return nil
		}
		batchStartedAt := time.Now()

		resp, callErr := ragClient.InsertPoint(ctx, &pb.InsertPointRequest{
			CollectionName: collectionName,
			Points:         batch,
		})
		if callErr != nil {
			return fmt.Errorf("insert points batch %d failed: %w", batchIndex, callErr)
		}
		if resp == nil || !resp.Status {
			return fmt.Errorf("insert points batch %d returned status=false", batchIndex)
		}

		inserted += len(batch)
		uc.logger.Info(
			"internal.application.use_cases.orchestrator.training_file.UploadVectorDB batch insert succeeded",
			"batch_index", batchIndex,
			"batch_size", len(batch),
			"collection_name", collectionName,
			"latency_ms", time.Since(batchStartedAt).Milliseconds(),
		)

		batch = batch[:0]
		batchIndex++
		return nil
	}

	for i, point := range req.Points {
		pbPoint, ok := toProtoPoint(point)
		if !ok {
			skipped++
			uc.logger.Info(
				"internal.application.use_cases.orchestrator.training_file.UploadVectorDB skip invalid point",
				"index", i,
				"collection_name", collectionName,
			)
			continue
		}

		batch = append(batch, pbPoint)
		if len(batch) >= batchSize {
			if err := flushBatch(); err != nil {
				uc.logger.Error("internal.application.use_cases.orchestrator.training_file.UploadVectorDB batch insert failed", err, "batch_index", batchIndex)
				return result, err
			}
		}
	}

	if err := flushBatch(); err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.UploadVectorDB final batch insert failed", err, "batch_index", batchIndex)
		return result, err
	}

	result.Success = true
	result.InsertedPoints = inserted
	result.SkippedPoints = skipped
	result.LatencyMs = time.Since(startedAt).Milliseconds()

	uc.logger.Info(
		"internal.application.use_cases.orchestrator.training_file.UploadVectorDB completed",
		"collection_name", collectionName,
		"input_points", len(req.Points),
		"inserted_points", result.InsertedPoints,
		"skipped_points", result.SkippedPoints,
		"latency_ms", result.LatencyMs,
	)

	return result, nil
}

func (uc *trainingFileUseCase) ensureRAGCollection(ctx context.Context, ragClient pb.RagServiceClient, collectionName string, points []dtos.UploadVectorDBPoint) error {
	vectorSizeByName := map[string]uint64{}
	for _, p := range points {
		for _, v := range p.Vectors {
			name := strings.TrimSpace(v.Name)
			if name == "" || len(v.Vector) == 0 {
				continue
			}
			if _, exists := vectorSizeByName[name]; exists {
				continue
			}
			vectorSizeByName[name] = uint64(len(v.Vector))
		}
	}

	vectors := make([]*pb.CollectionVectorConfig, 0, len(vectorSizeByName))
	if sz, ok := vectorSizeByName["text_dense"]; ok {
		vectors = append(vectors, &pb.CollectionVectorConfig{
			Name:     "text_dense",
			Size:     sz,
			Distance: "cosine",
		})
	}
	if sz, ok := vectorSizeByName["image_dense"]; ok {
		vectors = append(vectors, &pb.CollectionVectorConfig{
			Name:     "image_dense",
			Size:     sz,
			Distance: "cosine",
		})
	}
	if len(vectors) == 0 {
		return errors.New("cannot ensure collection: no valid vector config derived from points")
	}

	resp, err := ragClient.CreateCollection(ctx, &pb.SchemaCollection{
		Name:              collectionName,
		Vectors:           vectors,
		OnDiskPayload:     true,
		OptimizersMemmap:  true,
		Shards:            1,
		ReplicationFactor: 1,
	})
	if err != nil {
		return fmt.Errorf("create collection %q failed: %w", collectionName, err)
	}
	if resp == nil || !resp.Status {
		return fmt.Errorf("create collection %q returned status=false", collectionName)
	}

	uc.logger.Info(
		"internal.application.use_cases.orchestrator.training_file.UploadVectorDB collection ensured",
		"collection_name", collectionName,
		"vector_count", len(vectors),
	)
	return nil
}

func (uc *trainingFileUseCase) resolveTrainingBatchSize(requested int) int {
	batchSize := requested
	if batchSize <= 0 {
		batchSize = uc.Config.FileTraining.BatchSize
	}
	if batchSize <= 0 {
		batchSize = defaultTrainingBatchSize
	}
	if batchSize > maxTrainingBatchSize {
		batchSize = maxTrainingBatchSize
	}
	return batchSize
}

func (uc *trainingFileUseCase) newRAGServiceClient(ctx context.Context) (*grpc.ClientConn, pb.RagServiceClient, error) {
	host := strings.TrimSpace(uc.Config.RAGService.GRPCHost)
	port := strings.TrimSpace(uc.Config.RAGService.GRPCPort)
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = strings.TrimSpace(uc.Config.RAGService.Port)
	}
	if port == "" {
		return nil, nil, fmt.Errorf("rag grpc port is empty")
	}

	addr := net.JoinHostPort(host, port)
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot connect to rag service at %s; ensure cmd/rag_service/main.go is running: %w", addr, err)
	}

	return conn, pb.NewRagServiceClient(conn), nil
}

func toProtoPoint(point dtos.UploadVectorDBPoint) (*pb.Point, bool) {
	vectorObjects := make([]*pb.VectorObject, 0, len(point.Vectors))
	for _, vector := range point.Vectors {
		name := strings.TrimSpace(vector.Name)
		if name == "" || len(vector.Vector) == 0 {
			continue
		}
		vectorObjects = append(vectorObjects, &pb.VectorObject{
			Name:   name,
			Vector: vector.Vector,
		})
	}
	if len(vectorObjects) == 0 {
		return nil, false
	}

	payload := map[string]string{}
	for key, value := range point.Payload {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		payload[k] = value
	}

	return &pb.Point{
		VectorObject: vectorObjects,
		Payload:      payload,
	}, true
}
