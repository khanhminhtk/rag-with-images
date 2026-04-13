package trainingfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"rag_imagetotext_texttoimage/internal/application/dtos"
)

func (uc *trainingFileUseCase) ProcessAndIngest(ctx context.Context, req *dtos.ProcessAndIngestRequest) (dtos.ProcessAndIngestResult, error) {
	startedAt := time.Now()
	result := dtos.ProcessAndIngestResult{
		Success:        false,
		UploadedFiles:  0,
		InsertedPoints: 0,
		SkippedPoints:  0,
		Verified:       false,
		LatencyMs:      0,
	}

	if req == nil {
		err := errors.New("request is nil")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.ProcessAndIngest invalid request", err)
		return result, err
	}

	urlDownload := strings.TrimSpace(req.URLDownload)
	if urlDownload == "" {
		err := errors.New("url_download is required")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.ProcessAndIngest missing url_download", err)
		return result, err
	}

	reqUUID := strings.TrimSpace(req.UUID)
	if reqUUID == "" {
		reqUUID = uuid.NewString()
		uc.logger.Info(
			"internal.application.use_cases.orchestrator.training_file.ProcessAndIngest generated fallback uuid",
			"uuid", reqUUID,
		)
	}
	trainingUUID := reqUUID
	collectionName := trainingUUID
	result.UUID = trainingUUID

	effectiveBatchSize := uc.resolveTrainingBatchSize(0)
	markerDevMode := uc.Config.FileTraining.MarkerDevMode
	uc.logger.Info(
		"internal.application.use_cases.orchestrator.training_file.ProcessAndIngest batch config resolved",
		"uuid", trainingUUID,
		"config_batch_size", uc.Config.FileTraining.BatchSize,
		"effective_batch_size", effectiveBatchSize,
		"marker_dev_mode", markerDevMode,
	)

	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultPipelineTimeout
	}
	pipelineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	downloadDir := filepath.Join("data", "download", trainingUUID)
	processDir := filepath.Join("data", "processed", trainingUUID)
	uploadDir := filepath.Join("data", "upload", trainingUUID)
	result.ProcessDir = processDir

	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return result, fmt.Errorf("create download dir: %w", err)
	}
	if err := os.MkdirAll(processDir, 0755); err != nil {
		return result, fmt.Errorf("create process dir: %w", err)
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return result, fmt.Errorf("create upload dir: %w", err)
	}

	stepStartedAt := time.Now()
	downloadRes, err := uc.Download(pipelineCtx, &dtos.DownFileTrainingRequest{
		UrlDownFile: urlDownload,
		Uuid:        trainingUUID,
		PathSave:    downloadDir,
	})
	if err != nil {
		return result, fmt.Errorf("step download failed: %w", err)
	}
	result.DownloadPath = downloadRes.FilePath
	uc.logger.Info("internal.application.use_cases.orchestrator.training_file.ProcessAndIngest step completed", "step", "download", "uuid", trainingUUID, "latency_ms", time.Since(stepStartedAt).Milliseconds())

	stepStartedAt = time.Now()
	_, err = uc.AnalysisFile(pipelineCtx, &dtos.AnalysisFileRequest{
		FilePath: downloadRes.FilePath,
		DistDir:  processDir,
		Uuid:     trainingUUID,
		Dev:      markerDevMode,
	})
	if err != nil {
		return result, fmt.Errorf("step marker analysis failed: %w", err)
	}
	uc.logger.Info("internal.application.use_cases.orchestrator.training_file.ProcessAndIngest step completed", "step", "analysis", "uuid", trainingUUID, "latency_ms", time.Since(stepStartedAt).Milliseconds())

	markdownPath, _, artifactPaths, err := discoverProcessedArtifacts(processDir)
	if err != nil {
		return result, fmt.Errorf("discover processed artifacts: %w", err)
	}
	result.MarkdownPath = markdownPath

	stepStartedAt = time.Now()
	uploaded, err := uc.uploadArtifactsToMinio(pipelineCtx, uploadDir, append(artifactPaths, downloadRes.FilePath))
	if err != nil {
		return result, fmt.Errorf("step minio upload failed: %w", err)
	}
	result.UploadedFiles = uploaded
	uc.logger.Info("internal.application.use_cases.orchestrator.training_file.ProcessAndIngest step completed", "step", "upload_minio", "uuid", trainingUUID, "uploaded_files", uploaded, "latency_ms", time.Since(stepStartedAt).Milliseconds())

	stepStartedAt = time.Now()
	mdContent, err := uc.ReadMarkdownFile(pipelineCtx, markdownPath)
	if err != nil {
		return result, fmt.Errorf("step read markdown failed: %w", err)
	}
	chunkingCfg := uc.resolveChunkingConfig()
	chunks, parseStats := parseLineBasedChunks(mdContent, chunkingCfg)
	if len(chunks) == 0 {
		return result, errors.New("no chunk extracted from markdown")
	}
	shortChunkCount := countShortChunks(chunks, chunkingCfg)
	shortChunkPct := percentInt(shortChunkCount, len(chunks))
	headerDropPct := percentInt(parseStats.DroppedHeader, parseStats.RawSentenceCount)
	numericDropPct := percentInt(parseStats.DroppedNumeric, parseStats.RawSentenceCount)
	noiseDropPct := percentInt(parseStats.DroppedNoise, parseStats.RawSentenceCount)
	uc.logger.Info(
		"internal.application.use_cases.orchestrator.training_file.ProcessAndIngest step completed",
		"step", "parse_markdown",
		"uuid", trainingUUID,
		"raw_chunk_count", parseStats.RawSentenceCount,
		"chunk_count", len(chunks),
		"dropped_header", parseStats.DroppedHeader,
		"dropped_header_pct", fmt.Sprintf("%.2f", headerDropPct),
		"dropped_numeric", parseStats.DroppedNumeric,
		"dropped_numeric_pct", fmt.Sprintf("%.2f", numericDropPct),
		"dropped_noise", parseStats.DroppedNoise,
		"dropped_noise_pct", fmt.Sprintf("%.2f", noiseDropPct),
		"merged_short_sentences", parseStats.MergedShort,
		"short_chunk_pct", fmt.Sprintf("%.2f", shortChunkPct),
		"latency_ms", time.Since(stepStartedAt).Milliseconds(),
	)

	stepStartedAt = time.Now()
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Text)
	}
	textVectors, err := uc.embedTextAsyncByKafka(pipelineCtx, trainingUUID, texts, effectiveBatchSize)
	if err != nil {
		return result, fmt.Errorf("step embed text failed: %w", err)
	}
	filteredChunks, filteredTextVectors, skippedZeroNorm, skippedDimMismatch, err := filterValidChunksAndEmbeddings(chunks, textVectors)
	if err != nil {
		return result, fmt.Errorf("validate embeddings failed: %w", err)
	}
	if skippedZeroNorm > 0 || skippedDimMismatch > 0 {
		uc.logger.Info(
			"internal.application.use_cases.orchestrator.training_file.ProcessAndIngest invalid embeddings were filtered",
			"uuid", trainingUUID,
			"skipped_zero_norm", skippedZeroNorm,
			"skipped_dimension_mismatch", skippedDimMismatch,
			"remaining_chunks", len(filteredChunks),
		)
	}

	if err := uc.DoSemanticChunking(pipelineCtx, filteredTextVectors, chunkingCfg.SemanticSimThreshold); err != nil {
		return result, fmt.Errorf("step semantic chunking failed: %w", err)
	}
	mergedChunks, mergedVectors, err := mergeChunksBySemantic(filteredChunks, filteredTextVectors, chunkingCfg)
	if err != nil {
		return result, fmt.Errorf("merge semantic chunks failed: %w", err)
	}
	finalShortChunkCount := countShortChunks(mergedChunks, chunkingCfg)
	finalShortChunkPct := percentInt(finalShortChunkCount, len(mergedChunks))
	p50Tokens, p95Tokens, maxTokens := chunkTokenDistribution(mergedChunks)
	headSamples, tailSamples := sampleChunkTexts(mergedChunks, 3, 200)
	uc.logger.Info(
		"internal.application.use_cases.orchestrator.training_file.ProcessAndIngest step completed",
		"step", "embedding_semantic",
		"uuid", trainingUUID,
		"original_chunks", len(chunks),
		"final_chunk_count", len(mergedChunks),
		"short_chunk_pct", fmt.Sprintf("%.2f", finalShortChunkPct),
		"token_p50", p50Tokens,
		"token_p95", p95Tokens,
		"token_max", maxTokens,
		"head_samples", strings.Join(headSamples, " || "),
		"tail_samples", strings.Join(tailSamples, " || "),
		"latency_ms", time.Since(stepStartedAt).Milliseconds(),
	)

	stepStartedAt = time.Now()
	imageVectorByChunk := map[int][]float32{}
	for i, chunk := range mergedChunks {
		if len(chunk.ImagePaths) == 0 {
			continue
		}
		absPath := resolveImagePath(markdownPath, chunk.ImagePaths[0])
		if absPath == "" {
			continue
		}
		vec, imgErr := uc.embedSingleImageAsyncByKafka(pipelineCtx, trainingUUID, absPath)
		if imgErr != nil {
			uc.logger.Error("internal.application.use_cases.orchestrator.training_file.ProcessAndIngest image embedding failed", imgErr, "chunk_index", i, "image_path", absPath)
			continue
		}
		imageVectorByChunk[i] = vec
	}
	uc.logger.Info("internal.application.use_cases.orchestrator.training_file.ProcessAndIngest step completed", "step", "embedding_image_optional", "uuid", trainingUUID, "image_vector_count", len(imageVectorByChunk), "latency_ms", time.Since(stepStartedAt).Milliseconds())

	stepStartedAt = time.Now()
	points := make([]dtos.UploadVectorDBPoint, 0, len(mergedChunks))
	for i := range mergedChunks {
		payload := map[string]string{
			"doc_id":      trainingUUID,
			"unit_type":   "semantic_chunk",
			"text":        mergedChunks[i].Text,
			"chunk_index": strconv.Itoa(i),
			"lang":        strings.TrimSpace(req.Lang),
			"source_path": markdownPath,
			"page":        "0",
			"token_count": strconv.Itoa(len(strings.Fields(mergedChunks[i].Text))),
		}
		if len(mergedChunks[i].ImagePaths) > 0 {
			payload["image_path"] = strings.Join(mergedChunks[i].ImagePaths, ",")
		}

		vectors := []dtos.UploadVectorDBVector{
			{Name: "text_dense", Vector: mergedVectors[i]},
		}
		if imageVector, ok := imageVectorByChunk[i]; ok && len(imageVector) > 0 {
			vectors = append(vectors, dtos.UploadVectorDBVector{
				Name:   "image_dense",
				Vector: imageVector,
			})
		}

		points = append(points, dtos.UploadVectorDBPoint{
			Vectors: vectors,
			Payload: payload,
		})
	}

	uploadRes, err := uc.UploadVectorDB(pipelineCtx, &dtos.UploadVectorDBRequest{
		CollectionName: collectionName,
		Points:         points,
		BatchSize:      effectiveBatchSize,
	})
	if err != nil {
		return result, fmt.Errorf("step upload vectordb failed: %w", err)
	}
	result.InsertedPoints = uploadRes.InsertedPoints
	result.SkippedPoints = uploadRes.SkippedPoints
	uc.logger.Info("internal.application.use_cases.orchestrator.training_file.ProcessAndIngest step completed", "step", "upload_vectordb", "uuid", trainingUUID, "inserted_points", uploadRes.InsertedPoints, "latency_ms", time.Since(stepStartedAt).Milliseconds())

	stepStartedAt = time.Now()
	probeText := ""
	if len(mergedChunks) > 0 {
		probeText = strings.TrimSpace(mergedChunks[0].Text)
	}
	verified, err := uc.verifyVectorDBByDocID(pipelineCtx, collectionName, trainingUUID, probeText)
	if err != nil {
		return result, fmt.Errorf("step verify vectordb failed: %w", err)
	}
	result.Verified = verified
	if !verified {
		return result, errors.New("vectordb verification failed: no data found for doc_id")
	}
	uc.logger.Info("internal.application.use_cases.orchestrator.training_file.ProcessAndIngest step completed", "step", "verify_vectordb", "uuid", trainingUUID, "verified", verified, "latency_ms", time.Since(stepStartedAt).Milliseconds())

	result.Success = true
	result.LatencyMs = time.Since(startedAt).Milliseconds()
	return result, nil
}
