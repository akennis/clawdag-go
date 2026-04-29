# Observability Backlog

## Architecture

The observability framework lives in `library/observability/` and is designed to be consumed by **workflow programs** — programs built using the fluent DAG builder and library ops that are deployed and invoked repeatedly in production. The code generator (`clawdag-go` itself) is a development tool and is not the observability target.

The **unit of observation is the invocation** — one execution of the workflow DAG. For a CLI ticket triager, one invocation equals one ticket. For a workflow program wrapped in an HTTP server, one invocation equals one request. This maps to a trace root span, one audit record, and one token budget.

Three design principles:

1. **Zero-config by default.** Library ops work identically whether or not observability is initialized. No panics, no overhead, no errors when the context carries no observer.
2. **One-line opt-in.** A workflow program enables full observability by calling `observability.Init(ctx, cfg)` in `main()` and deferring `obs.Shutdown(ctx)`.
3. **Deployment-model agnostic.** The same library ops and the same observability package work whether the workflow program is a CLI (one invocation, then exit), a long-lived HTTP server, or an MCP server. The library provides building blocks; the workflow program decides how to expose them.

---

## Epic 1 — Observability Package and Invocation Context

**Goal:** Define the core interfaces and context injection pattern that all subsequent epics build on. No external dependencies. No user-visible behavior on its own — this is the foundation.

---

### Story 1.1 — Core interfaces

Define the four interfaces that library ops and workflow programs program against. All implementations are in `library/observability/`.

```go
type OpEvent struct {
    InvocationID   string
    NodeID         string
    OpName         string
    OpType         string // "ai" or "deterministic"
    DurationMS     int64
    Status         string // "ok" or "error"
    Error          string
    // AI-specific; zero value when OpType == "deterministic"
    AIModel        string
    AIInputTokens  int
    AIOutputTokens int
    AIAttempts     int
    AIReasoning    string
}

type AuditLogger interface {
    LogInvocationStart(ctx context.Context, e InvocationStartEvent) error
    LogOpResult(ctx context.Context, e OpEvent) error
    LogInvocationEnd(ctx context.Context, e InvocationEndEvent) error
    Close() error
}

type MetricsRecorder interface {
    RecordInvocation(status string, durationMS int64)
    RecordOpExecution(opName, opType, status string, durationMS int64)
    RecordAICall(opName, model, status string, inputTokens, outputTokens, retries int, latencyMS int64)
}

type AlertDispatcher interface {
    Dispatch(ctx context.Context, e AlertEvent) error
}
```

Provide no-op implementations: `NoopAuditLogger`, `NoopMetricsRecorder`, `NoopAlertDispatcher`.

**Acceptance criteria (TDD):**
- `TestInterfaces_NoopImplementations`: assert each no-op type satisfies its interface — this test exists solely to catch interface drift when fields are added.
- `TestNoopAuditLogger_AllMethodsReturnNil`: call each method; assert nil error and no panic.

---

### Story 1.2 — Invocation context injection

**As a** workflow program author, **I want** to inject an observability context once at the start of an invocation so that all library ops within that invocation automatically emit events without per-op wiring.

```go
// library/observability/context.go

type InvocationConfig struct {
    ID           string            // UUID; auto-generated if empty
    AuditLogger  AuditLogger
    Metrics      MetricsRecorder
    Alerts       AlertDispatcher
    Tokens       *TokenTracker
    Tracer       trace.Tracer      // from OTel
}

func WithInvocation(ctx context.Context, cfg InvocationConfig) context.Context
func InvocationFromContext(ctx context.Context) (*InvocationConfig, bool)
```

Library ops call `InvocationFromContext(ctx)`. If not present, they proceed with no observability.

**Acceptance criteria (TDD):**
- `TestWithInvocation_RoundTrip`: inject a config; extract with `InvocationFromContext`; assert same pointer returned, `ok == true`.
- `TestInvocationFromContext_Absent`: call on `context.Background()`; assert `ok == false`, no panic.
- `TestWithInvocation_IDGenerated`: pass `ID: ""`; assert stored `InvocationConfig.ID` is a non-empty UUID-format string.
- `TestWithInvocation_DoesNotMutateParent`: inject into a child context; assert `InvocationFromContext` on the parent returns `ok == false`.

---

### Story 1.3 — `Init` and `Shutdown` for workflow programs

**As a** workflow program author, **I want** to enable full observability with one call so that I do not need to wire each backend individually.

