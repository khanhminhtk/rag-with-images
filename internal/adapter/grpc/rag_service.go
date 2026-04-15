package grpc

import (
	"context"
	"errors"
	usecases "rag_imagetotext_texttoimage/internal/application/use_cases"
	"strconv"
	"strings"
	"time"

	"rag_imagetotext_texttoimage/internal/application/ports"
	domain "rag_imagetotext_texttoimage/internal/domain/entity_objects"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"

	"github.com/google/uuid"
)

type RagService struct {
	pb.UnimplementedRagServiceServer
	appLogger          util.Logger
	searchWithVectorDB *usecases.SearchWithVectorDB
	pointStore         ports.PointStore
	collectionStore    ports.CollectionStore
}

func NewRagService(
	appLogger util.Logger,
	searchWithVectorDB *usecases.SearchWithVectorDB,
	pointStore ports.PointStore,
	collectionStore ports.CollectionStore,
) *RagService {
	return &RagService{
		appLogger:          appLogger,
		searchWithVectorDB: searchWithVectorDB,
		pointStore:         pointStore,
		collectionStore:    collectionStore,
	}
}

func (r *RagService) CreateCollection(ctx context.Context, req *pb.SchemaCollection) (*pb.ResponseCreateCollection, error) {
	startedAt := time.Now()
	r.appLogger.Info("rag grpc CreateCollection started", "collection", req.Name, "vector_count", len(req.Vectors))

	vectors := make([]ports.CollectionVectorConfig, 0, len(req.Vectors))
	for _, v := range req.Vectors {
		vectors = append(vectors, ports.CollectionVectorConfig{
			Name:     v.Name,
			Size:     v.Size,
			Distance: ports.DistanceMetric(v.Distance),
		})
	}

	schema := ports.CollectionSchema{
		Name:              req.Name,
		Vectors:           vectors,
		Shards:            req.Shards,
		ReplicationFactor: req.ReplicationFactor,
		OnDiskPayload:     req.OnDiskPayload,
		OptimizersMemmap:  req.OptimizersMemmap,
	}

	if err := r.collectionStore.EnsureCollection(ctx, schema); err != nil {
		return &pb.ResponseCreateCollection{Name: req.Name, Status: false}, err
	}

	r.appLogger.Info("rag grpc CreateCollection completed", "collection", req.Name, "status", true, "latency_ms", time.Since(startedAt).Milliseconds())
	return &pb.ResponseCreateCollection{Name: req.Name, Status: true}, nil
}

func (r *RagService) DeleteCollection(ctx context.Context, req *pb.DeleteCollectionRequest) (*pb.ResponseDeleteCollection, error) {
	startedAt := time.Now()
	r.appLogger.Info("rag grpc DeleteCollection started", "collection", req.Name)

	if err := r.collectionStore.DeleteCollection(ctx, req.Name); err != nil {
		r.appLogger.Error("DeleteCollection error", err)
		return nil, err
	}

	exists, err := r.collectionStore.CollectionExists(ctx, req.Name)
	if err != nil {
		r.appLogger.Error("CollectionExists error", err)
		return nil, err
	}

	r.appLogger.Info("rag grpc DeleteCollection completed", "collection", req.Name, "status", !exists, "latency_ms", time.Since(startedAt).Milliseconds())
	return &pb.ResponseDeleteCollection{
		Name:   req.Name,
		Status: !exists,
	}, nil
}

