package library

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/wwz16/dagor/config"
	"github.com/wwz16/dagor/operator"
)

// MathOperands aggregates two float64 inputs for AI-powered binary numeric operations.
type MathOperands struct{ A, B float64 }

// FormatForPrompt implements AIInputFormatter.
func (m MathOperands) FormatForPrompt() string {
	return fmt.Sprintf("A=%v, B=%v", m.A, m.B)
}

const ConstOpDescription = "ConstOp: injects a constant float64. Params: Value string (e.g. \"3.0\"). Output: Result float64."
const AddOpDescription = "AddOp: deterministic addition. Inputs: A *float64, B *float64. Output: Result float64."
const SubOpDescription = "SubOp: A minus B. Inputs: A *float64, B *float64. Output: Result float64."
const DivOpDescription = "DivOp: A divided by B. Inputs: A *float64, B *float64. Output: Result float64. Error if B==0."

type AddOp struct {
	A      *float64 `dag:"input"`
	B      *float64 `dag:"input"`
	Result float64  `dag:"output"`
}

func (op *AddOp) Setup(params *config.Params) error { return nil }
func (op *AddOp) Reset() error                      { return nil }
func (op *AddOp) Run(ctx context.Context) error {
	op.Result = *op.A + *op.B
	log.Printf("[DEBUG] AddOp: %v + %v = %v", *op.A, *op.B, op.Result)
	return nil
}

type SubOp struct {
	A      *float64 `dag:"input"`
	B      *float64 `dag:"input"`
	Result float64  `dag:"output"`
}

func (op *SubOp) Setup(params *config.Params) error { return nil }
func (op *SubOp) Reset() error                      { return nil }
func (op *SubOp) Run(ctx context.Context) error {
	op.Result = *op.A - *op.B
	log.Printf("[DEBUG] SubOp: %v - %v = %v", *op.A, *op.B, op.Result)
	return nil
}

type DivOp struct {
	A      *float64 `dag:"input"`
	B      *float64 `dag:"input"`
	Result float64  `dag:"output"`
}

func (op *DivOp) Setup(params *config.Params) error { return nil }
func (op *DivOp) Reset() error                      { return nil }
func (op *DivOp) Run(ctx context.Context) error {
	if *op.B == 0 {
		return fmt.Errorf("division by zero")
	}
	op.Result = *op.A / *op.B
	log.Printf("[DEBUG] DivOp: %v / %v = %v", *op.A, *op.B, op.Result)
	return nil
}

type ConstOp struct {
	Result float64 `dag:"output"`
	value  float64
}

func (op *ConstOp) Setup(params *config.Params) error {
	s := params.GetString("Value", "0")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("ConstOp: invalid Value %q: %w", s, err)
	}
	op.value = v
	return nil
}
func (op *ConstOp) Reset() error { return nil }
func (op *ConstOp) Run(ctx context.Context) error {
	op.Result = op.value
	log.Printf("[DEBUG] ConstOp: value=%v", op.Result)
	return nil
}

const PackMathOperandsOpDescription = "PackMathOperandsOp: packs two float64 inputs into a MathOperands struct. Inputs: A *float64, B *float64. Output: Result MathOperands."
const AIComputeMathOperandsToFloat64OpDescription = `AIComputeMathOperandsToFloat64Op: AI-powered fallback for operations not available in the library.
  Params:   operation string — plain-English description of what to compute (e.g. "multiply A by B").
            max_retries string — number of parse retries (default "3").
  Inputs:   Input *MathOperands (connect PackMathOperandsOp's Result wire).
  Outputs:  Result float64, Reasoning string.`

// PackMathOperandsOp packs two scalar float64 inputs into a single MathOperands struct.
type PackMathOperandsOp struct {
	A      *float64     `dag:"input"`
	B      *float64     `dag:"input"`
	Result MathOperands `dag:"output"`
}

func (op *PackMathOperandsOp) Setup(params *config.Params) error { return nil }
func (op *PackMathOperandsOp) Reset() error                      { return nil }
func (op *PackMathOperandsOp) Run(ctx context.Context) error {
	op.Result = MathOperands{A: *op.A, B: *op.B}
	log.Printf("[DEBUG] PackMathOperandsOp: A=%v B=%v", op.Result.A, op.Result.B)
	return nil
}

// AIComputeMathOperandsToFloat64Op is the registered concrete variant of AIComputeOp
// for binary float64 math operations.
type AIComputeMathOperandsToFloat64Op struct {
	AIComputeOp[MathOperands, float64]
}

func init() {
	operator.RegisterOp[ConstOp]()
	operator.RegisterOp[AddOp]()
	operator.RegisterOp[SubOp]()
	operator.RegisterOp[DivOp]()
	operator.RegisterOp[PackMathOperandsOp]()
	operator.RegisterOp[AIComputeMathOperandsToFloat64Op]()
}