```go
// library/observability/init.go

type Config struct {
    ServiceName     string // default: "clawdag-workflow"
    OTLPEndpoint    string // overrides OTEL_EXPORTER_OTLP_ENDPOINT env var
    AuditLogPath    string // overrides AUDIT_LOG_PATH env var; "" disables
    AuditWebhookURL string // overrides AUDIT_WEBHOOK_URL env var; "" disables
    AlertWebhookURL string // overrides ALERT_WEBHOOK_URL env var; "" disables
    TokenBudget     TokenBudgetConfig
}

type Observer struct{ ... }

func Init(ctx context.Context, cfg Config) (*Observer, error)
func (o *Observer) InjectInvocation(ctx context.Context) context.Context
func (o *Observer) Shutdown(ctx context.Context) error
func (o *Observer) PrometheusHandler() http.Handler // Epic 5 Story 5.2
```

Typical workflow program `main()`:
```go
obs, err := observability.Init(context.Background(), observability.Config{})
if err != nil { log.Fatal(err) }
defer obs.Shutdown(context.Background())

ctx := obs.InjectInvocation(context.Background())
if err := eng.Run(ctx); err != nil { ... }
```

**Acceptance criteria (TDD):**
- `TestInit_AllEnvVarsUnset`: call `Init` with empty `Config`; assert no error, non-nil `Observer`.
- `TestInit_ShutdownIdempotent`: call `Shutdown` twice; assert no error, no panic on the second call.
- `TestInjectInvocation_ContainsConfig`: after `Init`, call `InjectInvocation`; assert `InvocationFromContext` returns a non-nil config.
- `TestInjectInvocation_UniqueIDs`: call `InjectInvocation` twice on the same `Observer`; assert the two injected invocation IDs differ.

---

## Epic 2 — Structured Logging in Library Ops

**Goal:** Replace `log.Printf("[DEBUG]...")` throughout the library ops with machine-parseable slog JSON. Workflow programs that run in production get this for free — their log aggregator (Splunk, ELK, Datadog, Cloudwatch Logs) ingests NDJSON from stderr with no adapter.

**Dependencies:** Epic 1 (invocation context provides `invocation_id`).

---

### Story 2.1 — slog JSON handler

`observability.Init()` calls `slog.SetDefault` with a JSON handler writing to stderr. Log level is controlled by `LOG_LEVEL` env var (`DEBUG`, `INFO`, `WARN`, `ERROR`; default `INFO`).

**Acceptance criteria (TDD):**
- `TestLogHandler_JSONOutput`: after `Init`, emit a slog message; capture stderr; assert the line is valid JSON with fields `time`, `level`, `msg`.
- `TestLogHandler_LevelFilter_WarnOnly`: set `LOG_LEVEL=WARN`; emit an INFO and a WARN message; assert only the WARN line appears.
- `TestLogHandler_DefaultsToInfo`: unset `LOG_LEVEL`; assert INFO messages appear and DEBUG messages do not.
- `TestLogHandler_InvalidLevel_FallsBackToInfo`: set `LOG_LEVEL=GARBAGE`; assert process starts without error and INFO messages appear.

---

### Story 2.2 — Invocation ID in every log line

Every log line emitted during an invocation must carry `invocation_id` so that log queries can reconstruct a complete invocation trace without a separate tracing system.

Implement an `slog.Handler` wrapper that reads `invocation_id` from the context and appends it to every record.

**Acceptance criteria (TDD):**
- `TestInvocationIDInLogs`: inject invocation context with `ID="inv-abc"`; emit a slog message using that context; assert the line contains `"invocation_id":"inv-abc"`.
- `TestInvocationIDAbsent_FieldOmitted`: emit a slog message without invocation context; assert the `invocation_id` key is absent from the JSON (not `"invocation_id":""`).
- `TestInvocationIDUnique_AcrossInvocations`: two sequential invocations each emit a log line; assert the `invocation_id` values differ.

---

### Story 2.3 — Structured op lifecycle events from library ops

Add shared helpers to `library/observability/`:
```go
func LogOpStart(ctx context.Context, opName, nodeID string)
func LogOpEnd(ctx context.Context, opName, nodeID string, durationMS int64, err error)
```

Call these at the start and end of every library op's `Run` method. Remove all existing `log.Printf("[DEBUG] <OpName>:...")` calls.

Affected files: `library/ai_compute_op.go`, `library/ai_ops.go`, and all deterministic op files in `library/`.

Fields on `op_start`: `event="op_start"`, `op`, `node_id`, `invocation_id`.
Fields on `op_end`: `event="op_end"`, `op`, `node_id`, `invocation_id`, `duration_ms`, `status` (`ok|error`), `error` (omitempty).

**Acceptance criteria (TDD):**
- `TestOpLifecycle_StartEvent`: run any library op with an initialized context; capture stderr; assert one JSON line with `"event":"op_start"` and non-empty `op` and `invocation_id`.
- `TestOpLifecycle_EndEvent_OK`: run a succeeding op; assert `"event":"op_end"`, `"status":"ok"`, `"duration_ms" >= 0`.
- `TestOpLifecycle_EndEvent_Error`: run a failing op; assert `"event":"op_end"`, `"status":"error"`, non-empty `"error"`.
- `TestOpLifecycle_NoObservability_NoPanic`: run any library op with `context.Background()` (no invocation injected); assert no panic.

