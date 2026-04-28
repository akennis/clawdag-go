package library

import (
	"context"
	"strings"
	"testing"

	"github.com/wwz16/dagor/config"
	"github.com/wwz16/dagor/graph"
	"github.com/wwz16/dagor/predicate"
)

func boolPtr(v bool) *bool { return &v }

// ───────────────────────────── CoalesceStringOp ──────────────────────────────

func TestCoalesceStringOp_FirstSet(t *testing.T) {
	a := "first"
	b := "second"
	op := &CoalesceStringOp{}
	op.A = &a
	op.B = &b
	if err := op.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if op.Result != "first" {
		t.Errorf("Result = %q, want %q", op.Result, "first")
	}
}

func TestCoalesceStringOp_ThirdSet(t *testing.T) {
	c := "third"
	d := "fourth"
	op := &CoalesceStringOp{}
	op.C = &c
	op.D = &d
	if err := op.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if op.Result != "third" {
		t.Errorf("Result = %q, want %q", op.Result, "third")
	}
}

func TestCoalesceStringOp_AllNil(t *testing.T) {
	op := &CoalesceStringOp{}
	err := op.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when all inputs nil")
	}
	if !strings.Contains(err.Error(), "all inputs nil") {
		t.Errorf("error %q should mention 'all inputs nil'", err.Error())
	}
}

// ──────────────────────── Other concrete variants ────────────────────────────

func TestCoalesceFloat64Op_SecondSet(t *testing.T) {
	b := 4.2
	op := &CoalesceFloat64Op{}
	op.B = &b
	if err := op.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if op.Result != 4.2 {
		t.Errorf("Result = %v, want 4.2", op.Result)
	}
}

func TestCoalesceIntOp_FourthSet(t *testing.T) {
	d := 42
	op := &CoalesceIntOp{}
	op.D = &d
	if err := op.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if op.Result != 42 {
		t.Errorf("Result = %v, want 42", op.Result)
	}
}

func TestCoalesceBoolOp_FirstSet(t *testing.T) {
	a := true
	op := &CoalesceBoolOp{}
	op.A = &a
	if err := op.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !op.Result {
		t.Errorf("Result = %v, want true", op.Result)
	}
}

func TestCoalesceIntOp_AllNil(t *testing.T) {
	op := &CoalesceIntOp{}
	if err := op.Run(context.Background()); err == nil {
		t.Fatal("expected error when all inputs nil")
	}
}

// ─────────────────────────── IOperator boilerplate ───────────────────────────

func TestCoalesceOp_InputFields(t *testing.T) {
	op := &CoalesceStringOp{}
	fields := op.InputFields()
	for _, name := range []string{"A", "B", "C", "D"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("missing input %s", name)
		}
	}
}

func TestCoalesceOp_OutputFields(t *testing.T) {
	op := &CoalesceFloat64Op{}
	if _, ok := op.OutputFields()["Result"]; !ok {
		t.Error("missing output Result")
	}
}

func TestCoalesceOp_SetInputField(t *testing.T) {
	op := &CoalesceStringOp{}
	v := "hi"
	if err := op.SetInputField("B", &v); err != nil {
		t.Fatalf("SetInputField B: %v", err)
	}
	if op.B == nil || *op.B != "hi" {
		t.Errorf("B not set; got %v", op.B)
	}
}

func TestCoalesceOp_SetInputField_WrongType(t *testing.T) {
	op := &CoalesceStringOp{}
	if err := op.SetInputField("A", 42); err == nil {
		t.Error("expected error for wrong-typed value")
	}
}

