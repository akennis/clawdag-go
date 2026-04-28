package library

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/wwz16/dagor/config"
	"github.com/wwz16/dagor/operator"
)

const JSONExtractOpDescription = `JSONExtractOp: extracts a value from a JSON string using a dot-separated path. Inputs: JSON *string, Path *string. Output: Value string (JSON-encoded leaf, or "" if not found).`

type JSONExtractOp struct {
	JSON  *string `dag:"input"`
	Path  *string `dag:"input"`
	Value string  `dag:"output"`
}

func (op *JSONExtractOp) Setup(_ *config.Params) error { return nil }
func (op *JSONExtractOp) Reset() error                 { return nil }
func (op *JSONExtractOp) Run(_ context.Context) error {
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
		m, ok := cur.(map[string]any)
		if !ok {
			op.Value = ""
			log.Printf("[DEBUG] JSONExtractOp: path %q not found", *op.Path)
			return nil
		}
		cur, ok = m[key]
		if !ok {
			op.Value = ""
			log.Printf("[DEBUG] JSONExtractOp: key %q not found", key)
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
	log.Printf("[DEBUG] JSONExtractOp: path=%q value=%q", *op.Path, op.Value)
	return nil
}

func init() {
	operator.RegisterOp[JSONExtractOp]()
}