---

### Story 2.4 — Structured AI call events from library ops

After each Anthropic API call inside a library op, emit one `ai_call` log event.

Fields: `event="ai_call"`, `op`, `model`, `attempt` (1-based), `input_tokens`, `output_tokens`, `latency_ms`, `status` (`ok|error`), `invocation_id`. At `LOG_LEVEL=DEBUG`, also include `prompt_chars` (int) and `response_chars` (int) — never the raw text.

Affected call sites: `AIComputeOp.Run()` in `library/ai_compute_op.go` (covers all generic variants), plus `AIClassifyMultiLabelOp`, `AIScoreOp`, `AIBoolOp`, `AIBestMatchOp`, `AIRerankOp` in `library/ai_ops.go`.

**Acceptance criteria (TDD):**
- `TestAICallEvent_AIComputeOp`: mock Anthropic client returning `Usage{InputTokens:100, OutputTokens:50}`; run any `AIComputeOp` variant; assert one line with `"event":"ai_call"`, `"input_tokens":100`, `"output_tokens":50`.
- `TestAICallEvent_BespokeOps`: same assertion for `AIScoreOp`, `AIBoolOp`, and `AIClassifyMultiLabelOp`.
- `TestAICallEvent_RetryAttempts`: configure mock to fail twice then succeed; assert three `ai_call` lines with `"attempt"` values 1, 2, 3 in order.
- `TestAICallEvent_NoRawTextAtInfo`: at `LOG_LEVEL=INFO`, assert no `ai_call` line contains a `prompt` field.

---

## Epic 3 — Immutable Audit Trail

**Goal:** Write an append-only NDJSON audit log that records every invocation with full op-level detail: which nodes ran, AI vs deterministic, token counts, result, and skipped branches. Enterprise SIEMs (Splunk, IBM QRadar, Microsoft Sentinel) ingest NDJSON directly. This is the compliance artifact that security and audit teams require.

**Dependencies:** Epic 1 (interfaces and context injection).

---

### Story 3.1 — NDJSON audit logger implementation

Implement `NDJSONAuditLogger` satisfying `AuditLogger`:
- Opens `AUDIT_LOG_PATH` with `os.O_APPEND|os.O_CREATE|os.O_WRONLY` and file mode `0600`.
- Writes one JSON line per event followed by `\n`.
- Calls `file.Sync()` after every write.
- `NoopAuditLogger` is used when `AUDIT_LOG_PATH` is empty.

**Acceptance criteria (TDD):**
- `TestNDJSONLogger_TwoEvents_TwoLines`: write two events; assert file has exactly two `\n`-terminated lines, each valid JSON.
- `TestNDJSONLogger_FileMode0600`: assert file is created with permission mode `0600`.
- `TestNDJSONLogger_AppendNotTruncate`: write one event, close, reopen, write another; assert file has two lines total.
- `TestNDJSONLogger_SyncPerWrite`: use a mock `io.Writer` that records `Sync` calls; assert `Sync` called exactly once per event.
- `TestNDJSONLogger_Close_Idempotent`: call `Close` twice; assert no panic and no error on the second call.

---

### Story 3.2 — InvocationStartEvent

Emitted by `obs.InjectInvocation()` immediately before the DAG engine runs.

```go
type InvocationStartEvent struct {
    Event        string `json:"event"`                   // "invocation_start"
    InvocationID string `json:"invocation_id"`
    Timestamp    string `json:"timestamp"`               // RFC3339Nano
    ServiceName  string `json:"service_name"`
    Version      string `json:"version"`                 // from ldflags; "dev" if unset
    OperatorID   string `json:"operator_id,omitempty"`   // from OPERATOR_ID env var
}
```

**Acceptance criteria (TDD):**
- `TestInvocationStart_Schema`: emit; parse the NDJSON line; assert `event="invocation_start"`, `invocation_id` non-empty, `timestamp` parses as RFC3339.
- `TestInvocationStart_OperatorIDPresent`: set `OPERATOR_ID=alice`; assert `"operator_id":"alice"` in output.
- `TestInvocationStart_OperatorIDOmitted`: unset `OPERATOR_ID`; assert `operator_id` key is absent from JSON.
- `TestInvocationStart_VersionDev`: build without ldflags; assert `"version":"dev"`.

---

### Story 3.3 — OpResultEvent from library ops

