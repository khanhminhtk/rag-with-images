package qdrant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"

	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultRequestTimeout = 15 * time.Second
	defaultRetryAttempts  = 3
	defaultRetryBackoff   = 300 * time.Millisecond
	maxRetryBackoff       = 5 * time.Second
)

type collectionClient interface {
	CollectionExists(ctx context.Context, collectionName string) (bool, error)
	CreateCollection(ctx context.Context, collection *qdrant.CreateCollection) error
	DeleteCollection(ctx context.Context, collectionName string) error
}

type CollectionStoreOption func(*CollectionStore)

func WithQdrantRequestTimeout(timeout time.Duration) CollectionStoreOption {
	return func(c *CollectionStore) {
		if timeout > 0 {
			c.requestTimeout = timeout
		}
	}
}

func WithQdrantRetryAttempts(attempts int) CollectionStoreOption {
	return func(c *CollectionStore) {
		if attempts > 0 {
			c.retryAttempts = attempts
		}
	}
}

func WithQdrantRetryBackoff(backoff time.Duration) CollectionStoreOption {
	return func(c *CollectionStore) {
		if backoff > 0 {
			c.retryBackoff = backoff
		}
	}
}

type CollectionStore struct {
	client         collectionClient
	appLogger      util.Logger
	requestTimeout time.Duration
	retryAttempts  int
	retryBackoff   time.Duration
}

func NewCollectionStore(client *qdrant.Client, appLogger util.Logger, opts ...CollectionStoreOption) *CollectionStore {
	return newCollectionStoreWithClient(client, appLogger, opts...)
}

func newCollectionStoreWithClient(client collectionClient, appLogger util.Logger, opts ...CollectionStoreOption) *CollectionStore {
	store := &CollectionStore{
		client:         client,
		appLogger:      appLogger,
		requestTimeout: defaultRequestTimeout,
		retryAttempts:  defaultRetryAttempts,
		retryBackoff:   defaultRetryBackoff,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(store)
		}
	}
	if store.retryAttempts <= 0 {
		store.retryAttempts = 1
	}
	if store.requestTimeout <= 0 {
		store.requestTimeout = defaultRequestTimeout
	}
	if store.retryBackoff <= 0 {
		store.retryBackoff = defaultRetryBackoff
	}
	return store
}

var _ ports.CollectionStore = (*CollectionStore)(nil)

func toQdrantDistance(d ports.DistanceMetric) qdrant.Distance {
	switch d {
	case ports.DistanceCosine:
		return qdrant.Distance_Cosine
	case ports.DistanceEuclid:
		return qdrant.Distance_Euclid
	case ports.DistanceDot:
		return qdrant.Distance_Dot
	case ports.DistanceManhattan:
		return qdrant.Distance_Manhattan
	default:
		return qdrant.Distance_Cosine
	}
}

func (c *CollectionStore) CreateCollection(ctx context.Context, schema ports.CollectionSchema) error {
	source := qdrantSource("CollectionStore.CreateCollection")
	c.appLogger.Debug(
		"create collection started",
		"source", source,
		"collection", schema.Name,
		"vector_count", len(schema.Vectors),
	)

	if len(schema.Vectors) == 0 {
		err := fmt.Errorf("collection schema must contain at least one vector config")
		c.appLogger.Error("create collection validation failed", err, "source", source, "collection", schema.Name)
		return fmt.Errorf("%s: %w", source, err)
	}

	req := &qdrant.CreateCollection{
		CollectionName:      schema.Name,
		OnDiskPayload:       &schema.OnDiskPayload,
		SparseVectorsConfig: buildBM25SparseVectorsConfig(schema.OptimizersMemmap),
	}

	if schema.Shards > 0 {
		shards := uint32(schema.Shards)
		req.ShardNumber = &shards
	}

	if schema.ReplicationFactor > 0 {
		rf := uint32(schema.ReplicationFactor)
		req.ReplicationFactor = &rf
	}

	paramsMap := make(map[string]*qdrant.VectorParams, len(schema.Vectors))
	for _, v := range schema.Vectors {
		name := v.Name
		if name == "" {
			name = ports.VectorNameTextDense
		}
		onDisk := schema.OptimizersMemmap
		paramsMap[name] = &qdrant.VectorParams{
			Size:     v.Size,
			Distance: toQdrantDistance(v.Distance),
			OnDisk:   &onDisk,
		}
	}
	req.VectorsConfig = qdrant.NewVectorsConfigMap(paramsMap)

	if schema.OptimizersMemmap {
		memmapThreshold := uint64(1)
		req.OptimizersConfig = &qdrant.OptimizersConfigDiff{
			MemmapThreshold: &memmapThreshold,
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	if err := c.client.CreateCollection(timeoutCtx, req); err != nil {
		c.appLogger.Error("create collection failed", err, "source", source, "collection", schema.Name)
		return fmt.Errorf("%s: create collection failed: %w", source, err)
	}

	c.appLogger.Info("create collection success", "source", source, "collection", schema.Name, "vector_count", len(schema.Vectors))
	return nil
}

func buildBM25SparseVectorsConfig(onDisk bool) *qdrant.SparseVectorConfig {
	modifier := qdrant.Modifier_Idf
	return qdrant.NewSparseVectorsConfig(map[string]*qdrant.SparseVectorParams{
		vectorNameBM25: {
			Modifier: &modifier,
			Index: &qdrant.SparseIndexConfig{
				OnDisk: &onDisk,
			},
		},
	})
}

func (c *CollectionStore) CollectionExists(ctx context.Context, collectionName string) (bool, error) {
	source := qdrantSource("CollectionStore.CollectionExists")
	startedAt := time.Now()

	for attempt := 1; attempt <= c.retryAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}

		attemptCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
		exists, err := c.client.CollectionExists(attemptCtx, collectionName)
		cancel()
		if err == nil {
			c.appLogger.Info(
				"collection exists check success",
				"source", source,
				"collection", collectionName,
				"exists", exists,
				"attempt", attempt,
				"max_attempts", c.retryAttempts,
				"timeout_ms", c.requestTimeout.Milliseconds(),
				"elapsed_ms", time.Since(startedAt).Milliseconds(),
			)
			return exists, nil
		}

		code := status.Code(err)
		terminal := !isTransientCollectionError(err)
		c.appLogger.Error(
			"collection exists check failed",
			err,
			"source", source,
			"collection", collectionName,
			"attempt", attempt,
			"max_attempts", c.retryAttempts,
			"timeout_ms", c.requestTimeout.Milliseconds(),
			"elapsed_ms", time.Since(startedAt).Milliseconds(),
			"grpc_code", code.String(),
			"terminal", terminal,
		)
		if terminal || attempt == c.retryAttempts {
			return false, fmt.Errorf("%s: collection exists check failed after %d/%d attempts: %w", source, attempt, c.retryAttempts, err)
		}
		sleepWithBackoff(ctx, c.retryBackoff, attempt)
	}

	return false, fmt.Errorf("%s: collection exists check reached unexpected state", source)
}

