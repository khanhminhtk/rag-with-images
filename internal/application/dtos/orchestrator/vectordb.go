package orchestrator

type CollectionVectorConfig struct {
	Name     string `json:"name"`
	Size     uint64 `json:"size"`
	Distance string `json:"distance"`
}

type CreateCollectionRequest struct {
	Name              string                   `json:"name"`
	Vectors           []CollectionVectorConfig `json:"vectors"`
	Shards            uint32                   `json:"shards,omitempty"`
	ReplicationFactor uint32                   `json:"replication_factor,omitempty"`
	OnDiskPayload     bool                     `json:"on_disk_payload,omitempty"`
	OptimizersMemmap  bool                     `json:"optimizers_memmap,omitempty"`
}

type CreateCollectionResponse struct {
	Name   string `json:"name"`
	Status bool   `json:"status"`
}

type DeleteCollectionRequest struct {
	Name string `json:"name"`
}

type DeleteCollectionResponse struct {
	Name   string `json:"name"`
	Status bool   `json:"status"`
}

type FieldCondition struct {
	Key          string   `json:"key"`
	Operator     string   `json:"operator"`
	StringValue  *string  `json:"string_value,omitempty"`
	BoolValue    *bool    `json:"bool_value,omitempty"`
	IntValue     *int64   `json:"int_value,omitempty"`
	StringValues []string `json:"string_values,omitempty"`
	IntValues    []int64  `json:"int_values,omitempty"`
}

type Filter struct {
	Must    []FieldCondition `json:"must,omitempty"`
	Should  []FieldCondition `json:"should,omitempty"`
	MustNot []FieldCondition `json:"must_not,omitempty"`
}

type DeletePointFilterRequest struct {
	CollectionName string `json:"collection_name"`
	Filter         Filter `json:"filter"`
}

type DeletePointFilterResponse struct {
	CollectionName string `json:"collection_name"`
	Status         bool   `json:"status"`
}