Library ops call `invCfg.AuditLogger.LogOpResult(ctx, OpEvent{...})` at the end of each `Run()`. The `OpEvent` struct from Story 1.1 is the carrier.

`AIReasoning` is populated for library AI ops that expose a `Reasoning` output field. It is always omitted for deterministic ops. `NodeID` is the dagor vertex name (e.g. `"classify"`, `"bug_severity"` in the ticket triager).

**Acceptance criteria (TDD):**
- `TestOpResult_DeterministicOp`: run a deterministic op (e.g. `StringLookupOp`); assert emitted `OpEvent` has `OpType="deterministic"`, `AIModel` empty.
- `TestOpResult_AIOpFields`: run a mocked `AIScoreOp`; assert `OpType="ai"`, `AIModel` non-empty, `AIInputTokens > 0`.
- `TestOpResult_ErrorStatus`: run a failing op; assert `Status="error"`, `Error` non-empty.
- `TestOpResult_ReasoningPresent`: run `AIBoolOp` (which populates `Reasoning`); assert `AIReasoning` non-empty.
- `TestOpResult_NoAuditLogger_NoPanic`: run any library op with no `AuditLogger` in context; assert no panic.

---

### Story 3.4 — InvocationEndEvent

Emitted after `eng.Run(ctx)` returns. Totals are accumulated from `OpEvent` values recorded during the invocation.

```go
type InvocationEndEvent struct {
    Event                string `json:"event"`                  // "invocation_end"
    InvocationID         string `json:"invocation_id"`
    Timestamp            string `json:"timestamp"`
    Status               string `json:"status"`                 // "success" or "failure"
    Error                string `json:"error,omitempty"`
    TotalDurationMS      int64  `json:"total_duration_ms"`
    AIOpCount            int    `json:"ai_op_count"`
    DeterministicOpCount int    `json:"deterministic_op_count"`
    SkippedOpCount       int    `json:"skipped_op_count"`       // DAG branch pruning
    TotalInputTokens     int    `json:"total_input_tokens"`
    TotalOutputTokens    int    `json:"total_output_tokens"`
}
```

`SkippedOpCount` matters for conditional DAGs like the ticket triager, where three of four lanes are pruned per invocation.

**Acceptance criteria (TDD):**
- `TestInvocationEnd_Totals`: simulate two AI ops (50+30 input, 20+10 output) and one deterministic; assert `TotalInputTokens=80`, `AIOpCount=2`, `DeterministicOpCount=1`.
- `TestInvocationEnd_SkippedCount`: simulate a 4-branch DAG where 3 branches are pruned (as in the ticket triager); assert `SkippedOpCount >= 3`.
- `TestInvocationEnd_Success`: successful invocation; assert `"status":"success"`, no `error` key in JSON.
- `TestInvocationEnd_Failure`: failed invocation; assert `"status":"failure"`, `error` field non-empty.

---

### Story 3.5 — Webhook delivery of audit events

When `AUDIT_WEBHOOK_URL` is set, POST each audit event as JSON to that URL with `Content-Type: application/json` and a 5-second `http.Client` timeout. A failed POST is logged at WARN level and does not fail the invocation.

When `AUDIT_WEBHOOK_SECRET` is set, include `X-Clawdag-Signature: sha256=<HMAC-SHA256(secret, body)>` so receivers can verify authenticity.

**Acceptance criteria (TDD):**
- `TestAuditWebhook_EventReceived`: start `httptest.Server`; emit `InvocationStartEvent`; assert server received POST with a body that deserializes to a valid event.
- `TestAuditWebhook_ContentType`: assert `Content-Type: application/json` on every POST.
- `TestAuditWebhook_Signature`: set `AUDIT_WEBHOOK_SECRET=s3cr3t`; assert `X-Clawdag-Signature` header present and HMAC validates against body.
- `TestAuditWebhook_FailureDoesNotFailInvocation`: point URL at a non-listening port; run an invocation; assert it completes normally.
- `TestAuditWebhook_Timeout`: server stalls for 10 seconds; assert client abandons after 5 seconds and does not block the invocation.

---

## Epic 4 — OpenTelemetry Distributed Tracing

**Goal:** Make every invocation a root OTel trace and every library op execution a child span. Enterprises configure the export destination via the standard `OTEL_EXPORTER_OTLP_ENDPOINT` env var. Datadog, Honeycomb, Jaeger, Grafana Tempo, New Relic, and Dynatrace all accept OTLP natively — no adapter required. `obs.Shutdown()` calls `tp.ForceFlush` before `tp.Shutdown`, which is critical for CLI workflow programs that must flush spans before process exit.

**Dependencies:** Epic 1.

---

### Story 4.1 — OTel TracerProvider initialization

