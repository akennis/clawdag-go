# Dagor LLM Hints

Dagor is a high-performance DAG (Directed Acyclic Graph) execution engine for Go. It supports conditional branching, vertex-level parallelism, and parameter-based configuration.

## Operator Implementation

Every operator must implement the `IOperator` interface. Below is a concise example of an operator that adds two integers.

```go
type AddOp struct {
	A      *int `dag:"input"`
	B      *int `dag:"input"`
	Result int  `dag:"output"`
}

func (op *AddOp) Setup(_ *config.Params) error { return nil }
func (op *AddOp) Reset() error                 { return nil }
func (op *AddOp) ResetFields()                 { op.A = nil; op.B = nil; op.Result = 0 }

func (op *AddOp) Run(ctx context.Context) error {
	if op.A == nil || op.B == nil {
		return fmt.Errorf("AddOp: missing required inputs")
	}
	op.Result = *op.A + *op.B
	return nil
}

// Boilerplate for IOperator
func (op *AddOp) InputFields() map[string]any  { return map[string]any{"A": &op.A, "B": &op.B} }
func (op *AddOp) OutputFields() map[string]any { return map[string]any{"Result": &op.Result} }
func (op *AddOp) SetInputField(field string, value any) error {
	if value == nil { return nil }
	ptr, ok := value.(*int)
	if !ok { return fmt.Errorf("AddOp: field %s expected *int", field) }
	if field == "A" { op.A = ptr } else if field == "B" { op.B = ptr }
	return nil
}

func init() {
	operator.RegisterOp[AddOp]()
}
```

## Conditional Branching & Merging

Dagor allows vertices to be executed conditionally using **Predicates**. When multiple conditional branches need to converge, use the `Coalesce` merge strategy.

### 1. Registering Predicates
Predicates are functions that take a map of inputs and return a boolean.

```go
predicate.Register("is_positive", func(inputs map[string]any) bool {
    val, ok := inputs["source_out"].(*int)
    return ok && val != nil && *val > 0
})
```

### 2. Building a Conditional Graph
The `Builder` DSL is used to define the DAG. The example below shows a graph that branches based on a value and coalesces the results.

```go
// source ──► positive_branch (if positive) ──► coalesce (MergeCoalesce)
//        └─► negative_branch (if negative) ──► 
func buildGraph(sourceVal int) (*graph.Graph, error) {
    return graph.NewBuilder("conditional_demo").
        Vertex("source").
        Op("SourceOp").
        Params(map[string]int{"value": sourceVal}).
        Output("out", "source_out").

        Vertex("pos_branch").
        Op("PositiveOp").
        Condition("is_positive"). // Only runs if "is_positive" is true
        Input("in", "source_out").
        Output("out", "pos_out").

        Vertex("neg_branch").
        Op("NegativeOp").
        Condition("is_negative").
        Input("in", "source_out").
        Output("out", "neg_out").

        Vertex("coalesce").
        Op("CoalesceIntOp"). // Built-in: returns the first non-nil input
        Merge(config.MergeCoalesce). // CRITICAL: Prevents "skip" propagation
        Input("A", "pos_out").
        Input("B", "neg_out").
        Output("Result", "final_out").

        Build()
}
```

## Key Concepts

- **Vertex**: A node in the DAG. Has a unique name and an Operator.
- **Operator (`Op`)**: The logic executed at a vertex.
- **Condition**: A named predicate that determines if a vertex should run.
- **Merge (`config.MergeCoalesce`)**: Used when a vertex depends on multiple conditional branches. It ensures that the vertex runs even if some upstream branches are skipped, as long as at least one branch provides data.
- **Input/Output Mapping**: `Input("op_field", "global_name")` and `Output("op_field", "global_name")` map operator fields to global names in the graph's scope.
- **CoalesceOp**: A built-in operator (`CoalesceIntOp`, `CoalesceStringOp`, etc.) that picks the first non-nil input from its branches.

---

