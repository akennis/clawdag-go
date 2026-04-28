package library

import (
	"context"
	"fmt"
	"log"

	"github.com/wwz16/dagor/config"
	"github.com/wwz16/dagor/operator"
)

// CoalesceOp picks the first non-nil input among A, B, C, D.
//
// Hand-rolled (generics + dag tags don't mix). The IOperator boilerplate is
// implemented on the generic *CoalesceOp[T] receiver and promoted to each
// concrete variant (e.g. CoalesceStringOp) via embedding.
//
// Use with config.MergeCoalesce when merging conditional branches; see the
// example in llm-hints.md.
type CoalesceOp[T any] struct {
	A, B, C, D *T
	Result     T
}

func (op *CoalesceOp[T]) Setup(_ *config.Params) error { return nil }
func (op *CoalesceOp[T]) Reset() error                 { return nil }

func (op *CoalesceOp[T]) Run(_ context.Context) error {
	switch {
	case op.A != nil:
		op.Result = *op.A
	case op.B != nil:
		op.Result = *op.B
	case op.C != nil:
		op.Result = *op.C
	case op.D != nil:
		op.Result = *op.D
	default:
		return fmt.Errorf("CoalesceOp: all inputs nil")
	}
	log.Printf("[DEBUG] CoalesceOp[%T]: result=%v", op.Result, op.Result)
	return nil
}

func (op *CoalesceOp[T]) InputFields() map[string]any {
	return map[string]any{
		"A": &op.A,
		"B": &op.B,
		"C": &op.C,
		"D": &op.D,
	}
}

func (op *CoalesceOp[T]) OutputFields() map[string]any {
	return map[string]any{"Result": &op.Result}
}

func (op *CoalesceOp[T]) SetInputField(field string, value any) error {
	val, ok := value.(*T)
	if !ok {
		var zero *T
		return fmt.Errorf("field %s: expected %T, got %T", field, zero, value)
	}
	switch field {
	case "A":
		op.A = val
	case "B":
		op.B = val
	case "C":
		op.C = val
	case "D":
		op.D = val
	default:
		return fmt.Errorf("field %s is not defined", field)
	}
	return nil
}

func (op *CoalesceOp[T]) ResetFields() {
	var zeroPtr *T
	op.A = zeroPtr
	op.B = zeroPtr
	op.C = zeroPtr
	op.D = zeroPtr
	var zeroVal T
	op.Result = zeroVal
}

// Concrete registered variants. Each is a thin wrapper around CoalesceOp[T]
// so the global operator registry can resolve them by name.

type CoalesceStringOp struct {
	CoalesceOp[string]
}

type CoalesceFloat64Op struct {
	CoalesceOp[float64]
}

type CoalesceIntOp struct {
	CoalesceOp[int]
}

type CoalesceBoolOp struct {
	CoalesceOp[bool]
}

const CoalesceStringOpDescription = `CoalesceStringOp: returns the first non-nil input among A, B, C, D.
  Inputs:  A *string, B *string, C *string, D *string (any may be nil).
  Output:  Result string.
  Errors:  if all four inputs are nil.
  Use with Merge(config.MergeCoalesce) to merge multiple conditional branches.`

const CoalesceFloat64OpDescription = `CoalesceFloat64Op: returns the first non-nil input among A, B, C, D.
  Inputs:  A *float64, B *float64, C *float64, D *float64 (any may be nil).
  Output:  Result float64.
  Errors:  if all four inputs are nil.
  Use with Merge(config.MergeCoalesce) to merge multiple conditional branches.`

const CoalesceIntOpDescription = `CoalesceIntOp: returns the first non-nil input among A, B, C, D.
  Inputs:  A *int, B *int, C *int, D *int (any may be nil).
  Output:  Result int.
  Errors:  if all four inputs are nil.
  Use with Merge(config.MergeCoalesce) to merge multiple conditional branches.`

const CoalesceBoolOpDescription = `CoalesceBoolOp: returns the first non-nil input among A, B, C, D.
  Inputs:  A *bool, B *bool, C *bool, D *bool (any may be nil).
  Output:  Result bool.
  Errors:  if all four inputs are nil.
  Use with Merge(config.MergeCoalesce) to merge multiple conditional branches.`

func init() {
	operator.RegisterOp[CoalesceStringOp]()
	operator.RegisterOp[CoalesceFloat64Op]()
	operator.RegisterOp[CoalesceIntOp]()
	operator.RegisterOp[CoalesceBoolOp]()
}