`observability.Init()` initializes an OTel `TracerProvider`. When `OTEL_EXPORTER_OTLP_ENDPOINT` is set, builds a gRPC OTLP exporter. Otherwise installs a no-op provider. `obs.Shutdown()` calls `tp.ForceFlush(ctx)` then `tp.Shutdown(ctx)`.

**Acceptance criteria (TDD):**
- `TestTracerProvider_NoEndpoint_Noop`: unset `OTEL_EXPORTER_OTLP_ENDPOINT`; assert no spans are exported and no error occurs after `Shutdown`.
- `TestTracerProvider_ShutdownFlushes`: use OTel in-memory exporter; start and end a span; call `Shutdown`; assert the span appears in the exporter's finished span list.

---

### Story 4.2 — Invocation root span

`obs.InjectInvocation(ctx)` starts a root span named `workflow.invocation` and stores it in the returned context. The workflow program ends it after `eng.Run(ctx)` returns.

Span attributes: `workflow.invocation_id`, `workflow.service_name`, `workflow.version`. On failure, set span status to `codes.Error` and call `span.RecordError(err)`.

**Acceptance criteria (TDD):**
- `TestRootSpan_Created`: use OTel in-memory exporter; call `InjectInvocation` and end the span; assert one finished span named `"workflow.invocation"`.
- `TestRootSpan_Attributes`: assert span has `workflow.invocation_id` (non-empty) and `workflow.service_name`.
- `TestRootSpan_ErrorRecorded`: call `span.RecordError(err)`; assert span status is `Error`.

---

### Story 4.3 — Per-op child spans from library ops

Each library op starts a child span at the top of `Run()` using the tracer from `InvocationFromContext`, and ends it via `defer span.End()`. Span name: `op.<OpName>` (e.g. `op.AIScoreOp`, `op.StringLookupOp`).

Span attributes: `op.name`, `op.node_id`, `op.type` (`ai|deterministic`). On error, set span status to `Error`.

**Acceptance criteria (TDD):**
- `TestOpSpan_ChildOfRoot`: assert all op spans share `trace_id` with the root span and have `parent_span_id` equal to the root span's `span_id`.
- `TestOpSpan_TypeAttribute`: assert `StringLookupOp` span has `op.type="deterministic"`; assert `AIScoreOp` span has `op.type="ai"`.
- `TestOpSpan_ErrorStatus`: failing op; assert span status is `Error` and `RecordError` was called.
- `TestOpSpan_NoTracer_NoPanic`: run any library op with `context.Background()` (no tracer injected); assert no panic.

---

### Story 4.4 — AI call sub-spans

Within each AI op, wrap each Anthropic API call in a sub-span named `ai.call`.

Span attributes: `ai.model`, `ai.attempt` (1-based int), `ai.input_tokens`, `ai.output_tokens`, `ai.latency_ms`. On API error, call `span.RecordError(err)`.

**Acceptance criteria (TDD):**
- `TestAICallSpan_Attributes`: mock Anthropic; run `AIScoreOp`; assert one `"ai.call"` span with `ai.input_tokens > 0` and `ai.model` non-empty.
- `TestAICallSpan_ParentIsOpSpan`: assert the `ai.call` span's `parent_span_id` equals the enclosing `op.AIScoreOp` span's `span_id`.
- `TestAICallSpan_RetrySpans`: fail twice then succeed; assert three `ai.call` spans with `ai.attempt` values 1, 2, 3.

---

## Epic 5 — Metrics

**Goal:** Record per-op execution counts, latency, and AI token consumption. OTel metrics via OTLP is the primary backend — it works for both CLI programs (flush at exit) and long-lived server programs (continuous export). An optional Prometheus handler is available as a secondary integration for workflow programs that are HTTP servers.

**Dependencies:** Epic 1 (`MetricsRecorder` interface and invocation context).

---

### Story 5.1 — OTel metric instruments

`observability.Init()` registers these instruments when `OTEL_EXPORTER_OTLP_ENDPOINT` is set. `obs.Shutdown()` flushes the metric provider — critical for CLI workflow programs.

- `clawdag.invocations` — Int64Counter, attributes: `status` (`success|failure`), `service.name`.
- `clawdag.invocation.duration` — Float64Histogram (ms), attributes: `status`.
- `clawdag.op.executions` — Int64Counter, attributes: `op.name`, `op.type`, `status`.
- `clawdag.op.duration` — Float64Histogram (ms), attributes: `op.name`, `op.type`.
- `clawdag.ai.calls` — Int64Counter, attributes: `op.name`, `ai.model`, `status`.
- `clawdag.ai.input_tokens` — Int64Counter, attributes: `op.name`, `ai.model`.
- `clawdag.ai.output_tokens` — Int64Counter, attributes: `op.name`, `ai.model`.
- `clawdag.ai.retries` — Int64Counter, attributes: `op.name`.