# config.Params API — CRITICAL: ALL GETTERS TAKE (path, defaultValue) AND RETURN ONE VALUE
  `config.Params` is passed to `Setup(p *config.Params)`. Every getter returns a single value with
  the supplied default when the key is absent. There is NO two-return-value form.

  SIGNATURES:
    p.GetString(path, defaultValue string) string
    p.GetInt(path string, defaultValue int) int
    p.GetInt64(path string, defaultValue int64) int64
    p.GetFloat64(path string, defaultValue float64) float64
    p.GetBool(path string, defaultValue bool) bool
    p.Exists(path string) bool                    // use when you need to distinguish "absent" from ""
    p.GetArrayString(path string) []string
    p.GetArrayInt64(path string) []int64
    p.GetArrayFloat64(path string) []float64

  CORRECT PATTERNS:
    // optional string — check empty string as sentinel:
    if v := p.GetString("key", ""); v != "" { op.field = v }

    // string with a meaningful default:
    op.field = p.GetString("key", "default_value")

    // int param (from Params(map[string]int{"key": 5})):
    op.count = p.GetInt("key", 0)

    // check existence before reading:
    if p.Exists("key") { op.flag = true }

  WRONG — compile errors:
    if v, ok := p.GetString("key"); ok { ... }    // WRONG: returns 1 value; missing defaultValue arg
    v, ok := p.GetString("key", "")               // WRONG: GetString returns 1 value, not 2

---

# NECESSARY IMPORTS — use these as required:
  "log/slog"                                   // structured logging — REQUIRED (set up in main, used in ops)
  "os"                                         // os.Stderr for slog handler — REQUIRED
  _ "github.com/akennis/clawdag-go/library"    // pre-programmed operations and pre-formed AI nodes
                                               // NOTE: replace the blank _ with a named import when you
                                               // need to embed library.AIComputeOp in a custom op type:
                                               //   clawdag "github.com/akennis/clawdag-go/library"
                                               // A named import also triggers init(), so the blank _ is not needed alongside it.
  _ "github.com/wwz16/dagor/operator/builtin"  // REQUIRED whenever ANY Coalesce*Op is used
  "github.com/panjf2000/ants/v2"               // goroutine worker pool
  "github.com/wwz16/dagor"                     // DAG execution engine
  "github.com/wwz16/dagor/config"              // config.MergeCoalesce — REQUIRED whenever .Merge() is called
  "github.com/wwz16/dagor/graph"               // graph.NewBuilder
  "github.com/wwz16/dagor/predicate"           // only when registering condition predicates
  "github.com/wwz16/dagor/reporter"            // SlogReporter — pass to dagor.WithReporter; REQUIRED
  "flag"                                        // REQUIRED — always present for --mode flag parsing
  "github.com/mark3labs/mcp-go/mcp"            // MCP tool definition — REQUIRED for --mode mcp
  "github.com/mark3labs/mcp-go/server"         // MCP stdio server — REQUIRED for --mode mcp

---

# HOW TO RUN A DAGOR GRAPH — use exactly this pattern:
  pool, _ := ants.NewPool(10); defer pool.Release()
  g, err := buildGraph(sourceVal)
  eng, err := dagor.NewEngine(g, pool, dagor.WithReporter(reporter.New(slog.Default())))
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute); defer cancel()
  if err := eng.Run(ctx); err != nil { log.Fatal(err) }
  raw, ok := eng.GetOutput("wire_name")  // returns (any, bool); cast result to *float64, *string, etc.

---

# LOGGING — structured observability, REQUIRED in all generated programs

## Setup (in main(), once, before pool and engine creation):
  slogLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
  slog.SetDefault(slogLogger)

## Engine creation — pass reporter to every NewEngine call:
  eng, err := dagor.NewEngine(g, pool, dagor.WithReporter(reporter.New(slog.Default())))
  // The reporter emits structured log lines for every graph start/finish and vertex start/finish/skip.
  // It also logs all operator input and output field values (OnVertexFields) — do NOT duplicate these.

## Custom op Run methods — use slog.DebugContext for intermediate state only:
  func (op *FetchDataOp) Run(ctx context.Context) error {
      slog.DebugContext(ctx, "FetchDataOp.run", "run_id", dagor.RunID(ctx))
      // ... call external API ...
      slog.DebugContext(ctx, "FetchDataOp.done", "run_id", dagor.RunID(ctx), "bytes", len(op.Result))
      return nil
  }

