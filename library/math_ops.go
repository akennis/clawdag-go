package library

import (
	"context"
	"fmt"
	"log"
	"math"

	"github.com/wwz16/dagor/config"
	"github.com/wwz16/dagor/operator"
)

// MathOperands aggregates two float64 inputs for AI-powered binary numeric operations.
type MathOperands struct{ A, B float64 }

// FormatForPrompt implements AIInputFormatter.
func (m MathOperands) FormatForPrompt() string {
	return fmt.Sprintf("A=%v, B=%v", m.A, m.B)
}

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

const MulOpDescription = "MulOp: A multiplied by B. Inputs: A *float64, B *float64. Output: Result float64."
const RoundOpDescription = "RoundOp: rounds Value to nearest integer. Input: Value *float64. Output: Result float64."
const ClampOpDescription = "ClampOp: clamps Value to [Min, Max]. Inputs: Value *float64, Min *float64, Max *float64. Output: Result float64."
const SumOpDescription = "SumOp: sums all values in a slice. Input: Values *[]float64. Output: Result float64."
const MinOpDescription = "MinOp: returns the minimum value in a slice. Input: Values *[]float64. Output: Result float64. Error if empty."
const MaxOpDescription = "MaxOp: returns the maximum value in a slice. Input: Values *[]float64. Output: Result float64. Error if empty."

type MulOp struct {
	A      *float64 `dag:"input"`
	B      *float64 `dag:"input"`
	Result float64  `dag:"output"`
}

func (op *MulOp) Setup(_ *config.Params) error { return nil }
func (op *MulOp) Reset() error                 { return nil }
func (op *MulOp) Run(_ context.Context) error {
	op.Result = *op.A * *op.B
	log.Printf("[DEBUG] MulOp: %v * %v = %v", *op.A, *op.B, op.Result)
	return nil
}

type RoundOp struct {
	Value  *float64 `dag:"input"`
	Result float64  `dag:"output"`
}

func (op *RoundOp) Setup(_ *config.Params) error { return nil }
func (op *RoundOp) Reset() error                 { return nil }
func (op *RoundOp) Run(_ context.Context) error {
	op.Result = math.Round(*op.Value)
	log.Printf("[DEBUG] RoundOp: round(%v) = %v", *op.Value, op.Result)
	return nil
}

type ClampOp struct {
	Value  *float64 `dag:"input"`
	Min    *float64 `dag:"input"`
	Max    *float64 `dag:"input"`
	Result float64  `dag:"output"`
}

func (op *ClampOp) Setup(_ *config.Params) error { return nil }
func (op *ClampOp) Reset() error                 { return nil }
func (op *ClampOp) Run(_ context.Context) error {
	v := *op.Value
	if v < *op.Min {
		v = *op.Min
	} else if v > *op.Max {
		v = *op.Max
	}
	op.Result = v
	log.Printf("[DEBUG] ClampOp: clamp(%v,[%v,%v]) = %v", *op.Value, *op.Min, *op.Max, op.Result)
	return nil
}

type SumOp struct {
	Values *[]float64
	Result float64
}

func (op *SumOp) Setup(_ *config.Params) error { return nil }
func (op *SumOp) Reset() error                 { return nil }
func (op *SumOp) Run(_ context.Context) error {
	var sum float64
	for _, v := range *op.Values {
		sum += v
	}
	op.Result = sum
	log.Printf("[DEBUG] SumOp: sum=%v", op.Result)
	return nil
}
func (op *SumOp) InputFields() map[string]any { return map[string]any{"Values": &op.Values} }
func (op *SumOp) OutputFields() map[string]any { return map[string]any{"Result": &op.Result} }
func (op *SumOp) SetInputField(field string, value any) error {
	if field != "Values" {
		return fmt.Errorf("field %s is not defined", field)
	}
	val, ok := value.(*[]float64)
	if !ok {
		return fmt.Errorf("field Values: expected *[]float64, got %T", value)
	}
	op.Values = val
	return nil
}
func (op *SumOp) ResetFields() { op.Values = nil; op.Result = 0 }

type MinOp struct {
	Values *[]float64
	Result float64
}

func (op *MinOp) Setup(_ *config.Params) error { return nil }
func (op *MinOp) Reset() error                 { return nil }
func (op *MinOp) Run(_ context.Context) error {
	if len(*op.Values) == 0 {
		return fmt.Errorf("MinOp: empty slice")
	}
	m := (*op.Values)[0]
	for _, v := range (*op.Values)[1:] {
		if v < m {
			m = v
		}
	}
	op.Result = m
	log.Printf("[DEBUG] MinOp: min=%v", op.Result)
	return nil
}
func (op *MinOp) InputFields() map[string]any { return map[string]any{"Values": &op.Values} }
func (op *MinOp) OutputFields() map[string]any { return map[string]any{"Result": &op.Result} }
func (op *MinOp) SetInputField(field string, value any) error {
	if field != "Values" {
		return fmt.Errorf("field %s is not defined", field)
	}
	val, ok := value.(*[]float64)
	if !ok {
		return fmt.Errorf("field Values: expected *[]float64, got %T", value)
	}
	op.Values = val
	return nil
}
func (op *MinOp) ResetFields() { op.Values = nil; op.Result = 0 }

type MaxOp struct {
	Values *[]float64
	Result float64
}

func (op *MaxOp) Setup(_ *config.Params) error { return nil }
func (op *MaxOp) Reset() error                 { return nil }
func (op *MaxOp) Run(_ context.Context) error {
	if len(*op.Values) == 0 {
		return fmt.Errorf("MaxOp: empty slice")
	}
	m := (*op.Values)[0]
	for _, v := range (*op.Values)[1:] {
		if v > m {
			m = v
		}
	}
	op.Result = m
	log.Printf("[DEBUG] MaxOp: max=%v", op.Result)
	return nil
}
func (op *MaxOp) InputFields() map[string]any { return map[string]any{"Values": &op.Values} }
func (op *MaxOp) OutputFields() map[string]any { return map[string]any{"Result": &op.Result} }
func (op *MaxOp) SetInputField(field string, value any) error {
	if field != "Values" {
		return fmt.Errorf("field %s is not defined", field)
	}
	val, ok := value.(*[]float64)
	if !ok {
		return fmt.Errorf("field Values: expected *[]float64, got %T", value)
	}
	op.Values = val
	return nil
}
func (op *MaxOp) ResetFields() { op.Values = nil; op.Result = 0 }

func init() {
	operator.RegisterOp[AddOp]()
	operator.RegisterOp[SubOp]()
	operator.RegisterOp[DivOp]()
	operator.RegisterOp[MulOp]()
	operator.RegisterOp[RoundOp]()
	operator.RegisterOp[ClampOp]()
	operator.RegisterOp[SumOp]()
	operator.RegisterOp[MinOp]()
	operator.RegisterOp[MaxOp]()
	operator.RegisterOp[PackMathOperandsOp]()
	operator.RegisterOp[AIComputeMathOperandsToFloat64Op]()
}