**Acceptance criteria (TDD):**
- `TestMetrics_NoEndpoint_Noop`: unset `OTEL_EXPORTER_OTLP_ENDPOINT`; assert recording calls do not panic.
- `TestMetrics_OpExecution_Recorded`: use OTel in-memory reader; run `AIScoreOp`; assert `clawdag.op.executions` has a data point with `op.name="AIScoreOp"`, `op.type="ai"`, `status="ok"`.
- `TestMetrics_AITokens_Recorded`: run mocked `AIScoreOp` returning 100 input tokens; assert `clawdag.ai.input_tokens` data point value equals 100.
- `TestMetrics_ShutdownFlushes`: record a metric; call `Shutdown`; assert the data point is accessible in the reader after shutdown.

---

### Story 5.2 — Optional Prometheus handler for HTTP server workflow programs

`obs.PrometheusHandler()` returns an `http.Handler` that workflow programs can mount at any path (e.g. `/metrics`) if they are HTTP servers. Not started automatically by `Init`. Backed by a bridge from the OTel metric instruments so there is one source of truth.

**Acceptance criteria (TDD):**
- `TestPrometheusHandler_NotNil`: call `obs.PrometheusHandler()`; assert non-nil handler.
- `TestPrometheusHandler_Returns200`: mount handler; GET; assert HTTP 200.
- `TestPrometheusHandler_MetricPresent`: run `AIScoreOp`; scrape; assert response body contains `clawdag_op_executions_total{op_name="AIScoreOp"`.
- `TestPrometheusHandler_LabelsCorrect`: assert `op_type="ai"` and `status="ok"` labels appear on the `AIScoreOp` counter.

---

## Epic 6 — Token Attribution and Budget Enforcement

**Goal:** Track Anthropic API token consumption per invocation, per op, and per model — for chargeback reporting, budget enforcement, and capacity planning.

**Dependencies:** Epic 1 (`TokenTracker` is a field on `InvocationConfig`).

---

### Story 6.1 — TokenTracker: per-invocation accumulator

```go
// library/observability/tokens.go

type TokenRecord struct {
    Op           string
    Model        string
    InputTokens  int
    OutputTokens int
}

type TokenSummary struct {
    TotalInputTokens  int
    TotalOutputTokens int
    ByOp              map[string]TokenRecord
    ByModel           map[string]TokenRecord
}

type TokenTracker struct{ ... }

func (t *TokenTracker) Record(op, model string, inputTokens, outputTokens int)
func (t *TokenTracker) Summary() TokenSummary
func (t *TokenTracker) Reset()
```

Library ops retrieve the `*TokenTracker` from `InvocationConfig` and call `Record` after each Anthropic API response. If the context contains no tracker, skip silently.

**Acceptance criteria (TDD):**
- `TestTokenTracker_RecordAndSum`: three `Record` calls with known values; assert `Summary().TotalInputTokens` equals their sum.
- `TestTokenTracker_ByOp`: two calls for `"AIScoreOp"`, one for `"AIBoolOp"`; assert `ByOp["AIScoreOp"].InputTokens` equals the sum of those two.
- `TestTokenTracker_Reset`: record values; call `Reset`; assert `Summary().TotalInputTokens == 0`.
- `TestTokenTracker_ConcurrentSafe`: 10 goroutines call `Record` simultaneously; run with `-race`; assert no data race.

---

### Story 6.2 — Instrument all library AI op call sites

After every `client.Messages.New` (or streaming accumulation), call `tracker.Record(opName, string(params.Model), int(msg.Usage.InputTokens), int(msg.Usage.OutputTokens))`.

Affected: `AIComputeOp.Run()` in `library/ai_compute_op.go` (covers `AIExtractStringSliceOp`, `AIExtractMapOp`, `AIParseNumberOp`, `AISummarizeOp`); plus `AIClassifyMultiLabelOp`, `AIScoreOp`, `AIBoolOp`, `AIBestMatchOp`, `AIRerankOp` in `library/ai_ops.go`.

**Acceptance criteria (TDD):**
- `TestTokenInstrumentation_AIComputeOp`: inject tracker; mock Anthropic returning `Usage{InputTokens:100, OutputTokens:50}`; run any `AIComputeOp` variant; assert tracker shows `input=100, output=50` attributed to that op.
- `TestTokenInstrumentation_BespokeOps`: same assertion for `AIScoreOp`, `AIBoolOp`, `AIClassifyMultiLabelOp`.
- `TestTokenInstrumentation_NoTracker_NoPanic`: run `AIScoreOp` with a context that has no tracker; assert no panic.
- `TestTokenInstrumentation_AllAIOpsInstrumented`: maintain a package-level list of all AI op type names; assert every name has a corresponding test entry — prevents new ops from being added without instrumentation.

