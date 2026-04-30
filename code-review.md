# Code Review — clawdag-go

Each issue below is self-contained and actionable. Work through them top-to-bottom;
later fixes may depend on earlier ones (e.g. the `COMPILE_FAILED` constant defined in
issue #4 is referenced in subsequent issues).

---

## Critical

---

### Issue 1 — `ValidateDAGOp.Run()` is a permanent no-op

**File:** `driver_ops.go:68–72`

**Problem:**
```go
func (op *ValidateDAGOp) Setup(params *config.Params) error { return nil }
func (op *ValidateDAGOp) Reset() error                      { return nil }
func (op *ValidateDAGOp) Run(ctx context.Context) error {
    return nil
}
```
`ValidationError` is never set. `FallbackOp.Run()` checks:
```go
validationOK := op.ValidationError == nil || *op.ValidationError == ""
```
Because `ValidationError` is always `""` (zero value), `validationOK` is always `true`,
and the `dagValidationErrorContextTemplate` path in `FallbackOp` can never fire.

**Fix:**
Implement real validation in `ValidateDAGOp.Run()`. The op already receives `GoFiles *string`
(the raw Go source). Parse and validate it using `go/parser`:

```go
import (
    "go/parser"
    "go/token"
)

func (op *ValidateDAGOp) Run(ctx context.Context) error {
    fset := token.NewFileSet()
    _, err := parser.ParseFile(fset, "main.go", *op.GoFiles, parser.AllErrors)
    if err != nil {
        op.ValidationError = err.Error()
    }
    return nil
}
```

For deeper DAG-structural validation (e.g., checking that the generated code actually
builds a valid `graph.NewBuilder` chain), that can be a follow-up, but syntax validation
is the minimum needed to make the existing `FallbackOp` path reachable.

---

### Issue 2 — Unchecked `GetOutput` + unsafe type assertions will panic

**File:** `main.go:208–214`, `main.go:246–247`, `main.go:273–274`

**Problem:**
```go
promptRaw, _ := eng1.GetOutput("prompt_out")   // error silently dropped
libraryRaw, _ := eng1.GetOutput("library_out")
designRaw, _ := eng1.GetOutput("design_out")

promptStr := *(promptRaw.(*string))   // panics if promptRaw is nil
libraryStr := *(libraryRaw.(*string))
approvedDesign := *(designRaw.(*string))
```
Same pattern at `main.go:246` for `eng2` and `main.go:273` for `eng3`.
If any vertex was skipped or errored, `promptRaw` is `nil` and the type assertion panics.

**Fix:**
Add a helper and use it everywhere:

```go
func mustGetString(eng *dagor.Engine, key string) string {
    raw, err := eng.GetOutput(key)
    if err != nil {
        log.Fatalf("GetOutput(%q): %v", key, err)
    }
    if raw == nil {
        log.Fatalf("GetOutput(%q): nil output (vertex may have been skipped)", key)
    }
    v, ok := raw.(*string)
    if !ok {
        log.Fatalf("GetOutput(%q): expected *string, got %T", key, raw)
    }
    return *v
}
```

Replace all three blocks:
```go
// Phase 1
promptStr    := mustGetString(eng1, "prompt_out")
libraryStr   := mustGetString(eng1, "library_out")
approvedDesign := mustGetString(eng1, "design_out")

// Phase 2 (refinement loop)
approvedDesign = mustGetString(eng2, "design_out")

// Phase 3 — binPath and mcpbPath are optional (vertices can be skipped)
// keep the existing nil-guard pattern for those.
```

---

### Issue 3 — `anthropic-sdk-go` listed as `// indirect` in `go.mod`

**File:** `go.mod:17`

**Problem:**
```
github.com/anthropics/anthropic-sdk-go v1.37.0 // indirect
```
The SDK is used directly throughout `driver_ops.go` and `library/`. `// indirect` means
it is not directly imported by the top-level module, which is incorrect and can cause
version resolution surprises.

**Fix:**
Run:
```bash
go get github.com/anthropics/anthropic-sdk-go@v1.37.0
go mod tidy
```
This promotes it to a direct dependency in `go.mod`.

---

### Issue 4 — Magic sentinel string `"COMPILE_FAILED"` scattered across four files

**Files:** `driver_ops.go:402`, `driver_ops.go:426`, `driver_ops.go:756`, `driver_ops.go:825`, `driver_ops.go:931`

**Problem:**
```go
// In CompileOp.Run()
op.BinPath = "COMPILE_FAILED"

// In RunOp.Run()
if *op.BinPath == "COMPILE_FAILED" || *op.BinPath == "" {

// In MCPBManifestAIOp.Run()
if *op.BinPath == "COMPILE_FAILED" || *op.BinPath == "" {

// In MCPBManifestPromptOp.Run()
if *op.BinPath == "COMPILE_FAILED" || *op.BinPath == "" {

// In PackageMCPBOp.Run()
if *op.BinPath == "COMPILE_FAILED" || *op.BinPath == "" {
```
A rename or typo in any one location silently breaks the entire pipeline.

**Fix:**
Add a package-level constant near the top of `driver_ops.go` (below imports):
```go
const compileFailed = "COMPILE_FAILED"
```

Replace every occurrence of the string literal with the constant. In `CompileOp.Run()`:
```go
op.BinPath = compileFailed
```
In all consumers:
```go
if *op.BinPath == compileFailed || *op.BinPath == "" {
```

---

## Important

---

### Issue 5 — No prompt caching on any Claude API call

**Files:** `driver_ops.go` (all AI op `Run()` methods), `library/ai_compute_op.go:174–183`, `library/ai_ops.go` (all bespoke op `Run()` methods), `library/mode_select_op.go:84–93`

**Problem:**
Every request is sent without cache-control headers. The system prompt and static context
are identical across all retries within a single `Run()` invocation, and in many cases
identical across multiple op invocations in the same pipeline run. This is unnecessary
latency and cost.

**Fix:**
Use `anthropic.CacheControlEphemeralParam` on the system prompt block for every API call
that has a retry loop. Example for `AIComputeOp.Run()` (`library/ai_compute_op.go:174`):

```go
msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
    Model:     anthropic.ModelClaudeSonnet4_6,
    MaxTokens: 16 * 1024,
    System: []anthropic.TextBlockParam{
        {
            Text: "Respond only with the requested format. Do not include any explanation or markdown formatting.",
            CacheControl: anthropic.F(anthropic.CacheControlEphemeralParam{
                Type: "ephemeral",
            }),
        },
    },
    Messages: []anthropic.MessageParam{
        anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
    },
})
```

Apply the same pattern to the first `System` block in every op that has a retry loop:
- `AIClassifyMultiLabelOp.Run()` (`library/ai_ops.go:155`)
- `AIScoreOp.Run()` (`library/ai_ops.go:284`)
- `AIBoolOp.Run()` (`library/ai_ops.go:400`)
- `AIBestMatchOp.Run()` (`library/ai_ops.go:525`)
- `AIRerankOp.Run()` (`library/ai_ops.go:655`)
- `ModeSelectOp.Run()` (`library/mode_select_op.go:84`)
- `DAGDesignOp.Run()` (`driver_ops.go:167`)
- `DAGDesignRefineOp.Run()` (`driver_ops.go:214`)
- `GenerateOp.Run()` and `FallbackOp.Run()` (streaming calls — cache the system block)
- `MCPBManifestAIOp.Run()` (`driver_ops.go:770`)

For the codegen prompt, also consider caching the static library description block in
the user message on retries (the base prompt does not change across retry iterations).

---

### Issue 6 — API client created per `Run()`, not per `Setup()`

**Files:** `driver_ops.go` (all AI ops), `library/ai_compute_op.go:131`, `library/ai_ops.go:142`, `library/mode_select_op.go:65`

**Problem:**
```go
func (op *DAGDesignOp) Run(ctx context.Context) error {
    apiKey := os.Getenv("CLAUDE_API_KEY")
    client := anthropic.NewClient(option.WithAPIKey(apiKey))
    ...
}
```
- A new HTTP client is created on every `Run()` invocation (and on every retry iteration
  inside `Run()`).
- An empty `CLAUDE_API_KEY` is not caught until the first actual API call, which may be
  deep into a run.

**Fix:**
Add a `client *anthropic.Client` field to each AI op struct and populate it in `Setup()`:

```go
type DAGDesignOp struct {
    Prompt             *string `dag:"input"`
    LibraryDescription *string `dag:"input"`
    Design             string  `dag:"output"`
    client             *anthropic.Client
}

func (op *DAGDesignOp) Setup(params *config.Params) error {
    apiKey := os.Getenv("CLAUDE_API_KEY")
    if apiKey == "" {
        return fmt.Errorf("DAGDesignOp: CLAUDE_API_KEY environment variable is not set")
    }
    op.client = anthropic.NewClient(option.WithAPIKey(apiKey))
    return nil
}
```

Apply to: `DAGDesignOp`, `DAGDesignRefineOp`, `GenerateOp`, `FallbackOp`,
`MCPBManifestAIOp`, `AIComputeOp`, `AIClassifyMultiLabelOp`, `AIScoreOp`,
`AIBoolOp`, `AIBestMatchOp`, `AIRerankOp`, `ModeSelectOp`.

For `AIComputeOp[In, Out]`, add the field to the generic struct and populate in `Setup()`.

---

### Issue 7 — No retry backoff or rate-limit awareness

**Files:** `library/ai_compute_op.go:165–203`, `library/ai_ops.go` (all retry loops), `library/mode_select_op.go:83–116`

**Problem:**
All retry loops execute immediately (`continue` with no delay). If the API returns a
rate-limit error or transient 5xx, the code retries immediately in a tight loop, which
worsens the rate-limit situation. Transport errors also cause immediate failure (`return
fmt.Errorf(...)`) rather than a retry.

**Fix:**
Distinguish transport errors from parse errors. Wrap the API call with exponential backoff
for transport/rate-limit errors, and keep the existing immediate-retry logic for parse
failures only:

```go
import (
    "time"
    "net/http"
)

// Inside the retry loop, after `client.Messages.New(...)`:
if err != nil {
    // Check if it's a rate-limit or server error worth retrying
    var apiErr *anthropic.Error
    if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= 500) {
        delay := time.Duration(1<<attempt) * time.Second // 1s, 2s, 4s, ...
        slog.WarnContext(ctx, "API transient error, backing off", "attempt", attempt, "delay", delay, "err", err)
        select {
        case <-time.After(delay):
        case <-ctx.Done():
            return ctx.Err()
        }
        continue
    }
    return fmt.Errorf("generate content: %w", err)
}
```

Apply this pattern in `AIComputeOp.Run()` and all bespoke AI op `Run()` methods.

---

### Issue 8 — `PackageMCPBOp` hardcodes Windows-only manifest

**File:** `driver_ops.go:1000–1012`

**Problem:**
```go
Server: serverBlock{
    Type:       "binary",
    EntryPoint: "server/solution_bin.exe",      // always .exe
    MCPConfig: mcpConfig{
        Command: "${__dirname}/server/solution_bin.exe",  // always .exe
        Args:    []string{"--mode", "mcp"},
        Env:     envMap,
    },
},
...
Compatibility: compatibility{Platforms: []string{"win32"}},  // always win32
```
Also at `driver_ops.go:1035`:
```go
bf, err := zw.Create("server/solution_bin.exe")  // always .exe zip entry
```
The binary compiled by `CompileOp`/`FallbackOp` uses `runtime.GOOS` to conditionally
add `.exe`, but the manifest and zip always use `.exe`.

**Fix:**
Derive the binary name and platform from `runtime.GOOS`:

```go
binName := "solution_bin"
platform := "linux"
if runtime.GOOS == "windows" {
    binName += ".exe"
    platform = "win32"
} else if runtime.GOOS == "darwin" {
    platform = "darwin"
}

entryPoint := fmt.Sprintf("server/%s", binName)

// Use entryPoint everywhere instead of the hardcoded string:
Server: serverBlock{
    Type:       "binary",
    EntryPoint: entryPoint,
    MCPConfig: mcpConfig{
        Command: fmt.Sprintf("${__dirname}/%s", entryPoint),
        Args:    []string{"--mode", "mcp"},
        Env:     envMap,
    },
},
Compatibility: compatibility{Platforms: []string{platform}},
```

And in the zip creation:
```go
bf, err := zw.Create(entryPoint)
```

---

### Issue 9 — `FallbackOp` duplicates `WriteFilesOp` + `CompileOp` logic

**File:** `driver_ops.go:624–673`

**Problem:**
`FallbackOp.Run()` manually re-implements:
1. Writing `go.mod` (identical to `WriteFilesOp`)
2. Running `go mod tidy` (identical to `WriteFilesOp`)
3. Running `gofmt` (not in `WriteFilesOp` — unique here)
4. Compiling the binary (identical to `CompileOp`, but failure is hard error here)

Any change to module path construction or build flags must be made in two places.

**Fix:**
Extract shared helpers. At minimum, extract the go.mod content generation:

```go
// generateGoMod returns the go.mod content for a generated solution binary.
func generateGoMod(dagAIModulePath string) string {
    modPath := filepath.ToSlash(dagAIModulePath)
    dagorPath := filepath.ToSlash(filepath.Join(filepath.Dir(dagAIModulePath), "dagor"))
    return fmt.Sprintf(
        "module solution\n\ngo %s\n\nrequire github.com/akennis/clawdag-go v0.0.0\n\nreplace github.com/akennis/clawdag-go => %s\nreplace github.com/wwz16/dagor => %s\n",
        goVersion(), modPath, dagorPath,
    )
}

// writeAndTidy writes main.go + go.mod to dir and runs go mod tidy.
func writeAndTidy(ctx context.Context, dir, dagAIModulePath, src string) error {
    if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(generateGoMod(dagAIModulePath)), 0644); err != nil {
        return fmt.Errorf("write go.mod: %w", err)
    }
    if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0644); err != nil {
        return fmt.Errorf("write main.go: %w", err)
    }
    tidy := exec.CommandContext(ctx, "go", "mod", "tidy")
    tidy.Dir = dir
    tidy.Env = os.Environ()
    if out, err := tidy.CombinedOutput(); err != nil {
        return fmt.Errorf("go mod tidy: %w\n%s", err, out)
    }
    return nil
}
```

Use `writeAndTidy` in both `WriteFilesOp.Run()` and `FallbackOp.Run()`.

---

### Issue 10 — `HTTPGetOp` has no timeout or body size limit

**File:** `library/io_ops.go:54–69`

**Problem:**
```go
func (op *HTTPGetOp) Run(ctx context.Context) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, *op.URL, nil)
    ...
    resp, err := http.DefaultClient.Do(req)
    ...
    data, err := io.ReadAll(resp.Body)
```
`http.DefaultClient` has no timeout. `io.ReadAll` has no size limit. A slow or large
response blocks the goroutine until the 60-minute context deadline.

**Fix:**
Use a package-level client with a timeout and cap the body read:

```go
var httpClient = &http.Client{Timeout: 30 * time.Second}

const maxHTTPBodyBytes = 10 << 20 // 10 MB

func (op *HTTPGetOp) Run(ctx context.Context) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, *op.URL, nil)
    if err != nil {
        return fmt.Errorf("HTTPGetOp: build request: %w", err)
    }
    resp, err := httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("HTTPGetOp: %w", err)
    }
    defer resp.Body.Close()
    data, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPBodyBytes))
    if err != nil {
        return fmt.Errorf("HTTPGetOp: read body: %w", err)
    }
    op.Body = string(data)
    op.StatusCode = resp.StatusCode
    return nil
}
```

---

### Issue 11 — `goVersion()` may return an invalid go.mod directive

**File:** `driver_ops.go:63–66`

**Problem:**
```go
func goVersion() string {
    v := strings.TrimPrefix(runtime.Version(), "go")
    return v
}
```
`runtime.Version()` can return:
- `go1.22.2` → strips to `1.22.2` ✓
- `devel +abc123 Mon Jan 01 ...` → strips nothing (no `go` prefix) → invalid in go.mod ✗

**Fix:**
Parse and validate the version, falling back to a safe default:

```go
func goVersion() string {
    v := strings.TrimPrefix(runtime.Version(), "go")
    // Keep only the numeric semver portion (e.g. "1.22.2" from "1.22.2 linux/amd64").
    if idx := strings.IndexFunc(v, func(r rune) bool {
        return r != '.' && (r < '0' || r > '9')
    }); idx > 0 {
        v = v[:idx]
    }
    // Validate: must look like X.Y or X.Y.Z
    parts := strings.Split(v, ".")
    if len(parts) < 2 {
        return "1.21.0" // safe minimum
    }
    return v
}
```

---

### Issue 12 — `LibraryScanOp` hardcodes op list and has stale log count

**File:** `driver_ops.go:100–116`

**Problem:**
```go
func (op *LibraryScanOp) Run(ctx context.Context) error {
    op.LibraryDescription = strings.Join([]string{
        dagailib.AddOpDescription,
        dagailib.SubOpDescription,
        ...
    }, "\n")
    slog.DebugContext(ctx, "LibraryScanOp.done", ..., "op_count", 10)  // hardcoded 10
```
Adding a new library op requires manually updating this list. The log count `10` is
already wrong — there are more ops in the library.

**Fix:**
Create a registry slice in the `library` package:

In `library/registry.go` (new file):
```go
package library

// AllOpDescriptions is the ordered list of all library op descriptions
// exposed to the DAG design AI.
var AllOpDescriptions = []string{
    AddOpDescription,
    SubOpDescription,
    DivOpDescription,
    MulOpDescription,
    RoundOpDescription,
    ClampOpDescription,
    PackMathOperandsOpDescription,
    AIComputeMathOperandsToFloat64OpDescription,
    StringLookupOpDescription,
    StringToLowerOpDescription,
    StringConcatOpDescription,
    AIComputeStringToStringOpDescription,
    CityTimeOpDescription,
    ModeSelectOpDescription,
    // add new op descriptions here
}
```

In `LibraryScanOp.Run()`:
```go
op.LibraryDescription = strings.Join(dagailib.AllOpDescriptions, "\n")
slog.DebugContext(ctx, "LibraryScanOp.done", "run_id", dagor.RunID(ctx), "op_count", len(dagailib.AllOpDescriptions))
```

---

## Recommended

---

### Issue 13 — `ModeSelectOp` rebuilds category set on every `Run()`

**File:** `library/mode_select_op.go:76–78`

**Problem:**
```go
func (op *ModeSelectOp) Run(ctx context.Context) error {
    ...
    catSet := make(map[string]bool, len(op.categories))
    for _, c := range op.categories {
        catSet[c] = true
    }
```
`catSet` is rebuilt from `op.categories` on every `Run()`. Since `op.categories` is set
in `Setup()` and never changes, this is wasteful (and fragile if `Run()` is called many
times via pool reuse).

**Fix:**
Add `catSet` as a struct field and populate it in `Setup()`:
```go
type ModeSelectOp struct {
    Input     *string `dag:"input"`
    Result    string  `dag:"output"`
    Reasoning string  `dag:"output"`
    categories []string
    catSet     map[string]bool  // add this
    maxRetries int
}
```
In `Setup()`, after building `op.categories`:
```go
op.catSet = make(map[string]bool, len(op.categories))
for _, c := range op.categories {
    op.catSet[c] = true
}
```
Remove the `catSet` construction from `Run()` and use `op.catSet` directly.

---

### Issue 14 — Two `bufio.NewReader(os.Stdin)` instances share stdin

**File:** `driver_ops.go:84` and `main.go:218`

**Problem:**
```go
// PromptOp.Run() — driver_ops.go:84
line, err := bufio.NewReader(os.Stdin).ReadString('\n')

// main() review loop — main.go:218
reader := bufio.NewReader(os.Stdin)
```
Two separate `bufio.Reader` wrappers both read from `os.Stdin`. A buffered reader
reads ahead in chunks — bytes consumed by one reader are invisible to the other. If
`PromptOp` runs first and its reader buffers ahead past the newline, subsequent reads
in `main()` may miss characters.

**Fix:**
Create a single `bufio.Reader` in `main()` before the DAG runs and pass it as a param
to ops that need it via `Setup()`. Alternatively, since `PromptOp` runs in Phase 1 and
`main()` reads in Phase 2, and neither overlaps, the simplest fix is to make `PromptOp`
use an unbuffered read:

```go
func (op *PromptOp) Run(ctx context.Context) error {
    fmt.Print("Enter prompt: ")
    // Read exactly one line without buffering ahead.
    var line string
    _, err := fmt.Fprintln(os.Stdout, "")  // flush
    scanner := bufio.NewScanner(os.Stdin)
    scanner.Buffer(make([]byte, 4096), 4096)
    if scanner.Scan() {
        line = scanner.Text()
    } else if err := scanner.Err(); err != nil {
        return fmt.Errorf("reading prompt: %w", err)
    }
    op.Prompt = strings.TrimSpace(line)
    ...
}
```

But the cleanest long-term fix is to inject a shared `*bufio.Reader` via params or a
constructor so all stdin reads share one buffered reader.

---

### Issue 15 — `MCPBManifestAIOp` silently swallows CSV parse failures

**File:** `driver_ops.go:794–797`

**Problem:**
```go
r := csv.NewReader(strings.NewReader(raw))
fields, err := r.Read()
if err != nil || len(fields) < 3 {
    slog.DebugContext(ctx, "MCPBManifestAIOp.csv_error", ...)
    return nil   // silent swallow at Debug level
}
```
A parse failure is logged at `Debug` level and the op returns `nil`. All three output
fields stay as empty strings. Downstream `MCPBManifestPromptOp` silently falls back to
heuristics. Operators monitoring the pipeline at `Info` or higher will see no indication
that the AI-generated manifest metadata was discarded.

**Fix:**
Elevate the log level to `Warn`:
```go
if err != nil || len(fields) < 3 {
    slog.WarnContext(ctx, "MCPBManifestAIOp: failed to parse CSV response; falling back to heuristics",
        "run_id", dagor.RunID(ctx), "err", err, "raw", raw)
    return nil
}
```

---

### Issue 16 — `EnvScanOp` AI-detection heuristic is fragile

**File:** `driver_ops.go:725–730`

**Problem:**
```go
aiOpRe := regexp.MustCompile(`\b\w*(AI|Compute)\w*Op\b`)
hasAI := strings.Contains(src, "AIComputeOp") ||
    strings.Contains(src, "ModeSelectOp") ||
    aiOpRe.MatchString(src)
```
Issues:
1. `"AIComputeOp"` is not a real registered op name — concrete registered names are
   `AIComputeMathOperandsToFloat64Op`, `AIComputeStringToStringOp`, etc.
2. The regex `\w*(AI|Compute)\w*Op` matches any op containing "AI" or "Compute",
   including hypothetical deterministic ops with those words in their names.
3. `ModeSelectOp` is hardcoded by name but other AI ops (`AIBoolOp`, `AIScoreOp`, etc.)
   are not.

**Fix:**
Replace the heuristic with an explicit set of known AI op names:
```go
var aiOpNames = []string{
    "AIComputeMathOperandsToFloat64Op",
    "AIComputeStringToStringOp",
    "AIExtractStringSliceOp",
    "AIExtractMapOp",
    "AIParseNumberOp",
    "AISummarizeOp",
    "AIClassifyMultiLabelOp",
    "AIScoreOp",
    "AIBoolOp",
    "AIBestMatchOp",
    "AIRerankOp",
    "ModeSelectOp",
}

hasAI := false
for _, name := range aiOpNames {
    if strings.Contains(src, name) {
        hasAI = true
        break
    }
}
```

---

### Issue 17 — No unit tests for any driver ops

**File:** (new file) `driver_ops_test.go`

**Problem:**
`WriteFilesOp`, `CompileOp`, `FallbackOp`, `GenerateOp`, `EnvScanOp`, `ValidateDAGOp`,
`MCPBManifestAIOp`, and others have zero test coverage. The `Setup()` validation paths
and field logic are untested.

**Fix:**
Add at minimum the following unit tests (no API key required):

```go
package main

import (
    "testing"
    "strings"
)

func TestCompileFailed_Constant(t *testing.T) {
    // Ensures the sentinel is defined and non-empty.
    if compileFailed == "" {
        t.Fatal("compileFailed constant must not be empty")
    }
}

func TestEnvScanOp_DetectsAIOp(t *testing.T) {
    op := &EnvScanOp{}
    src := `package main
import "github.com/akennis/clawdag-go/library"
var _ = library.AIBoolOp{}`
    op.GoFiles = &src
    if err := op.Run(nil); err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(op.RequiredEnvVars, "CLAUDE_API_KEY") {
        t.Errorf("expected CLAUDE_API_KEY in RequiredEnvVars, got %s", op.RequiredEnvVars)
    }
}

func TestEnvScanOp_DetectsGetenv(t *testing.T) {
    op := &EnvScanOp{}
    src := `package main
import "os"
func main() { _ = os.Getenv("MY_SECRET") }`
    op.GoFiles = &src
    if err := op.Run(nil); err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(op.RequiredEnvVars, "MY_SECRET") {
        t.Errorf("expected MY_SECRET in RequiredEnvVars, got %s", op.RequiredEnvVars)
    }
}

func TestValidateDAGOp_InvalidSyntax(t *testing.T) {
    op := &ValidateDAGOp{}
    bad := "package main\nfunc main() { INVALID SYNTAX"
    op.GoFiles = &bad
    if err := op.Run(nil); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if op.ValidationError == "" {
        t.Error("expected ValidationError to be set for invalid Go source")
    }
}

func TestValidateDAGOp_ValidSyntax(t *testing.T) {
    op := &ValidateDAGOp{}
    good := "package main\nfunc main() {}"
    op.GoFiles = &good
    if err := op.Run(nil); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if op.ValidationError != "" {
        t.Errorf("expected empty ValidationError, got %q", op.ValidationError)
    }
}

func TestStripMarkdownFences(t *testing.T) {
    cases := []struct{ in, want string }{
        {"```go\npackage main\n```", "package main"},
        {"```json\n{}\n```", "{}"},
        {"```\nfoo\n```", "foo"},
        {"package main", "package main"},
    }
    for _, c := range cases {
        got := stripMarkdownFences(c.in)
        if got != c.want {
            t.Errorf("stripMarkdownFences(%q) = %q, want %q", c.in, got, c.want)
        }
    }
}
```

---

## Nice-to-Have

---

### Issue 18 — `log.Printf("[DEBUG] ...")` calls in `main()` bypass slog

**File:** `main.go:256–267`

**Problem:**
```go
log.Printf("[DEBUG] codegen DAG built: %d vertices", codegenDAG.Size())
for name := range codegenDAG.Vertices() {
    log.Printf("[DEBUG]   vertex: %s", name)
}
...
log.Printf("[DEBUG] codegen DAG run finished, err=%v", runErr)
log.Printf("[DEBUG] envscan skipped=%v, ...")
```
The `slog` logger is configured at `Debug` level on `stderr`. These `log.Printf` calls
always print regardless of log level and don't use the structured format.

**Fix:**
Replace with `slog.DebugContext`:
```go
slog.DebugContext(ctx, "codegen DAG built", "vertex_count", codegenDAG.Size())
for name := range codegenDAG.Vertices() {
    slog.DebugContext(ctx, "codegen DAG vertex", "name", name)
}
...
slog.DebugContext(ctx, "codegen DAG run finished", "err", runErr)
slog.DebugContext(ctx, "codegen DAG vertex skipped",
    "envscan", eng3.VertexSkipped("envscan"),
    "mcpbai", eng3.VertexSkipped("mcpbai"),
    "mcpbprompt", eng3.VertexSkipped("mcpbprompt"),
    "package", eng3.VertexSkipped("package"),
)
```

---

### Issue 19 — Spurious blank lines at start of two `Run()` methods

**File:** `library/ai_ops.go:501–504` and `library/ai_ops.go:631–634`

**Problem:**
```go
func (op *AIBestMatchOp) Run(ctx context.Context) error {


    n := len(*op.Candidates)
```
Two blank lines between the function signature and first statement (same in `AIRerankOp.Run()`).

**Fix:**
Delete the extra blank lines in both functions.

---

### Issue 20 — `google/generative-ai-go` appears as a direct dependency but is unused

**File:** `go.mod:6`

**Problem:**
```
github.com/google/generative-ai-go v0.20.1
google.golang.org/api v0.276.0
```
Listed as direct dependencies. Searching the source finds no import of the Gemini SDK.

**Fix:**
Run `go mod tidy` and verify these are removed. If they were kept intentionally (e.g.,
for a planned Gemini fallback), add a comment in `go.mod` explaining why, and add a
blank import in a `_` file to pin them as direct:
```go
// doc.go or tools.go
import _ "github.com/google/generative-ai-go/genai"
```
Otherwise, remove them.

---

*End of review. Issues are ordered by severity within each section. Start with the
Critical section and work downward.*
