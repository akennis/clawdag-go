package library

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/wwz16/dagor"
	"github.com/wwz16/dagor/config"
	"github.com/wwz16/dagor/operator"
)

const JSONExtractOpDescription = `JSONExtractOp: extracts a value from a JSON string using a dot-separated path. Numeric path segments index into arrays (e.g. "meals.0.name"). Inputs: JSON *string, Path *string. Output: Value string (JSON-encoded leaf, or "" if not found).`

type JSONExtractOp struct {
	JSON  *string `dag:"input"`
	Path  *string `dag:"input"`
	Value string  `dag:"output"`
}

func (op *JSONExtractOp) Setup(_ *config.Params) error { return nil }
func (op *JSONExtractOp) Reset() error                 { return nil }
func (op *JSONExtractOp) Run(ctx context.Context) error {
	var root any
	if err := json.Unmarshal([]byte(*op.JSON), &root); err != nil {
		return fmt.Errorf("JSONExtractOp: invalid JSON: %w", err)
	}
	parts := strings.Split(*op.Path, ".")
	cur := root
	for _, key := range parts {
		if key == "" {
			continue
		}
		switch container := cur.(type) {
		case map[string]any:
			next, ok := container[key]
			if !ok {
				op.Value = ""
				slog.DebugContext(ctx, "JSONExtractOp.missing_key", "run_id", dagor.RunID(ctx), "key", key)
				return nil
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(key)
			if err != nil || idx < 0 || idx >= len(container) {
				op.Value = ""
				slog.DebugContext(ctx, "JSONExtractOp.invalid_index", "run_id", dagor.RunID(ctx), "key", key, "array_len", len(container))
				return nil
			}
			cur = container[idx]
		default:
			op.Value = ""
			slog.DebugContext(ctx, "JSONExtractOp.non_traversable", "run_id", dagor.RunID(ctx), "path", *op.Path, "type", fmt.Sprintf("%T", cur))
			return nil
		}
	}
	switch v := cur.(type) {
	case string:
		op.Value = v
	default:
		b, _ := json.Marshal(v)
		op.Value = string(b)
	}
	return nil
}

func init() {
	operator.RegisterOp[JSONExtractOp]()
}