---

### Story 6.3 — Per-invocation token budget enforcement

Read `MAX_INPUT_TOKENS_PER_INVOCATION` and `MAX_OUTPUT_TOKENS_PER_INVOCATION` env vars (integers; default 0 = unlimited) into `TokenBudgetConfig`. Before each Anthropic API call inside a library op, check the running total. If exceeded, return sentinel error `observability.ErrTokenBudgetExceeded`.

Workflow programs handle this with `errors.Is(err, observability.ErrTokenBudgetExceeded)`.

**Acceptance criteria (TDD):**
- `TestBudgetEnforcement_Exceeded`: set `MAX_INPUT_TOKENS_PER_INVOCATION=1`; run any AI op; assert returned error wraps `ErrTokenBudgetExceeded`.
- `TestBudgetEnforcement_NotExceeded`: set budget to 100000; run AI op with small token count; assert no budget error.
- `TestBudgetEnforcement_Unlimited`: leave env var unset; run 5 AI calls; assert no budget error regardless of token count.
- `TestBudgetEnforcement_SentinelIsDistinct`: assert `errors.Is(ErrTokenBudgetExceeded, context.DeadlineExceeded)` is false.

---

### Story 6.4 — Token totals in InvocationEndEvent

Populate `TotalInputTokens` and `TotalOutputTokens` in `InvocationEndEvent` (Story 3.4) from `tracker.Summary()`.

**Acceptance criteria (TDD):**
- `TestInvocationEnd_TokenTotals`: run two AI ops with known token usage; assert `InvocationEndEvent.TotalInputTokens` equals the sum from the tracker.
- `TestInvocationEnd_TokensZeroWhenNoAI`: invocation with only deterministic ops; assert both token fields are zero.

---

## Epic 7 — Deployment Utilities for Workflow Programs

**Goal:** Provide composable utilities that workflow programs call from their own `main()` or server setup to support production deployment. The library provides primitives; the workflow program decides how to expose them (CLI flag, HTTP endpoint, startup log, or all three).

**Dependencies:** Epic 1.

---

### Story 7.1 — Dependency preflight checks

```go
// library/observability/checks.go

type CheckResult struct {
    Name    string
    Status  string // "ok" or a short error string
    Message string `json:",omitempty"`
}

func CheckAPIKey() CheckResult           // validates CLAUDE_API_KEY is set and non-empty
func CheckAuditLogWritable() CheckResult // validates AUDIT_LOG_PATH is writable
func RunChecks(checks ...func() CheckResult) []CheckResult
```

A workflow program might expose these as `--check` output, as a `/readyz` handler's body, or as a structured log event at startup. The checks themselves are independent of the exposure mechanism.

**Acceptance criteria (TDD):**
- `TestCheckAPIKey_Set`: set `CLAUDE_API_KEY=sk-test`; assert `Status == "ok"`.
- `TestCheckAPIKey_Missing`: unset `CLAUDE_API_KEY`; assert `Status == "missing"`.
- `TestCheckAuditLogWritable_OK`: set `AUDIT_LOG_PATH` to a writable temp path; assert `Status == "ok"`.
- `TestCheckAuditLogWritable_Unwritable`: set path to a read-only directory; assert `Status == "not_writable"`.
- `TestRunChecks_PartialFailure`: mix one passing and one failing check; assert both results are returned and the failing check's status is not `"ok"`.

---

### Story 7.2 — Build info embedding

```go
// library/observability/buildinfo.go

type BuildInfo struct {
    Version   string // from -ldflags "-X observability.Version=..."; "dev" if unset
    GoVersion string // runtime.Version()
    BuildTime string // RFC3339 from ldflags; "unknown" if unset
}

func GetBuildInfo() BuildInfo
```

Workflow programs use this in `--version` output, startup log lines, or version endpoints — the presentation is the workflow program's choice.

**Acceptance criteria (TDD):**
- `TestBuildInfo_DevDefaults`: build without ldflags; assert `Version == "dev"`, `BuildTime == "unknown"`.
- `TestBuildInfo_LdflagsSet`: build with `-ldflags "-X github.com/akennis/clawdag-go/library/observability.Version=abc123"`; assert `GetBuildInfo().Version == "abc123"`.
- `TestBuildInfo_GoVersionPresent`: assert `GoVersion` starts with `"go"`.

---

### Story 7.3 — Structured startup and shutdown log events

`obs.Init()` emits an `"event":"observability_init"` slog line with fields: `audit_log_enabled` (bool), `otlp_enabled` (bool), `alert_webhook_enabled` (bool), `service_name`. This lets operators verify that observability configured correctly without reading source code.