## Rules:
  * NEVER use log.Printf in custom op Run methods — use slog.DebugContext for structured, correlated output.
    log.Fatal / log.Fatalf in main() for unrecoverable errors is fine.
  * Always include "run_id", dagor.RunID(ctx) in every slog call inside an op — this ties op log lines
    to the reporter's vertex.start / vertex.finish events for the same execution.
  * Log ONLY intermediate state not captured in fields: e.g. "calling API", "received N bytes", "cache miss".
    The reporter already logs all input/output field values automatically — do not repeat them.
  * Message format: "OpName.event" (e.g. "FetchDataOp.run", "ParseOp.done", "LookupOp.cache_miss").

---

# COALESCE OPS — registered by _ "github.com/wwz16/dagor/operator/builtin" (ALWAYS add this import when using any coalesce vertex):

  2-input (A, B → Result):
    CoalesceStringOp   — first non-nil *string wins
    CoalesceIntOp      — first non-nil *int wins
    CoalesceFloat64Op  — first non-nil *float64 wins
    CoalesceBoolOp     — first non-nil *bool wins

  N-input (Input0…Input(n-1) → Result, requires Params(map[string]int{"n": <count>})):
    CoalesceNStringOp
    CoalesceNIntOp
    CoalesceNFloat64Op
    CoalesceNBoolOp

  RULES:
  * Every coalesce vertex MUST include .Merge(config.MergeCoalesce) — without it the engine
    propagates "skip" from the branch that didn't run and the coalesce vertex never fires.
  * NEVER use CoalesceStringOp (or any Coalesce*Op) without the builtin blank import — the op
    will not be registered and the engine will fail with "operator pool not found".

  Example — merge two conditional string branches:
    Vertex("coalesce_result").
    Op("CoalesceStringOp").
    Merge(config.MergeCoalesce).
    Input("A", "det_result").
    Input("B", "ai_result").
    Output("Result", "final_result").

  Example — merge three conditional string branches:
    Vertex("coalesce_result").
    Op("CoalesceNStringOp").
    Params(map[string]int{"n": 3}).
    Merge(config.MergeCoalesce).
    Input("Input0", "branch_a_result").
    Input("Input1", "branch_b_result").
    Input("Input2", "branch_c_result").
    Output("Result", "final_result").

---

# MAP NODES — fan out a sub-graph over a slice, collect results into []any:
  Use a map vertex when a workflow must apply the same pipeline to each element of a
  runtime-produced slice. Map vertices have NO Op() — they are the fan-out mechanism.

  BUILDER PATTERN:
    Vertex("map_vertex_name").
    Input("Items", "input_slice_wire").       // single slice input
    MapOver("item").                           // item wire name inside the sub-graph
        SubVertex("step1").
            Op("ProcessOp").
            Input("In", "item").              // item injected as *T pointer
            Output("Out", "intermediate").
        SubVertex("step2").
            Op("TransformOp").
            Input("In", "intermediate").
            Output("Result", "result").
        CollectInto("result", "output_wire"). // terminates sub-graph; returns to parent chain

  RULES:
  * Map vertex MUST NOT have Op() set — it replaces the operator.
  * Exactly ONE Input() on the map vertex (the slice).
  * MapOver() argument is the item wire name — available inside the sub-graph only.
  * Each element is injected as a *T pointer; sub-graph operators must type-assert in SetInputField.
  * CollectInto(resultWire, outputWire): resultWire is the sub-graph wire collected per element;
    outputWire is the parent-graph wire written with the assembled []any result.
  * Output is always []any. Read and type-assert downstream:
      raw, ok := eng.GetOutput("output_wire")
      results := raw.([]any)
      for _, v := range results {
          item := v.(string)  // use the concrete element type
      }

---

# CUSTOM AI COMPUTE OPS — DEFINING NEW AICOMPUTEOP VARIANTS:
  AIComputeOp[In, Out] is a generic base type. It CANNOT be registered or used in the graph directly.
  You must embed it in a named concrete struct and register that struct.

  ## Simple case — scalar or slice In/Out:
  No extra interfaces needed. The base type handles float64, int, string, []float64, []string natively.

    type AIComputeStringToFloat64Op struct {
        clawdag.AIComputeOp[string, float64]
    }
    func init() { operator.RegisterOp[AIComputeStringToFloat64Op]() }

  ## Struct output case — only when a true heterogeneous struct is needed:
  When Out is a struct, implement two interfaces on *Out (pointer receiver):

    ExpectedFormat() string          — REPLACES the entire built-in format prompt.
    ParseAIResponse(string) error    — receives the raw AI response string and must populate the struct.

  Example:
    type TicketFields struct {
        L int    `json:"l"`
        O string `json:"o"`
        R int    `json:"r"`
    }
    func (t *TicketFields) ExpectedFormat() string {
        return `Respond with a JSON object only: {"l": <int>, "o": "<string>", "r": <int>}. No explanation.`
    }
    func (t *TicketFields) ParseAIResponse(raw string) error {
        return json.Unmarshal([]byte(raw), t)
    }

    type AIComputeStringToTicketFieldsOp struct {
        clawdag.AIComputeOp[string, TicketFields]
    }
    func init() { operator.RegisterOp[AIComputeStringToTicketFieldsOp]() }

  NOTE: when defining a custom AIComputeOp variant, use a named import for the library package:
    clawdag "github.com/akennis/clawdag-go/library"