func (c *CollectionStore) EnsureCollection(ctx context.Context, schema ports.CollectionSchema) error {
	source := qdrantSource("CollectionStore.EnsureCollection")
	startedAt := time.Now()

	exists, err := c.CollectionExists(ctx, schema.Name)
	if err != nil {
		c.appLogger.Error("ensure collection failed to check existence", err, "source", source, "collection", schema.Name)
		return fmt.Errorf("%s: collection exists check failed: %w", source, err)
	}
	if exists {
		c.appLogger.Info("ensure collection skipped, already exists", "source", source, "collection", schema.Name)
		return nil
	}

	c.appLogger.Info("ensure collection creating", "source", source, "collection", schema.Name)
	if err := c.CreateCollection(ctx, schema); err != nil {
		if isAlreadyExistsError(err) {
			c.appLogger.Info(
				"ensure collection create returned already-exists; continuing",
				"source", source,
				"collection", schema.Name,
				"elapsed_ms", time.Since(startedAt).Milliseconds(),
			)
		} else {
			c.appLogger.Error("ensure collection create failed", err, "source", source, "collection", schema.Name)
			return fmt.Errorf("%s: create collection failed: %w", source, err)
		}
	}

	verifyExists, verifyErr := c.CollectionExists(ctx, schema.Name)
	if verifyErr != nil {
		c.appLogger.Error("ensure collection verify failed", verifyErr, "source", source, "collection", schema.Name)
		return fmt.Errorf("%s: verify collection exists failed: %w", source, verifyErr)
	}
	if !verifyExists {
		err = fmt.Errorf("collection %q does not exist after ensure", schema.Name)
		c.appLogger.Error("ensure collection create failed", err, "source", source, "collection", schema.Name)
		return fmt.Errorf("%s: %w", source, err)
	}
	c.appLogger.Info("ensure collection completed", "source", source, "collection", schema.Name, "elapsed_ms", time.Since(startedAt).Milliseconds())
	return nil
}

func (c *CollectionStore) DeleteCollection(ctx context.Context, collectionName string) error {
	source := qdrantSource("CollectionStore.DeleteCollection")
	timeoutCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	if err := c.client.DeleteCollection(timeoutCtx, collectionName); err != nil {
		c.appLogger.Error("delete collection failed", err, "source", source, "collection", collectionName)
		return fmt.Errorf("%s: delete collection failed: %w", source, err)
	}

	c.appLogger.Info("delete collection success", "source", source, "collection", collectionName)
	return nil
}

func isTransientCollectionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	code := status.Code(err)
	switch code {
	case codes.DeadlineExceeded, codes.Unavailable, codes.ResourceExhausted, codes.Aborted:
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "temporarily unavailable")
}

func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	if status.Code(err) == codes.AlreadyExists {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "already exists")
}

func sleepWithBackoff(ctx context.Context, base time.Duration, attempt int) {
	backoff := base
	if backoff <= 0 {
		backoff = defaultRetryBackoff
	}
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if backoff >= maxRetryBackoff {
			backoff = maxRetryBackoff
			break
		}
	}
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
