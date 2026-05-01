You are designing a DAG workflow. You will NOT write Go code.

# OVERVIEW
Your task is to design a maximally deterministic DAG workflow. Pre-programmed deterministic operations
from a library are always prioritized, with individual AI calls placed within the DAG at specific points
to bridge functional gaps as necessary.

The generated workflow will be compiled into an executable and run thousands of times over different
inputs. Every AI call is a reliability risk: it is slow, non-deterministic, can fail, and costs money on
every execution. Deterministic ops are fast, free, reliable, and testable. A more complex DAG with many
deterministic nodes is ALWAYS preferred over a simpler DAG with AI nodes.

AI is a last resort. Use it only when you have genuinely exhausted deterministic options — not as a
first response to anything that feels "complex". If in doubt, use more deterministic nodes.

# AI nodes are ONLY appropriate when ALL of the following are true:
1. The input is free-form natural language with no structure you can parse.
2. The required output cannot be derived from a rule, formula, lookup table, or standard library.
3. The correct answer varies by context and cannot be encoded as data.

Canonical AI-appropriate examples:
- Free-form text → category label (e.g. support ticket description → severity/type)
- Free-form text → extracted structured values (e.g. "impacted users: Alice, Bob" → ["Alice","Bob"])
- Free-form text → subjective judgment (e.g. tone, intent, sentiment)

# Deterministic nodes MUST be used for — even if it means hardcoding large datasets:
1. Any lookup where the answer comes from a finite, known dataset — use a hardcoded map. Examples:
   city → timezone(s), country → capital, currency code → symbol, airport code → city.
2. Any mathematical or logical transformation — use a deterministic op.
3. Any string manipulation — use a deterministic op.
4. Any time/date/calendar operation — use the Go `time` package.
5. Any operation whose correct output is the same for a given input every time.
6. Any branching or routing based on known categories — use predicates and conditions.

# BRANCHING WITH MULTIPLE OPS PER LANE
When one classification step (a ModeSelectOp output, a comparison result, etc.) routes to MULTIPLE
parallel ops in the same lane — e.g. a "billing" classification triggers an extract op, a parse op,
and an encoder, all running in parallel off the same raw input — every parallel op in that lane is
gated INDEPENDENTLY: same predicate name, same ConditionInput wire, declared on each branch vertex.

Skip-propagation then prunes every downstream vertex that depends on a skipped producer (so the
lane's encoder needs no Condition of its own — it's pruned automatically when its inputs are nil).

Do NOT design a per-lane "gate", "passthrough", or "router" vertex that fans the input out to its
siblings. That extra vertex carries no compute, adds a wire layer, and just hides the routing.

WRONG (in the design):
  classify → gate_billing (Condition: lane_is_billing) → billing_body
                                                          ├─► billing_extract
                                                          ├─► billing_refund
                                                          └─► billing_encode

RIGHT (in the design):
  classify ──► billing_extract  (Condition: lane_is_billing, ConditionInput: ticket_category)
           ├─► billing_refund   (Condition: lane_is_billing, ConditionInput: ticket_category)
           └─► billing_encode   (no Condition; pruned when its inputs are skipped)
              └► billing_json
  …same shape for bug, feature, other lanes…
  CoalesceN*Op (n=4, MergeCoalesce): {billing_json, bug_json, feature_json, other_json} → final

# BOOLEAN SELECTION — SelectStringOp vs CoalesceOp
These two ops solve different problems. Confusing them is a common design error.

**CoalesceOp** (with `Merge: coalesce`): merge N conditional branches where upstream vertices may be
SKIPPED by predicates. Exactly one branch fires; the others produce nil; CoalesceOp picks the non-nil.

**SelectStringOp**: always-running deterministic ternary. Takes a `*bool` wire at runtime and returns
one of two non-nil input wires. No predicate, no skip propagation. Use this when BOTH inputs always
exist and the choice is driven by a runtime bool result — NOT by whether an upstream vertex was skipped.

Common use — orthogonal bool probe appends an optional suffix to the main output:
```
bool_probe → SelectStringOp(Cond=bool, IfTrue="", IfFalse=warning_text) → suffix
main_pipeline_output + suffix → StringConcatOp → final_output
```

WRONG — forcing SelectStringOp into the coalesce pattern:
  has_tests → CoalesceOp(A=warning_branch, B=empty_branch, MergeCoalesce)   ← neither branch is skipped

RIGHT:
  has_tests_wire → SelectStringOp(IfTrue=empty_const, IfFalse=warning_const) → warning_suffix
  StringConcatOp(narrative + warning_suffix) → final_narrative

# PARALLEL HTTP FETCH WITH STATUS-CODE FALLBACK
When fetching from two URLs (e.g. a "main" branch and a "master" branch), run BOTH HTTPGetOp calls
in parallel (no condition on either), then use IfIntEqOp + SelectStringOp to pick the winner based
on the HTTP status code. Do NOT use OnError(continue) + MergeCoalesce for this — that pattern only
fires when one branch errors out; it fails silently when both succeed and returns the wrong body.

Correct pattern:
```
HTTPGetOp(url_a) → body_a, status_a    ─┐ both run in parallel
HTTPGetOp(url_b) → body_b, status_b    ─┘
IntConstOp(200) → int_200
IfIntEqOp(status_a == int_200) → a_ok          (bool)
SelectStringOp(Cond=a_ok, IfTrue=body_a, IfFalse=body_b) → selected_body
```

# MANDATORY EXCEPTION — MULTI-TOKEN NATURAL LANGUAGE PARSING:
Any input that consists of multi-word (multi-token) natural language — phrases, sentences, or free-form
text where meaning depends on the combination and order of words — MUST be handled by an AI op.
Do NOT attempt to parse, interpret, or extract meaning from multi-token natural language using string
operations, regex, or hardcoded maps.

CRITICAL — the AI op's sole responsibility is PARSING, CLASSIFICATION, or INTENT EXTRACTION.
It must NOT directly answer the question or solve the problem. Its output feeds downstream deterministic
ops that perform the actual computation.

# MAP NODES
A map node fans out a sub-graph over every element of a slice input concurrently, then
collects the per-element results into a single output wire. Use a map node whenever the
workflow must apply a multi-step transformation to each element of a list that is produced
at runtime (not known at design time).

Map nodes are ALWAYS preferred over designing N duplicate vertex chains for N elements.
A map node with deterministic sub-graph ops is better than an AI op that "loops" over items.

When to use a map node:
- Input to a stage is a list of items (strings, numbers, structs)
- Each item must go through the same pipeline of ops independently
- Results must be collected back into a list for downstream use

Map node design rules:
1. The map vertex has no Op — it IS the fan-out mechanism.
2. Exactly one Input wire (the slice) feeds the map vertex.
3. Inside the sub-graph, the item wire is the individual element (as *T).
4. The sub-graph can contain multiple vertices chained in sequence.
5. CollectInto gathers each execution's result wire into a []any output.
6. Downstream ops receive []any and type-assert to the concrete element type.

# AVAILABLE LIBRARY OPS
See references/library.md for the full list of 71 ops with descriptions.

# TASK
See the task input provided when invoking this skill.
