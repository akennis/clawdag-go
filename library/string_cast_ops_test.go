package library

import (
	"context"
	"strings"
	"testing"
)

// ── Typed cast ops ──────────────────────────────────────────────────────────

func TestFloat64ToStringOp_Run(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{3.14, "3.14"},
		{0, "0"},
		{-1.5, "-1.5"},
		{1e6, "1e+06"},
	}
	for _, tc := range cases {
		op := &Float64ToStringOp{Value: floatPtr(tc.in)}
		if err := op.Run(context.Background()); err != nil {
			t.Fatalf("Run(%v): unexpected error: %v", tc.in, err)
		}
		if op.Result != tc.want {
			t.Errorf("Run(%v): got %q, want %q", tc.in, op.Result, tc.want)
		}
	}
}

func TestFloat64ToStringOp_SetInputField_WrongType(t *testing.T) {
	op := &Float64ToStringOp{}
	err := op.SetInputField("Value", 42)
	if err == nil {
		t.Fatal("expected error for wrong type, got nil")
	}
}

func TestFloat64ToStringOp_ResetFields(t *testing.T) {
	op := &Float64ToStringOp{Value: floatPtr(1.0), Result: "1"}
	op.ResetFields()
	if op.Value != nil {
		t.Errorf("expected Value=nil after ResetFields, got %v", op.Value)
	}
	if op.Result != "" {
		t.Errorf("expected Result=\"\" after ResetFields, got %q", op.Result)
	}
}

func TestIntToStringOp_Run(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{42, "42"},
		{0, "0"},
		{-7, "-7"},
	}
	for _, tc := range cases {
		op := &IntToStringOp{Value: intPtr(tc.in)}
		if err := op.Run(context.Background()); err != nil {
			t.Fatalf("Run(%v): unexpected error: %v", tc.in, err)
		}
		if op.Result != tc.want {
			t.Errorf("Run(%v): got %q, want %q", tc.in, op.Result, tc.want)
		}
	}
}

func TestIntToStringOp_SetInputField_WrongType(t *testing.T) {
	op := &IntToStringOp{}
	err := op.SetInputField("Value", "not an int ptr")
	if err == nil {
		t.Fatal("expected error for wrong type, got nil")
	}
}

func TestIntToStringOp_ResetFields(t *testing.T) {
	op := &IntToStringOp{Value: intPtr(5), Result: "5"}
	op.ResetFields()
	if op.Value != nil {
		t.Errorf("expected Value=nil after ResetFields, got %v", op.Value)
	}
	if op.Result != "" {
		t.Errorf("expected Result=\"\" after ResetFields, got %q", op.Result)
	}
}

func TestBoolToStringOp_Run(t *testing.T) {
	trueOp := &BoolToStringOp{Value: boolPtr(true)}
	if err := trueOp.Run(context.Background()); err != nil {
		t.Fatalf("Run(true): unexpected error: %v", err)
	}
	if trueOp.Result != "true" {
		t.Errorf("Run(true): got %q, want \"true\"", trueOp.Result)
	}

	falseOp := &BoolToStringOp{Value: boolPtr(false)}
	if err := falseOp.Run(context.Background()); err != nil {
		t.Fatalf("Run(false): unexpected error: %v", err)
	}
	if falseOp.Result != "false" {
		t.Errorf("Run(false): got %q, want \"false\"", falseOp.Result)
	}
}

func TestBoolToStringOp_SetInputField_WrongType(t *testing.T) {
	op := &BoolToStringOp{}
	err := op.SetInputField("Value", "not a bool ptr")
	if err == nil {
		t.Fatal("expected error for wrong type, got nil")
	}
}

func TestBoolToStringOp_ResetFields(t *testing.T) {
	op := &BoolToStringOp{Value: boolPtr(true), Result: "true"}
	op.ResetFields()
	if op.Value != nil {
		t.Errorf("expected Value=nil after ResetFields, got %v", op.Value)
	}
	if op.Result != "" {
		t.Errorf("expected Result=\"\" after ResetFields, got %q", op.Result)
	}
}

// ── ToStringOp (reflection-based escape hatch) ──────────────────────────────

func TestToStringOp_SetInputField_Float64(t *testing.T) {
	op := &ToStringOp{}
	v := 3.14
	if err := op.SetInputField("Value", &v); err != nil {
		t.Fatalf("SetInputField(*float64): unexpected error: %v", err)
	}
	if err := op.Run(context.Background()); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if op.Result != "3.14" {
		t.Errorf("got %q, want \"3.14\"", op.Result)
	}
}

func TestToStringOp_SetInputField_Int(t *testing.T) {
	op := &ToStringOp{}
	v := 99
	if err := op.SetInputField("Value", &v); err != nil {
		t.Fatalf("SetInputField(*int): unexpected error: %v", err)
	}
	if err := op.Run(context.Background()); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if op.Result != "99" {
		t.Errorf("got %q, want \"99\"", op.Result)
	}
}

func TestToStringOp_SetInputField_Bool(t *testing.T) {
	op := &ToStringOp{}
	v := true
	if err := op.SetInputField("Value", &v); err != nil {
		t.Fatalf("SetInputField(*bool): unexpected error: %v", err)
	}
	if err := op.Run(context.Background()); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if op.Result != "true" {
		t.Errorf("got %q, want \"true\"", op.Result)
	}
}

func TestToStringOp_SetInputField_Struct(t *testing.T) {
	type point struct{ X, Y int }
	op := &ToStringOp{}
	v := point{X: 3, Y: 7}
	if err := op.SetInputField("Value", &v); err != nil {
		t.Fatalf("SetInputField(*struct): unexpected error: %v", err)
	}
	if err := op.Run(context.Background()); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if !strings.Contains(op.Result, "3") || !strings.Contains(op.Result, "7") {
		t.Errorf("expected Result to contain struct fields, got %q", op.Result)
	}
}

func TestToStringOp_SetInputField_NonPointer(t *testing.T) {
	op := &ToStringOp{}
	err := op.SetInputField("Value", 42)
	if err == nil {
		t.Fatal("expected error for non-pointer value, got nil")
	}
	if !strings.Contains(err.Error(), "expected a pointer") {
		t.Errorf("expected error to mention 'expected a pointer', got %q", err.Error())
	}
}

func TestToStringOp_SetInputField_UnknownField(t *testing.T) {
	op := &ToStringOp{}
	v := 1.0
	err := op.SetInputField("Unknown", &v)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestToStringOp_ResetFields(t *testing.T) {
	op := &ToStringOp{Result: "hello"}
	v := any(42)
	op.Value = &v
	op.ResetFields()
	if op.Value != nil {
		t.Errorf("expected Value=nil after ResetFields, got %v", op.Value)
	}
	if op.Result != "" {
		t.Errorf("expected Result=\"\" after ResetFields, got %q", op.Result)
	}
}
