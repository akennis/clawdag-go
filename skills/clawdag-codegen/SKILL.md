---
name: clawdag-codegen
description: Generate a compilable Go workflow executable from an approved clawdag-go DAG design
version: 0.1.0
library_version: github.com/akennis/clawdag-go v0.1.0
triggers: [clawdag codegen, generate workflow code, implement dag design]
input:
  design:     {type: string, description: "Approved DAG design (output of clawdag-design)", required: true}
  output_dir: {type: string, description: "Directory to write generated Go program", required: true}
  task:       {type: string, description: "Original task description", required: false}
---

# Context

You are generating Go source code for a clawdag-go DAG workflow from an approved design.
The output must compile with `go build` and run correctly.

Read the following references before writing any code:
1. `references/library.md` — all 71 op descriptions with exact field names and types
2. `references/dagor-api.md` — operator boilerplate, builder DSL, config.Params API, logging, coalesce/map rules
3. `references/examples/README.md` — pick the most structurally similar example
4. Read that example file in `references/examples/`

# Steps

1. Read all three references as listed above.
2. Implement the approved design exactly — no improvisation, no added ops.
3. Create `<output_dir>/` and write the complete Go source to `<output_dir>/main.go`.
4. Write `<output_dir>/go.mod` with this content (substitute the actual Go version):
   ```
   module solution

   go <version>

   require (
       github.com/akennis/clawdag-go v0.1.0
       github.com/wwz16/dagor v0.0.0
   )

   replace github.com/wwz16/dagor => github.com/akennis/dagor v0.0.0
   ```
5. Run `go mod tidy` in `<output_dir>` — this resolves all remaining dependencies (mcp-go, ants, etc.) and writes `go.sum`.
6. Run `go build ./...` in `<output_dir>` to compile.
7. If the build fails, read the error output, fix `main.go`, and re-run step 6.
8. Repeat until the build exits 0.

# Implementation rules

## Operator boilerplate contract
Every custom op must implement `Setup`, `Reset`, `Run`, `InputFields`, `OutputFields`,
`SetInputField`, and `ResetFields`. The first three methods are defined on the operator;
the last four are the IOperator interface. Library ops with `dag:"input"` / `dag:"output"` tags
have these generated — do NOT write them manually for library ops.

## Trailing commas
Go requires a trailing comma after the LAST element of every multi-line composite literal.
Missing it is a compile error.
```
// WRONG:              RIGHT:
map[string]any{        map[string]any{
  "a": 1,               "a": 1,
  "b": 2                "b": 2,   // ← required
}                      }
```

## Wire naming
Wire names are arbitrary strings you assign in `Output("FieldName", "wire_name")` then reference
in `Input("FieldName", "wire_name")`. They are NOT "vertex.Field" syntax.

## ConditionInput rule
When a predicate needs a wire that the op itself does not consume, use
`.ConditionInput("wire_name")` on the vertex. Do NOT add a dummy field to the op struct.

## PassthroughWire rule
Use `.PassthroughWire("OutputField", "source_wire")` to inherit an upstream wire's value when
the vertex is skipped, so a downstream CoalesceOp sees a non-nil slot.

## Predicate wire name rule
Predicates receive WIRE NAMES as keys, never op field names or output field names.
```
// WRONG: inputs["City"]           // "City" is an op field name
// WRONG: inputs["Result"]         // "Result" is an output field
// RIGHT: inputs["lookup_result"]  // wire name from Input("City", "lookup_result")
```

## CoalesceOp vs SelectStringOp
- **CoalesceOp** (+ `Merge(config.MergeCoalesce)`): use when upstream branches may be SKIPPED.
- **SelectStringOp**: use when BOTH inputs always exist and the choice is a runtime bool wire.
Never use CoalesceOp when neither branch is conditional.

## Env var resolution in main()
ALL `os.Getenv` calls MUST use literal string names in `main()`.
Never call `os.Getenv` inside an operator's `Setup` or `Run`.

## MCP mode
Always support `--mode mcp` via `server.ServeStdio`. The MCP server is long-lived; use
`context.Background()` for the server, and per-call timeouts inside the tool handler.

# Prohibited patterns

## ModeGateOp anti-pattern
Do NOT introduce a "gate" or "passthrough" vertex that fans the input out to lane siblings.
Every lane vertex must gate ITSELF with its own `Condition` + `ConditionInput`.

## VertexSkipped misuse
Do NOT use `eng.VertexSkipped` to select between branch results. Always coalesce and read
from `eng.GetOutput("final_result")`.

## g.Vertices iteration
```
// WRONG: for _, v := range g.Vertices { ... }  // g.Vertices is a func — compile error
// RIGHT: eng.GetOutput("wire_name") for every value you need
```

## MERGE constant
```
// WRONG: .Merge(1)                    // untyped int — compile error
// RIGHT: .Merge(config.MergeCoalesce) // import "github.com/wwz16/dagor/config"
```