`obs.Shutdown()` emits `"event":"observability_shutdown"` with `duration_ms`.

**Acceptance criteria (TDD):**
- `TestInit_EmitsStructuredEvent`: call `Init`; capture stderr; assert a JSON line with `"event":"observability_init"` and `"audit_log_enabled"` key present.
- `TestShutdown_EmitsStructuredEvent`: call `Shutdown`; assert a JSON line with `"event":"observability_shutdown"` and `"duration_ms" >= 0`.

---

## Epic 8 — Alerting Webhooks

**Goal:** Notify external alerting systems (PagerDuty, Opsgenie, Slack, ITSM platforms) when actionable events occur. All integration is via HTTP POST — no vendor SDK required. Workflow programs configure the target URL via `ALERT_WEBHOOK_URL`.

**Dependencies:** Epic 1 (`AlertDispatcher` interface), Epic 6 (token budget state).

---

### Story 8.1 — WebhookAlertDispatcher

Implement `WebhookAlertDispatcher` satisfying `AlertDispatcher`:

```go
type AlertEvent struct {
    Type         string         `json:"type"`
    InvocationID string         `json:"invocation_id"`
    Timestamp    string         `json:"timestamp"`   // RFC3339
    Severity     string         `json:"severity"`    // "info" | "warning" | "critical"
    Message      string         `json:"message"`
    Details      map[string]any `json:"details,omitempty"`
}
```

POSTs JSON to `ALERT_WEBHOOK_URL` with `Content-Type: application/json` and a 5-second `http.Client` timeout. Non-2xx responses return an error to the caller. `NoopAlertDispatcher` is used when the URL is empty.

When `ALERT_WEBHOOK_SECRET` is set, include `X-Clawdag-Signature: sha256=<HMAC-SHA256(secret, body)>`.

**Acceptance criteria (TDD):**
- `TestWebhookDispatcher_PostsJSON`: dispatch one event; assert `httptest.Server` received POST with a body that deserializes to a valid `AlertEvent`.
- `TestWebhookDispatcher_ContentType`: assert `Content-Type: application/json` on every POST.
- `TestWebhookDispatcher_Signature`: set secret; assert `X-Clawdag-Signature` header present and HMAC validates against body.
- `TestWebhookDispatcher_Non2xx_ReturnsError`: server returns 500; assert `Dispatch` returns a non-nil error.
- `TestWebhookDispatcher_Timeout_5s`: server stalls; assert client abandons after 5 seconds.

---

### Story 8.2 — Invocation failure alert

After `eng.Run(ctx)` returns a non-nil error, the workflow program calls `alertDispatcher.Dispatch` with `type="invocation_failed"`, `severity="critical"`.

`details`: `error` (string), `service_name`, `ai_op_count` (from token tracker), `total_input_tokens`.

A dispatch failure is logged at WARN level and does not alter the invocation's error.

**Acceptance criteria (TDD):**
- `TestFailureAlert_Dispatched`: inject a failing op; assert a `critical` alert with `type="invocation_failed"` is dispatched.
- `TestFailureAlert_NotDispatchedOnSuccess`: successful invocation; assert no `invocation_failed` alert.
- `TestFailureAlert_DispatchError_InvocationErrorUnchanged`: break webhook URL; force invocation failure; assert the returned error is the original op error, not a webhook error.

---

### Story 8.3 — Token budget alert

When token consumption exceeds 80% of `MAX_INPUT_TOKENS_PER_INVOCATION` or `MAX_OUTPUT_TOKENS_PER_INVOCATION`, dispatch `type="token_budget_80pct"`, `severity="warning"`. Fire at most once per invocation per direction.

`details`: `direction` (`"input"|"output"`), `consumed` (int), `budget` (int), `pct` (float64, e.g. `83.5`).

**Acceptance criteria (TDD):**
- `TestTokenBudgetAlert_Dispatched`: set budget to 100; simulate 85 tokens consumed; assert `warning` alert dispatched.
- `TestTokenBudgetAlert_OncePerInvocationPerDirection`: cross 80% twice in one invocation; assert exactly one alert dispatched per direction.
- `TestTokenBudgetAlert_NoBudget_NeverDispatched`: leave budget env var unset; assert no `token_budget_80pct` alert regardless of token count.

---

### Story 8.4 — Alert delivery failure isolation

A failed alert delivery must never propagate to the invocation result. Failures are logged at WARN level only.

**Acceptance criteria (TDD):**
- `TestAlertFailure_InvocationUnaffected`: break `ALERT_WEBHOOK_URL`; run a failing invocation; assert the returned error is the original op error, not a webhook error.
- `TestAlertFailure_WarnLogged`: break webhook; dispatch an alert; capture stderr; assert a WARN-level slog line is emitted mentioning the dispatch failure.