func (r *RagService) InsertPoint(ctx context.Context, req *pb.InsertPointRequest) (*pb.ResponseInsertPoint, error) {
	startedAt := time.Now()
	r.appLogger.Info("rag grpc InsertPoint started", "collection", req.CollectionName, "points", len(req.Points))

	exists, err := r.collectionStore.CollectionExists(ctx, req.CollectionName)
	if err != nil {
		return &pb.ResponseInsertPoint{CollectionName: req.CollectionName, Status: false}, err
	}
	if !exists {
		return &pb.ResponseInsertPoint{CollectionName: req.CollectionName, Status: false},
			errors.New("collection does not exist")
	}

	points := make([]domain.PointObject, 0, len(req.Points))
	for _, p := range req.Points {
		id, err := uuid.NewUUID()
		if err != nil {
			return &pb.ResponseInsertPoint{CollectionName: req.CollectionName, Status: false}, err
		}

		vec := domain.VectorObject{}
		for _, v := range p.VectorObject {
			switch v.Name {
			case "text_dense":
				vec.TextDense = v.Vector
			case "image_dense":
				vec.ImageDense = v.Vector
			default:
				return &pb.ResponseInsertPoint{CollectionName: req.CollectionName, Status: false},
					errors.New("vector name is not text_dense or image_dense")
			}
		}

		payload := mapToPointPayload(p.Payload)

		points = append(points, domain.PointObject{
			ID:      id.String(),
			Vector:  vec,
			Payload: payload,
		})
	}

	if err := r.pointStore.Upsert(ctx, req.CollectionName, points); err != nil {
		return &pb.ResponseInsertPoint{CollectionName: req.CollectionName, Status: false}, err
	}

	r.appLogger.Info("rag grpc InsertPoint completed", "collection", req.CollectionName, "status", true, "points", len(points), "latency_ms", time.Since(startedAt).Milliseconds())
	return &pb.ResponseInsertPoint{CollectionName: req.CollectionName, Status: true}, nil
}

func (r *RagService) DeletePointFilter(ctx context.Context, req *pb.DeletePointFilterRequest) (*pb.ResponseDeletePointFilter, error) {
	startedAt := time.Now()
	r.appLogger.Info("rag grpc DeletePointFilter started", "collection", req.CollectionName)

	exists, err := r.collectionStore.CollectionExists(ctx, req.CollectionName)
	if err != nil {
		return &pb.ResponseDeletePointFilter{CollectionName: req.CollectionName, Status: false}, err
	}
	if !exists {
		return &pb.ResponseDeletePointFilter{CollectionName: req.CollectionName, Status: false},
			errors.New("collection does not exist")
	}

	if req.Filter == nil {
		return &pb.ResponseDeletePointFilter{CollectionName: req.CollectionName, Status: false},
			errors.New("filter is required")
	}

	filter := pbFilterToPortsFilter(req.Filter)
	if filter.IsEmpty() {
		return &pb.ResponseDeletePointFilter{CollectionName: req.CollectionName, Status: false},
			errors.New("filter must have at least one condition")
	}

	if err := r.pointStore.DeleteByFilter(ctx, req.CollectionName, filter); err != nil {
		r.appLogger.Error("DeletePointFilter error", err)
		return &pb.ResponseDeletePointFilter{CollectionName: req.CollectionName, Status: false}, err
	}

	r.appLogger.Info("rag grpc DeletePointFilter completed", "collection", req.CollectionName, "status", true, "latency_ms", time.Since(startedAt).Milliseconds())
	return &pb.ResponseDeletePointFilter{CollectionName: req.CollectionName, Status: true}, nil
}

func (r *RagService) SearchPoint(ctx context.Context, req *pb.SearchPointRequest) (*pb.ResponseSearchPoint, error) {
	startedAt := time.Now()
	r.appLogger.Info("rag grpc SearchPoint started", "collection", req.CollectionName, "vector_name", req.VectorName, "limit", req.Limit)

	query := ports.SearchQuery{
		CollectionName: req.CollectionName,
		VectorName:     req.VectorName,
		Vector:         req.Vector,
		QueryText:      req.QueryText,
		Limit:          req.Limit,
		WithPayload:    req.WithPayload,
	}
	if req.ScoreThreshold != nil {
		v := req.GetScoreThreshold()
		query.ScoreThreshold = &v
	}
	if req.Filter != nil {
		f := pbFilterToPortsFilter(req.Filter)
		query.Filter = &f
	}

	results, err := r.searchWithVectorDB.Search(ctx, query)
	if err != nil {
		r.appLogger.Error("SearchPoint error", err)
		return &pb.ResponseSearchPoint{CollectionName: req.CollectionName}, err
	}

	items := make([]*pb.SearchResultItem, 0, len(results))
	for _, res := range results {
		item := &pb.SearchResultItem{Score: res.Score}
		if res.Point != nil {
			item.Id = res.Point.ID
			item.Payload = pointPayloadToMap(res.Point.Payload)
		}
		items = append(items, item)
	}
	topScore := float32(-1)
	if len(items) > 0 && items[0] != nil {
		topScore = items[0].Score
	}

	r.appLogger.Info(
		"rag grpc SearchPoint completed",
		"collection", req.CollectionName,
		"vector_name", req.VectorName,
		"result_count", len(items),
		"top_score", topScore,
		"latency_ms", time.Since(startedAt).Milliseconds(),
	)
	return &pb.ResponseSearchPoint{
		CollectionName: req.CollectionName,
		Results:        items,
	}, nil
}

