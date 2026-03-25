package qdrant

import (
	"fmt"
	"math"

	"rag_imagetotext_texttoimage/internal/application/ports"

	"github.com/qdrant/go-client/qdrant"
)

func toQdrantFilter(f *ports.Filter) (*qdrant.Filter, error) {
	source := qdrantSource("toQdrantFilter")
	if f == nil || f.IsEmpty() {
		return nil, nil
	}

	must, err := toQdrantConditions(f.Must)
	if err != nil {
		return nil, fmt.Errorf("%s: must: %w", source, err)
	}
	should, err := toQdrantConditions(f.Should)
	if err != nil {
		return nil, fmt.Errorf("%s: should: %w", source, err)
	}
	mustNot, err := toQdrantConditions(f.MustNot)
	if err != nil {
		return nil, fmt.Errorf("%s: must_not: %w", source, err)
	}

	return &qdrant.Filter{
		Must:    must,
		Should:  should,
		MustNot: mustNot,
	}, nil
}

func toQdrantConditions(conditions []ports.FieldCondition) ([]*qdrant.Condition, error) {
	source := qdrantSource("toQdrantConditions")
	out := make([]*qdrant.Condition, 0, len(conditions))
	for idx, c := range conditions {
		cond, err := toQdrantCondition(c)
		if err != nil {
			return nil, fmt.Errorf("%s: condition[%d] key=%q: %w", source, idx, c.Key, err)
		}
		out = append(out, cond)
	}
	return out, nil
}

func toQdrantCondition(c ports.FieldCondition) (*qdrant.Condition, error) {
	source := qdrantSource("toQdrantCondition")
	if c.Key == "" {
		return nil, fmt.Errorf("%s: empty key", source)
	}

	switch c.Operator {
	case ports.MatchOperatorEqual:
		switch v := c.Value.(type) {
		case string:
			return qdrant.NewMatchKeyword(c.Key, v), nil
		case bool:
			return qdrant.NewMatchBool(c.Key, v), nil
		case int:
			return qdrant.NewMatchInt(c.Key, int64(v)), nil
		case int64:
			return qdrant.NewMatchInt(c.Key, v), nil
		case uint:
			if uint64(v) > math.MaxInt64 {
				return nil, fmt.Errorf("%s: uint value out of int64 range: %d", source, v)
			}
			return qdrant.NewMatchInt(c.Key, int64(v)), nil
		case uint64:
			if v > math.MaxInt64 {
				return nil, fmt.Errorf("%s: uint64 value out of int64 range: %d", source, v)
			}
			return qdrant.NewMatchInt(c.Key, int64(v)), nil
		default:
			return nil, fmt.Errorf("%s: eq unsupported type: %T", source, c.Value)
		}

	case ports.MatchOperatorIn:
		switch v := c.Value.(type) {
		case []string:
			if len(v) == 0 {
				return nil, fmt.Errorf("%s: in requires non-empty []string", source)
			}
			return qdrant.NewMatchKeywords(c.Key, v...), nil
		case []int:
			if len(v) == 0 {
				return nil, fmt.Errorf("%s: in requires non-empty []int", source)
			}
			ints := make([]int64, 0, len(v))
			for _, item := range v {
				ints = append(ints, int64(item))
			}
			return qdrant.NewMatchInts(c.Key, ints...), nil
		case []int64:
			if len(v) == 0 {
				return nil, fmt.Errorf("%s: in requires non-empty []int64", source)
			}
			return qdrant.NewMatchInts(c.Key, v...), nil
		case []uint64:
			if len(v) == 0 {
				return nil, fmt.Errorf("%s: in requires non-empty []uint64", source)
			}
			ints := make([]int64, 0, len(v))
			for _, item := range v {
				if item > math.MaxInt64 {
					return nil, fmt.Errorf("%s: uint64 item out of int64 range: %d", source, item)
				}
				ints = append(ints, int64(item))
			}
			return qdrant.NewMatchInts(c.Key, ints...), nil
		case []any:
			if len(v) == 0 {
				return nil, fmt.Errorf("%s: in requires non-empty []any", source)
			}
			var strs []string
			var ints []int64
			for _, item := range v {
				switch x := item.(type) {
				case string:
					strs = append(strs, x)
				case int:
					ints = append(ints, int64(x))
				case int64:
					ints = append(ints, x)
				case uint64:
					if x > math.MaxInt64 {
						return nil, fmt.Errorf("%s: uint64 item out of int64 range: %d", source, x)
					}
					ints = append(ints, int64(x))
				default:
					return nil, fmt.Errorf("%s: in unsupported item type: %T", source, item)
				}
			}
			if len(strs) > 0 && len(ints) > 0 {
				return nil, fmt.Errorf("%s: in mixed value types (string + integer) are not supported", source)
			}
			if len(strs) > 0 {
				return qdrant.NewMatchKeywords(c.Key, strs...), nil
			}
			if len(ints) > 0 {
				return qdrant.NewMatchInts(c.Key, ints...), nil
			}
			return nil, fmt.Errorf("%s: in has no supported values", source)
		default:
			return nil, fmt.Errorf("%s: in unsupported type: %T", source, c.Value)
		}
	default:
		return nil, fmt.Errorf("%s: unsupported operator: %q", source, c.Operator)
	}
}