---

# KNOWN LIBRARY GAPS — fill these with inline custom ops when needed:
  * **Integer constants**: `ConstOp` outputs `float64` ONLY. When you need a `*int` constant to feed
    `IfIntEqOp`, `IfIntLtOp`, etc., write a custom `IntConstOp` inline (see pattern below). Using
    `ConstOp` for int comparisons causes a runtime type-mismatch error.
  * **String truncation**: no library op caps string length. Write a custom `StringTruncateOp` when
    passing large text (e.g. a fetched README or HTTP body) to AI ops to stay within context limits.

  Minimal `IntConstOp` pattern:
  ```go
  type IntConstOp struct { Result int; value int }
  func (op *IntConstOp) Setup(p *config.Params) error {
      s := p.GetString("Value", "0"); v, err := strconv.Atoi(s)
      if err != nil { return fmt.Errorf("IntConstOp: %w", err) }
      op.value = v; return nil
  }
  func (op *IntConstOp) Reset() error { return nil }
  func (op *IntConstOp) Run(_ context.Context) error { op.Result = op.value; return nil }
  func (op *IntConstOp) InputFields() map[string]any  { return map[string]any{} }
  func (op *IntConstOp) OutputFields() map[string]any { return map[string]any{"Result": &op.Result} }
  func (op *IntConstOp) SetInputField(f string, _ any) error { return fmt.Errorf("no inputs: %s", f) }
  func (op *IntConstOp) ResetFields() { op.Result = 0 }
  func init() { operator.RegisterOp[IntConstOp]() }
  ```

---

# GENERATING NEW DETERMINISTIC OPS ON THE FLY:
  Before using ANY AI op, ask: "can Go code — including a hardcoded dataset — compute this correctly
  every time for a given input?" If yes, write a new deterministic op. There is no complexity limit:
  a 300-line map or a multi-case switch is correct and expected. AI is not a substitute for data.

  These MUST be deterministic ops — NEVER AI calls:
    • any string manipulation: toLower, toUpper, trim, split, join, format, parse
    • any math or logical operation
    • any time/date operation (use the Go `time` package and `time.LoadLocation`)
    • any lookup where the answer comes from known data — write the map inline
    • any normalization or canonicalization (case folding, whitespace, accent stripping)
    • any routing/branching based on known categories or patterns

  How to write a new op inline (place it above main() in the generated file):

  ```go
  type StringToLowerOp struct {
      Value  *string `dag:"input"`
      Result string  `dag:"output"`
  }
  func (op *StringToLowerOp) Setup(_ *config.Config) error { return nil }
  func (op *StringToLowerOp) Reset() error                  { return nil }
  func (op *StringToLowerOp) Run(_ context.Context) error {
      if op.Value != nil { op.Result = strings.ToLower(*op.Value) }
      return nil
  }
  func (op *StringToLowerOp) InputFields() map[string]any  { return map[string]any{"Value": &op.Value} }
  func (op *StringToLowerOp) OutputFields() map[string]any { return map[string]any{"Result": &op.Result} }
  func (op *StringToLowerOp) SetInputField(f string, v any) error {
      if f == "Value" { op.Value = v.(*string); return nil }
      return fmt.Errorf("unknown field %s", f)
  }
  func (op *StringToLowerOp) ResetFields() { op.Value = nil; op.Result = "" }
  func init() { operator.RegisterOp[StringToLowerOp]() }
  ```

  Note: if the op is already in library.md (e.g. StringToLowerOp), use it directly rather than
  re-implementing it. Generate a new op only when the library doesn't cover the need.