func pbFilterToPortsFilter(f *pb.Filter) ports.Filter {
	if f == nil {
		return ports.Filter{}
	}
	return ports.Filter{
		Must:    pbConditionsToPortsConditions(f.Must),
		Should:  pbConditionsToPortsConditions(f.Should),
		MustNot: pbConditionsToPortsConditions(f.MustNot),
	}
}

func pbConditionsToPortsConditions(conds []*pb.FieldCondition) []ports.FieldCondition {
	out := make([]ports.FieldCondition, 0, len(conds))
	for _, c := range conds {
		fc := ports.FieldCondition{
			Key:      c.Key,
			Operator: ports.MatchOperator(c.Operator),
		}
		switch v := c.ScalarValue.(type) {
		case *pb.FieldCondition_StringValue:
			fc.Value = v.StringValue
		case *pb.FieldCondition_BoolValue:
			fc.Value = v.BoolValue
		case *pb.FieldCondition_IntValue:
			fc.Value = v.IntValue
		default:
			if len(c.StringValues) > 0 {
				fc.Value = c.StringValues
			} else if len(c.IntValues) > 0 {
				fc.Value = c.IntValues
			}
		}
		out = append(out, fc)
	}
	return out
}

func mapToPointPayload(m map[string]string) domain.PointPayload {
	p := domain.PointPayload{}

	p.DocID = m["doc_id"]
	p.SourcePath = m["source_path"]
	p.Modality = m["modality"]
	p.UnitType = m["unit_type"]
	p.Text = m["text"]
	p.OCRText = m["ocr_text"]
	p.ImagePath = m["image_path"]
	p.SectionTitle = m["section_title"]
	p.Lang = m["lang"]
	p.ParentID = m["parent_id"]

	if v, err := strconv.Atoi(m["page"]); err == nil {
		p.Page = v
	}
	if v, err := strconv.Atoi(m["chunk_index"]); err == nil {
		p.ChunkIndex = v
	}
	if v, err := strconv.Atoi(m["token_count"]); err == nil {
		p.TokenCount = v
	}

	if v, err := strconv.ParseBool(m["has_table"]); err == nil {
		p.HasTable = v
	}
	if v, err := strconv.ParseBool(m["has_figure"]); err == nil {
		p.HasFigure = v
	}

	if kw := m["keywords"]; kw != "" {
		p.Keywords = strings.Split(kw, ",")
	}
	if ts := m["created_at"]; ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			p.CreatedAt = t
		}
	}

	return p
}

func pointPayloadToMap(p domain.PointPayload) map[string]string {
	m := make(map[string]string, 18)

	if p.DocID != "" {
		m["doc_id"] = p.DocID
	}
	if p.SourcePath != "" {
		m["source_path"] = p.SourcePath
	}
	if p.Modality != "" {
		m["modality"] = p.Modality
	}
	if p.UnitType != "" {
		m["unit_type"] = p.UnitType
	}
	if p.Text != "" {
		m["text"] = p.Text
	}
	if p.OCRText != "" {
		m["ocr_text"] = p.OCRText
	}
	if p.ImagePath != "" {
		m["image_path"] = p.ImagePath
	}
	if p.SectionTitle != "" {
		m["section_title"] = p.SectionTitle
	}
	if p.Lang != "" {
		m["lang"] = p.Lang
	}
	if p.ParentID != "" {
		m["parent_id"] = p.ParentID
	}

	if p.Page != 0 {
		m["page"] = strconv.Itoa(p.Page)
	}
	if p.ChunkIndex != 0 {
		m["chunk_index"] = strconv.Itoa(p.ChunkIndex)
	}
	if p.TokenCount != 0 {
		m["token_count"] = strconv.Itoa(p.TokenCount)
	}

	if p.HasTable {
		m["has_table"] = "true"
	}
	if p.HasFigure {
		m["has_figure"] = "true"
	}

	if len(p.Keywords) > 0 {
		m["keywords"] = strings.Join(p.Keywords, ",")
	}
	if !p.CreatedAt.IsZero() {
		m["created_at"] = p.CreatedAt.Format(time.RFC3339)
	}

	return m
}