func TestCoalesceOp_SetInputField_UnknownField(t *testing.T) {
	op := &CoalesceStringOp{}
	v := "x"
	if err := op.SetInputField("Z", &v); err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestCoalesceOp_ResetFields(t *testing.T) {
	a := "a"
	b := "b"
	op := &CoalesceStringOp{}
	op.A = &a
	op.B = &b
	op.Result = "x"
	op.ResetFields()
	if op.A != nil || op.B != nil || op.C != nil || op.D != nil {
		t.Errorf("ResetFields did not nil pointers: %+v", op)
	}
	if op.Result != "" {
		t.Errorf("ResetFields Result = %q, want \"\"", op.Result)
	}
}

func TestCoalesceOp_SetupReset(t *testing.T) {
	op := &CoalesceIntOp{}
	if err := op.Setup(mustParams(t, map[string]string{})); err != nil {
		t.Errorf("Setup: %v", err)
	}
	if err := op.Reset(); err != nil {
		t.Errorf("Reset: %v", err)
	}
}

// ─────────────────────────── Description constants ───────────────────────────

func TestCoalesceDescriptions_NonEmpty(t *testing.T) {
	descs := map[string]string{
		"CoalesceStringOp":  CoalesceStringOpDescription,
		"CoalesceFloat64Op": CoalesceFloat64OpDescription,
		"CoalesceIntOp":     CoalesceIntOpDescription,
		"CoalesceBoolOp":    CoalesceBoolOpDescription,
	}
	for name, desc := range descs {
		if desc == "" {
			t.Errorf("%s description is empty", name)
		}
		if !strings.HasPrefix(desc, name) {
			t.Errorf("%s description should start with op name, got %q", name, desc)
		}
	}
}

// ───────────────────────────── Integration test ──────────────────────────────
//
// TestCoalesceOp_Integration_ConditionalMerge mirrors the merge example in
// llm-hints.md: two conditional branches feed into a CoalesceIntOp gated with
// MergeCoalesce. The "is_positive" branch runs and provides A; the negative
// branch is skipped, so B is nil. The coalesce should still execute (thanks
// to MergeCoalesce) and pick A.

func TestCoalesceOp_Integration_ConditionalMerge(t *testing.T) {
	const posPred = "test_coalesce_int_is_positive"
	const negPred = "test_coalesce_int_is_negative"
	predicate.Unregister(posPred)
	predicate.Unregister(negPred)
	if err := predicate.Register(posPred, func(inputs map[string]any) bool {
		v, ok := inputs["src_val"].(*float64)
		return ok && v != nil && *v > 0
	}); err != nil {
		t.Fatalf("predicate.Register pos: %v", err)
	}
	defer predicate.Unregister(posPred)
	if err := predicate.Register(negPred, func(inputs map[string]any) bool {
		v, ok := inputs["src_val"].(*float64)
		return ok && v != nil && *v < 0
	}); err != nil {
		t.Fatalf("predicate.Register neg: %v", err)
	}
	defer predicate.Unregister(negPred)

	// Source: 5 (positive). Pos branch: emit ConstOp(100). Neg branch: ConstOp(-100).
	// Coalesce picks A (pos branch's int output).
	g, err := graph.NewBuilder("coalesce_int_demo").
		Vertex("src").Op("ConstOp").
		Params(map[string]string{"Value": "5"}).
		Output("Result", "src_val").
		// Positive branch — runs (5 > 0).
		Vertex("pos").Op("ConstOp").
		Condition(posPred).
		ConditionInput("src_val").
		Params(map[string]string{"Value": "100"}).
		Output("Result", "pos_out").
		// Negative branch — skipped (5 not < 0).
		Vertex("neg").Op("ConstOp").
		Condition(negPred).
		ConditionInput("src_val").
		Params(map[string]string{"Value": "-100"}).
		Output("Result", "neg_out").
		// Coalesce — must run despite neg being skipped.
		Vertex("coalesce").Op("CoalesceFloat64Op").
		Merge(config.MergeCoalesce).
		Input("A", "pos_out").Input("B", "neg_out").
		Output("Result", "final").
		Build()
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	eng := runGraph(t, g)
	raw, ok := eng.GetOutput("final")
	if !ok {
		t.Fatal("final wire missing")
	}
	v, ok := raw.(*float64)
	if !ok || v == nil {
		t.Fatalf("final: expected *float64, got %T", raw)
	}
	if *v != 100 {
		t.Errorf("final = %v, want 100", *v)
	}
}

// TestCoalesceOp_Integration_StringVariant exercises the registered op-name
// resolution path for the string variant.
func TestCoalesceOp_Integration_StringVariant(t *testing.T) {
	const truePred = "test_coalesce_string_always_true"
	const falsePred = "test_coalesce_string_always_false"
	predicate.Unregister(truePred)
	predicate.Unregister(falsePred)
	if err := predicate.Register(truePred, func(_ map[string]any) bool { return true }); err != nil {
		t.Fatalf("register true: %v", err)
	}
	defer predicate.Unregister(truePred)
	if err := predicate.Register(falsePred, func(_ map[string]any) bool { return false }); err != nil {
		t.Fatalf("register false: %v", err)
	}
	defer predicate.Unregister(falsePred)

	g, err := graph.NewBuilder("coalesce_string_demo").
		Vertex("a_branch").Op("StringConstOp").
		Condition(truePred).
		Params(map[string]string{"Value": "alpha"}).
		Output("Result", "a_out").
		Vertex("b_branch").Op("StringConstOp").
		Condition(falsePred).
		Params(map[string]string{"Value": "beta"}).
		Output("Result", "b_out").
		Vertex("coalesce").Op("CoalesceStringOp").
		Merge(config.MergeCoalesce).
		Input("A", "a_out").Input("B", "b_out").
		Output("Result", "final").
		Build()
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	eng := runGraph(t, g)
	raw, ok := eng.GetOutput("final")
	if !ok {
		t.Fatal("final wire missing")
	}
	v, ok := raw.(*string)
	if !ok || v == nil {
		t.Fatalf("final: expected *string, got %T", raw)
	}
	if *v != "alpha" {
		t.Errorf("final = %q, want %q", *v, "alpha")
	}
}

// keep boolPtr referenced even if a refactor drops a use.
var _ = boolPtr
