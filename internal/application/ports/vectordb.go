package ports

import (
	"context"

	domain "rag_imagetotext_texttoimage/internal/domain/entity_objects"
)

type DistanceMetric string

const (
	DistanceCosine    DistanceMetric = "cosine"
	DistanceDot       DistanceMetric = "dot"
	DistanceEuclid    DistanceMetric = "euclid"
	DistanceManhattan DistanceMetric = "manhattan"
)

type CollectionVectorConfig struct {
	Name     string
	Size     uint64
	Distance DistanceMetric
}

type CollectionSchema struct {
	Name              string
	Vectors           []CollectionVectorConfig
	Shards            uint32
	ReplicationFactor uint32
	OnDiskPayload     bool
	OptimizersMemmap  bool
}

type MatchOperator string

const (
	MatchOperatorEqual MatchOperator = "eq"
	MatchOperatorIn    MatchOperator = "in"
)

type FieldCondition struct {
	Key      string
	Operator MatchOperator
	Value    any
}

type Filter struct {
	Must    []FieldCondition
	Should  []FieldCondition
	MustNot []FieldCondition
}

func (f Filter) IsEmpty() bool {
	return len(f.Must) == 0 && len(f.Should) == 0 && len(f.MustNot) == 0
}

type SearchQuery struct {
	CollectionName string
	VectorName     string
	Vector         []float32
	Limit          uint64
	ScoreThreshold *float32
	WithPayload    bool
	Filter         *Filter
}

type SearchResult struct {
	Point *domain.PointObject
	Score float32
}

type PointStore interface {
	Upsert(ctx context.Context, collectionName string, points []domain.PointObject) error
	Search(ctx context.Context, query SearchQuery) ([]SearchResult, error)
	DeleteByIDs(ctx context.Context, collectionName string, ids []string) error
	DeleteByFilter(ctx context.Context, collectionName string, filter Filter) error
}

type CollectionStore interface {
	CollectionExists(ctx context.Context, collectionName string) (bool, error)
	CreateCollection(ctx context.Context, schema CollectionSchema) error
	EnsureCollection(ctx context.Context, schema CollectionSchema) error
	DeleteCollection(ctx context.Context, collectionName string) error
}

type QdrantStore interface {
	PointStore
	CollectionStore
	HealthCheck(ctx context.Context) error
}
