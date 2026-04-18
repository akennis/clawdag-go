package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	dagailib "github.com/akennis/clawdag-go/library"

	"github.com/google/generative-ai-go/genai"
	"github.com/wwz16/dagor/config"
	"google.golang.org/api/option"
)

// PromptOp reads a user prompt from stdin.
type PromptOp struct {
	Prompt string `dag:"output"`
}

func (op *PromptOp) Setup(params *config.Params) error { return nil }
func (op *PromptOp) Reset() error                      { return nil }
func (op *PromptOp) Run(ctx context.Context) error {
	log.Printf("[DEBUG] PromptOp: waiting for user input")
	fmt.Print("Enter prompt: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading prompt: %w", err)
	}
	op.Prompt = strings.TrimSpace(line)
	log.Printf("[DEBUG] PromptOp: prompt=%q", op.Prompt)
	return nil
}

// LibraryScanOp collects descriptions of all available library ops.
type LibraryScanOp struct {
	LibraryDescription string `dag:"output"`
}

func (op *LibraryScanOp) Setup(params *config.Params) error { return nil }
func (op *LibraryScanOp) Reset() error                      { return nil }
func (op *LibraryScanOp) Run(ctx context.Context) error {
	log.Printf("[DEBUG] LibraryScanOp: collecting library op descriptions")
	op.LibraryDescription = strings.Join([]string{
		dagailib.ConstOpDescription,
		dagailib.AddOpDescription,
		dagailib.SubOpDescription,
		dagailib.DivOpDescription,
		dagailib.PackMathOperandsOpDescription,
		dagailib.AIComputeMathOperandsToFloat64OpDescription,
	}, "\n")
	log.Printf("[DEBUG] LibraryScanOp: loaded %d ops", 6)
	return nil
}

// GenerateOp calls Gemini to generate Go source files for the solution.
type GenerateOp struct {
	Prompt             *string `dag:"input"`
	LibraryDescription *string `dag:"input"`
	GoFiles            string  `dag:"output"` // JSON: map[string]string
}

func (op *GenerateOp) Setup(params *config.Params) error { return nil }
func (op *GenerateOp) Reset() error                      { return nil }
func (op *GenerateOp) Run(ctx context.Context) error {
	log.Printf("[DEBUG] GenerateOp: calling Gemini")
	apiKey := os.Getenv("GOOGLE_GENAI_API_KEY")
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("genai client: %w", err)
	}
	defer client.Close()

	// modelsIter := client.ListModels(ctx)
	// var models []string
	// for {
	// 	model, err := modelsIter.Next()
	// 	if err == iterator.Done {
	// 		break
	// 	}
	// 	if err != nil {
	// 		return fmt.Errorf("list models: %w", err)
	// 	}
	// 	models = append(models, model.Name)
	// }
	// fmt.Println(models)

	model := client.GenerativeModel("gemini-flash-latest")
	model.ResponseMIMEType = "application/json"
	model.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"main_go": {Type: genai.TypeString},
		},
		Required: []string{"main_go"},
	}

	var sb strings.Builder
	sb.WriteString("You are a Go code generator for a DAG-based computation system.\n\n")
	sb.WriteString("FRAMEWORK: github.com/wwz16/dagor\n\n")
	sb.WriteString("GRAPH JSON SCHEMA:\n")
	sb.WriteString("  { \"name\": \"...\", \"vertices\": { \"<name>\": { \"op\": \"<Type>\", \"params\": {...},\n")
	sb.WriteString("    \"inputs\": {\"<Field>\": \"<wire>\"}, \"outputs\": {\"<Field>\": \"<wire>\"} } } }\n\n")
	sb.WriteString("REQUIRED IMPORTS — use exactly these, no others:\n")
	sb.WriteString("  \"context\", \"encoding/json\", \"fmt\", \"log\", \"time\"\n")
	sb.WriteString("  _ \"github.com/akennis/clawdag-go/library\"\n")
	sb.WriteString("  \"github.com/panjf2000/ants/v2\"\n")
	sb.WriteString("  \"github.com/wwz16/dagor\"\n")
	sb.WriteString("  \"github.com/wwz16/dagor/graph\"\n")
	sb.WriteString("  DO NOT import any other packages. Every import must be used.\n\n")
	sb.WriteString("HOW TO RUN A DAGOR GRAPH — use exactly this pattern:\n")
	sb.WriteString("  pool, _ := ants.NewPool(10); defer pool.Release()\n")
	sb.WriteString("  g, err := graph.NewGraphFromJson(json.RawMessage(dagJSON))\n")
	sb.WriteString("  eng, err := dagor.NewEngine(g, pool)\n")
	sb.WriteString("  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute); defer cancel()\n")
	sb.WriteString("  if err := eng.Run(ctx); err != nil { log.Fatal(err) }\n")
	sb.WriteString("  raw, ok := eng.GetOutput(\"wire_name\")  // returns (any, bool); cast result to *float64, *string, etc.\n\n")
	sb.WriteString("CRITICAL — DO NOT iterate over g.Vertices or inspect graph internals:\n")
	sb.WriteString("  WRONG: for _, v := range g.Vertices { ... }  // g.Vertices is a func, not a map — compile error\n")
	sb.WriteString("  WRONG: vertex.Inputs  // map[string]string, not an interface — type mismatch error\n")
	sb.WriteString("  RIGHT: use eng.GetOutput(\"wire_name\") for every value you need — always by wire name.\n\n")
	sb.WriteString("WIRE NAMING: wire names are arbitrary strings you assign in \"outputs\", then reference in \"inputs\".\n")
	sb.WriteString("  NOT \"vertex.Field\" syntax — just plain strings like \"val_a\", \"mul_result\".\n\n")
	sb.WriteString("GO SYNTAX RULE — TRAILING COMMAS: Go requires a trailing comma after the LAST element\n")
	sb.WriteString("  of every multi-line composite literal (map, slice, struct). Forgetting this is a compile error.\n")
	sb.WriteString("  WRONG:                             RIGHT:\n")
	sb.WriteString("    map[string]any{                    map[string]any{\n")
	sb.WriteString("      \"a\": 1,                           \"a\": 1,\n")
	sb.WriteString("      \"b\": 2   // ← missing comma       \"b\": 2,  // ← required\n")
	sb.WriteString("    }                                  }\n\n")
	sb.WriteString("STDOUT: result JSON MUST go to stdout via fmt.Println — NOT log.Printf (that goes to stderr).\n\n")
	sb.WriteString("AVAILABLE OPS (blank-import _ \"github.com/akennis/clawdag-go/library\" registers ALL of them — this import is REQUIRED):\n")
	sb.WriteString(*op.LibraryDescription)
	sb.WriteString("\n\n")
	sb.WriteString("KEY RULE: you may ONLY use ops listed above. There is NO MulOp, MultiplyOp, or any other op.\n")
	sb.WriteString("  For any operation not covered by the library (e.g. multiplication, power, modulo),\n")
	sb.WriteString("  use PackMathOperandsOp + AIComputeMathOperandsToFloat64Op.\n\n")
	sb.WriteString("EXAMPLE — solving 2 * 3 + 4 using AI for multiply and AddOp for add:\n")
	sb.WriteString("  Vertices:\n")
	sb.WriteString("    \"c2\":   {\"op\":\"ConstOp\", \"params\":{\"Value\":\"2\"}, \"outputs\":{\"Result\":\"v2\"}},\n")
	sb.WriteString("    \"c3\":   {\"op\":\"ConstOp\", \"params\":{\"Value\":\"3\"}, \"outputs\":{\"Result\":\"v3\"}},\n")
	sb.WriteString("    \"c4\":   {\"op\":\"ConstOp\", \"params\":{\"Value\":\"4\"}, \"outputs\":{\"Result\":\"v4\"}},\n")
	sb.WriteString("    \"pack\": {\"op\":\"PackMathOperandsOp\",\n")
	sb.WriteString("              \"inputs\":{\"A\":\"v2\",\"B\":\"v3\"}, \"outputs\":{\"Result\":\"operands\"}},\n")
	sb.WriteString("    \"mul\":  {\"op\":\"AIComputeMathOperandsToFloat64Op\", \"params\":{\"operation\":\"multiply A by B\", \"max_retries\":\"3\"},\n")
	sb.WriteString("              \"inputs\":{\"Input\":\"operands\"}, \"outputs\":{\"Result\":\"mul_res\",\"Reasoning\":\"mul_why\"}},\n")
	sb.WriteString("    \"add\":  {\"op\":\"AddOp\", \"inputs\":{\"A\":\"mul_res\",\"B\":\"v4\"}, \"outputs\":{\"Result\":\"final\"}}\n")
	sb.WriteString("  Read result:    raw, _ := eng.GetOutput(\"final\"); val := *(raw.(*float64))\n")
	sb.WriteString("  Read reasoning: rRaw, _ := eng.GetOutput(\"mul_why\"); reasoning := *(rRaw.(*string))\n")
	sb.WriteString("  Read AI result: arRaw, _ := eng.GetOutput(\"mul_res\"); aiResult := *(arRaw.(*float64))\n\n")
	sb.WriteString("TASK: ")
	sb.WriteString(*op.Prompt)
	sb.WriteString("\n\n")
	sb.WriteString("OUTPUT REQUIREMENTS — generate main_go: a complete compilable package main that:\n")
	sb.WriteString("  * uses ONLY the required imports listed above (no extra packages)\n")
	sb.WriteString("  * blank-imports _ \"github.com/akennis/clawdag-go/library\" (REQUIRED — never omit this)\n")
	sb.WriteString("  * does NOT import \"github.com/wwz16/dagor/operator\" (not needed — library registers ops)\n")
	sb.WriteString("  * adds a trailing comma after the last element of EVERY multi-line composite literal\n")
	sb.WriteString("  * uses the exact dagor engine pattern above\n")
	sb.WriteString("  * prints to stdout: {\"result\":\"<answer as string>\",\"ai_nodes\":[{\"op\":\"AIComputeMathOperandsToFloat64Op\",\"inputs\":{...},\"output\":<num>,\"reasoning\":\"...\"}]}\n")
	sb.WriteString("  * result MUST be a JSON string (use fmt.Sprintf(\"%g\", val) to convert float64 to string)\n")
	sb.WriteString("  * ai_nodes contains one entry per AIComputeMathOperandsToFloat64Op vertex, with 'output' from its Result wire and 'reasoning' from its Reasoning wire\n")
	sb.WriteString("  * ai_nodes is [] if no AI op was used\n")
	sb.WriteString("  * calls log.Fatal on any error\n")

	resp, err := model.GenerateContent(ctx, genai.Text(sb.String()))
	if err != nil {
		return fmt.Errorf("generate content: %w", err)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return fmt.Errorf("no candidates in response")
	}

	var raw string
	for _, part := range resp.Candidates[0].Content.Parts {
		raw += fmt.Sprintf("%v", part)
	}

	var files map[string]string
	if err := json.Unmarshal([]byte(raw), &files); err != nil {
		return fmt.Errorf("parse generated JSON: %w\nraw: %s", err, raw)
	}
	if files["main_go"] == "" {
		return fmt.Errorf("generated JSON missing main_go\nraw: %s", raw)
	}
	op.GoFiles = raw
	log.Printf("[DEBUG] GenerateOp: received main_go (%d bytes)", len(files["main_go"]))
	return nil
}

// WriteFilesOp writes generated Go files to a temp directory.
type WriteFilesOp struct {
	GoFiles *string `dag:"input"`
	TempDir string  `dag:"output"`

	dagAIModulePath string // injected via Setup params
}

func (op *WriteFilesOp) Setup(params *config.Params) error {
	op.dagAIModulePath = params.GetString("dag_ai_module_path", "")
	return nil
}
func (op *WriteFilesOp) Reset() error { return nil }
func (op *WriteFilesOp) Run(ctx context.Context) error {
	log.Printf("[DEBUG] WriteFilesOp: parsing generated files JSON")
	var files map[string]string
	if err := json.Unmarshal([]byte(*op.GoFiles), &files); err != nil {
		return fmt.Errorf("parse GoFiles: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("UserHomeDir: %w", err)
	}
	tmpDir := filepath.Join(home, ".dag-ai", "solution")
	log.Printf("[DEBUG] WriteFilesOp: preparing dir %s", tmpDir)

	// Wipe and recreate so each attempt starts clean
	if err := os.RemoveAll(tmpDir); err != nil {
		return fmt.Errorf("remove solution dir: %w", err)
	}
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("mkdir solution dir: %w", err)
	}

	// Write go.mod (not from AI)
	modPath := filepath.ToSlash(op.dagAIModulePath)
	goMod := fmt.Sprintf("module solution\n\ngo 1.24\n\nrequire github.com/akennis/clawdag-go v0.0.0\n\nreplace github.com/akennis/clawdag-go => %s\n", modPath)
	log.Printf("[DEBUG] WriteFilesOp: writing go.mod (replace github.com/akennis/clawdag-go => %s)", modPath)
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		return fmt.Errorf("write go.mod: %w", err)
	}

	// Write main.go from AI
	log.Printf("[DEBUG] WriteFilesOp: writing main.go (%d bytes)", len(files["main_go"]))
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(files["main_go"]), 0644); err != nil {
		return fmt.Errorf("write main.go: %w", err)
	}

	// Run go mod tidy to bootstrap go.sum
	log.Printf("[DEBUG] WriteFilesOp: running go mod tidy")
	tidy := exec.CommandContext(ctx, "go", "mod", "tidy")
	tidy.Dir = tmpDir
	tidy.Env = os.Environ()
	if out, err := tidy.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy: %w\n%s", err, out)
	}

	op.TempDir = tmpDir
	log.Printf("[DEBUG] WriteFilesOp: done, solution written to %s", tmpDir)
	return nil
}

// CodegenOp runs go generate in the temp directory.
type CodegenOp struct {
	TempDir  *string `dag:"input"`
	ExitCode int     `dag:"output"`
	Stderr   string  `dag:"output"`
}

func (op *CodegenOp) Setup(params *config.Params) error { return nil }
func (op *CodegenOp) Reset() error                      { return nil }
func (op *CodegenOp) Run(ctx context.Context) error {
	log.Printf("[DEBUG] CodegenOp: running go generate in %s", *op.TempDir)
	cmd := exec.CommandContext(ctx, "go", "generate", "./...")
	cmd.Dir = *op.TempDir
	cmd.Env = os.Environ()
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		op.ExitCode = 1
		op.Stderr = errBuf.String()
		log.Printf("[DEBUG] CodegenOp: exit_code=1 stderr=%q", op.Stderr)
	} else {
		log.Printf("[DEBUG] CodegenOp: exit_code=0")
	}
	return nil
}

// CompileOp compiles the solution binary.
type CompileOp struct {
	TempDir  *string `dag:"input"`
	BinPath  string  `dag:"output"`
	ExitCode int     `dag:"output"`
	Stderr   string  `dag:"output"`
}

func (op *CompileOp) Setup(params *config.Params) error { return nil }
func (op *CompileOp) Reset() error                      { return nil }
func (op *CompileOp) Run(ctx context.Context) error {
	log.Printf("[DEBUG] CompileOp: compiling solution in %s", *op.TempDir)
	binName := "solution_bin"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(*op.TempDir, binName)

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, "./...")
	cmd.Dir = *op.TempDir
	cmd.Env = os.Environ()
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		op.BinPath = "COMPILE_FAILED"
		op.ExitCode = 1
		op.Stderr = errBuf.String()
		log.Printf("[DEBUG] CompileOp: FAILED stderr=%q", op.Stderr)
		return nil
	}

	op.BinPath = binPath
	log.Printf("[DEBUG] CompileOp: OK bin=%s", binPath)
	return nil
}

// RunOp executes the compiled solution binary.
type RunOp struct {
	BinPath       *string `dag:"input"`
	CompileStderr *string `dag:"input"`
	Stdout        string  `dag:"output"`
	Stderr        string  `dag:"output"`
	ExitCode      int     `dag:"output"`
}

func (op *RunOp) Setup(params *config.Params) error { return nil }
func (op *RunOp) Reset() error                      { return nil }
func (op *RunOp) Run(ctx context.Context) error {
	if *op.BinPath == "COMPILE_FAILED" || *op.BinPath == "" {
		op.ExitCode = 1
		op.Stderr = *op.CompileStderr
		if op.Stderr == "" {
			op.Stderr = "binary not available"
		}
		log.Printf("[DEBUG] RunOp: skipped (compile failed): %s", op.Stderr)
		return nil
	}

	log.Printf("[DEBUG] RunOp: executing %s", *op.BinPath)
	cmd := exec.CommandContext(ctx, *op.BinPath)
	cmd.Env = os.Environ()
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		op.ExitCode = 1
	}
	op.Stdout = outBuf.String()
	op.Stderr = errBuf.String()
	log.Printf("[DEBUG] RunOp: exit_code=%d stdout_len=%d stderr_len=%d", op.ExitCode, len(op.Stdout), len(op.Stderr))
	if op.Stderr != "" {
		log.Printf("[DEBUG] RunOp: stderr=%q", op.Stderr)
	}
	return nil
}

// OutputOp parses run results and formats final output.
type OutputOp struct {
	RawStdout *string `dag:"input"`
	RawStderr *string `dag:"input"`
	ExitCode  *int    `dag:"input"`
	Result    string  `dag:"output"`
	AINodes   string  `dag:"output"`
	ErrorMsg  string  `dag:"output"`
}

func (op *OutputOp) Setup(params *config.Params) error { return nil }
func (op *OutputOp) Reset() error                      { return nil }
func (op *OutputOp) Run(ctx context.Context) error {
	log.Printf("[DEBUG] OutputOp: exit_code=%d stdout_len=%d stderr_len=%d", *op.ExitCode, len(*op.RawStdout), len(*op.RawStderr))

	if *op.ExitCode != 0 {
		op.ErrorMsg = *op.RawStderr
		if op.ErrorMsg == "" {
			op.ErrorMsg = "run failed with no stderr"
		}
		log.Printf("[DEBUG] OutputOp: non-zero exit: %s", op.ErrorMsg)
		return nil
	}

	stdout := strings.TrimSpace(*op.RawStdout)
	if stdout == "" {
		op.ErrorMsg = fmt.Sprintf("empty stdout; stderr: %s", *op.RawStderr)
		log.Printf("[DEBUG] OutputOp: empty stdout")
		return nil
	}

	// Parse flexibly so result can be either a JSON string or a number.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		op.ErrorMsg = fmt.Sprintf("parse output JSON: %v\nstdout: %s", err, stdout)
		log.Printf("[DEBUG] OutputOp: JSON parse error: %v", err)
		return nil
	}

	if r, ok := raw["result"]; ok {
		var s string
		if err := json.Unmarshal(r, &s); err == nil {
			op.Result = s
		} else {
			// AI returned a number instead of a string — convert it.
			var n json.Number
			if err2 := json.Unmarshal(r, &n); err2 == nil {
				op.Result = n.String()
				log.Printf("[DEBUG] OutputOp: result was numeric, coerced to string: %s", op.Result)
			}
		}
	}

	if nodes, ok := raw["ai_nodes"]; ok {
		var aiNodes []AINodeDiag
		if err := json.Unmarshal(nodes, &aiNodes); err == nil {
			aiNodesJSON, _ := json.Marshal(aiNodes)
			op.AINodes = string(aiNodesJSON)
		}
	}

	if op.Result == "" {
		op.ErrorMsg = fmt.Sprintf("result field missing or empty in output; stdout: %s", stdout)
		log.Printf("[DEBUG] OutputOp: result empty after parse")
		return nil
	}

	log.Printf("[DEBUG] OutputOp: result=%q ai_nodes=%s", op.Result, op.AINodes)
	return nil
}

// FallbackOp handles a single retry of code generation and compilation.
// If the initial compile succeeded it is a no-op (passes the binary through).
// If the initial compile failed it calls Gemini with the error + original code,
// writes the new files, and recompiles. A second compile failure is a hard error
// that fails the DAG.
type FallbackOp struct {
	Prompt             *string `dag:"input"`
	LibraryDescription *string `dag:"input"`
	CompileExitCode    *int    `dag:"input"` // from initial CompileOp
	CompileStderr      *string `dag:"input"` // from initial CompileOp
	GoFilesOriginal    *string `dag:"input"` // from initial GenerateOp
	InitialBinPath     *string `dag:"input"` // from initial CompileOp
	BinPath            string  `dag:"output"`
	Stderr             string  `dag:"output"` // forwarded to RunOp as CompileStderr

	dagAIModulePath string
}

func (op *FallbackOp) Setup(params *config.Params) error {
	op.dagAIModulePath = params.GetString("dag_ai_module_path", "")
	return nil
}
func (op *FallbackOp) Reset() error { return nil }
func (op *FallbackOp) Run(ctx context.Context) error {
	if *op.CompileExitCode == 0 {
		op.BinPath = *op.InitialBinPath
		log.Printf("[DEBUG] FallbackOp: initial compile succeeded, passthrough bin=%s", op.BinPath)
		return nil
	}
	log.Printf("[DEBUG] FallbackOp: initial compile failed, generating fallback code")

	// Extract the original generated code to include in the retry prompt.
	var origFiles map[string]string
	_ = json.Unmarshal([]byte(*op.GoFilesOriginal), &origFiles)
	originalCode := origFiles["main_go"]

	// Call Gemini with the same base prompt as GenerateOp, plus error context.
	apiKey := os.Getenv("GOOGLE_GENAI_API_KEY")
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("genai client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-flash-latest")
	model.ResponseMIMEType = "application/json"
	model.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"main_go": {Type: genai.TypeString},
		},
		Required: []string{"main_go"},
	}

	var sb strings.Builder
	sb.WriteString("You are a Go code generator for a DAG-based computation system.\n\n")
	sb.WriteString("FRAMEWORK: github.com/wwz16/dagor\n\n")
	sb.WriteString("GRAPH JSON SCHEMA:\n")
	sb.WriteString("  { \"name\": \"...\", \"vertices\": { \"<name>\": { \"op\": \"<Type>\", \"params\": {...},\n")
	sb.WriteString("    \"inputs\": {\"<Field>\": \"<wire>\"}, \"outputs\": {\"<Field>\": \"<wire>\"} } } }\n\n")
	sb.WriteString("REQUIRED IMPORTS — use exactly these, no others:\n")
	sb.WriteString("  \"context\", \"encoding/json\", \"fmt\", \"log\", \"time\"\n")
	sb.WriteString("  _ \"github.com/akennis/clawdag-go/library\"\n")
	sb.WriteString("  \"github.com/panjf2000/ants/v2\"\n")
	sb.WriteString("  \"github.com/wwz16/dagor\"\n")
	sb.WriteString("  \"github.com/wwz16/dagor/graph\"\n")
	sb.WriteString("  DO NOT import any other packages. Every import must be used.\n\n")
	sb.WriteString("HOW TO RUN A DAGOR GRAPH — use exactly this pattern:\n")
	sb.WriteString("  pool, _ := ants.NewPool(10); defer pool.Release()\n")
	sb.WriteString("  g, err := graph.NewGraphFromJson(json.RawMessage(dagJSON))\n")
	sb.WriteString("  eng, err := dagor.NewEngine(g, pool)\n")
	sb.WriteString("  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute); defer cancel()\n")
	sb.WriteString("  if err := eng.Run(ctx); err != nil { log.Fatal(err) }\n")
	sb.WriteString("  raw, ok := eng.GetOutput(\"wire_name\")  // returns (any, bool); cast result to *float64, *string, etc.\n\n")
	sb.WriteString("CRITICAL — DO NOT iterate over g.Vertices or inspect graph internals:\n")
	sb.WriteString("  WRONG: for _, v := range g.Vertices { ... }  // g.Vertices is a func, not a map — compile error\n")
	sb.WriteString("  WRONG: vertex.Inputs  // map[string]string, not an interface — type mismatch error\n")
	sb.WriteString("  RIGHT: use eng.GetOutput(\"wire_name\") for every value you need — always by wire name.\n\n")
	sb.WriteString("WIRE NAMING: wire names are arbitrary strings you assign in \"outputs\", then reference in \"inputs\".\n")
	sb.WriteString("  NOT \"vertex.Field\" syntax — just plain strings like \"val_a\", \"mul_result\".\n\n")
	sb.WriteString("GO SYNTAX RULE — TRAILING COMMAS: Go requires a trailing comma after the LAST element\n")
	sb.WriteString("  of every multi-line composite literal (map, slice, struct). Forgetting this is a compile error.\n")
	sb.WriteString("  WRONG:                             RIGHT:\n")
	sb.WriteString("    map[string]any{                    map[string]any{\n")
	sb.WriteString("      \"a\": 1,                           \"a\": 1,\n")
	sb.WriteString("      \"b\": 2   // ← missing comma       \"b\": 2,  // ← required\n")
	sb.WriteString("    }                                  }\n\n")
	sb.WriteString("STDOUT: result JSON MUST go to stdout via fmt.Println — NOT log.Printf (that goes to stderr).\n\n")
	sb.WriteString("AVAILABLE OPS (blank-import _ \"github.com/akennis/clawdag-go/library\" registers ALL of them — this import is REQUIRED):\n")
	sb.WriteString(*op.LibraryDescription)
	sb.WriteString("\n\n")
	sb.WriteString("KEY RULE: you may ONLY use ops listed above. There is NO MulOp, MultiplyOp, or any other op.\n")
	sb.WriteString("  For any operation not covered by the library (e.g. multiplication, power, modulo),\n")
	sb.WriteString("  use PackMathOperandsOp + AIComputeMathOperandsToFloat64Op.\n\n")
	sb.WriteString("EXAMPLE — solving 2 * 3 + 4 using AI for multiply and AddOp for add:\n")
	sb.WriteString("  Vertices:\n")
	sb.WriteString("    \"c2\":   {\"op\":\"ConstOp\", \"params\":{\"Value\":\"2\"}, \"outputs\":{\"Result\":\"v2\"}},\n")
	sb.WriteString("    \"c3\":   {\"op\":\"ConstOp\", \"params\":{\"Value\":\"3\"}, \"outputs\":{\"Result\":\"v3\"}},\n")
	sb.WriteString("    \"c4\":   {\"op\":\"ConstOp\", \"params\":{\"Value\":\"4\"}, \"outputs\":{\"Result\":\"v4\"}},\n")
	sb.WriteString("    \"pack\": {\"op\":\"PackMathOperandsOp\",\n")
	sb.WriteString("              \"inputs\":{\"A\":\"v2\",\"B\":\"v3\"}, \"outputs\":{\"Result\":\"operands\"}},\n")
	sb.WriteString("    \"mul\":  {\"op\":\"AIComputeMathOperandsToFloat64Op\", \"params\":{\"operation\":\"multiply A by B\", \"max_retries\":\"3\"},\n")
	sb.WriteString("              \"inputs\":{\"Input\":\"operands\"}, \"outputs\":{\"Result\":\"mul_res\",\"Reasoning\":\"mul_why\"}},\n")
	sb.WriteString("    \"add\":  {\"op\":\"AddOp\", \"inputs\":{\"A\":\"mul_res\",\"B\":\"v4\"}, \"outputs\":{\"Result\":\"final\"}}\n")
	sb.WriteString("  Read result:    raw, _ := eng.GetOutput(\"final\"); val := *(raw.(*float64))\n")
	sb.WriteString("  Read reasoning: rRaw, _ := eng.GetOutput(\"mul_why\"); reasoning := *(rRaw.(*string))\n")
	sb.WriteString("  Read AI result: arRaw, _ := eng.GetOutput(\"mul_res\"); aiResult := *(arRaw.(*float64))\n\n")
	sb.WriteString("TASK: ")
	sb.WriteString(*op.Prompt)
	sb.WriteString("\n\n")
	sb.WriteString("PREVIOUS ATTEMPT FAILED TO COMPILE.\n\n")
	sb.WriteString("Compile error:\n")
	sb.WriteString(*op.CompileStderr)
	sb.WriteString("\n\nPreviously generated code:\n```go\n")
	sb.WriteString(originalCode)
	sb.WriteString("\n```\n\nFix ALL errors above before responding.\n\n")
	sb.WriteString("OUTPUT REQUIREMENTS — generate main_go: a complete compilable package main that:\n")
	sb.WriteString("  * uses ONLY the required imports listed above (no extra packages)\n")
	sb.WriteString("  * blank-imports _ \"github.com/akennis/clawdag-go/library\" (REQUIRED — never omit this)\n")
	sb.WriteString("  * does NOT import \"github.com/wwz16/dagor/operator\" (not needed — library registers ops)\n")
	sb.WriteString("  * adds a trailing comma after the last element of EVERY multi-line composite literal\n")
	sb.WriteString("  * uses the exact dagor engine pattern above\n")
	sb.WriteString("  * prints to stdout: {\"result\":\"<answer as string>\",\"ai_nodes\":[{\"op\":\"AIComputeMathOperandsToFloat64Op\",\"inputs\":{...},\"output\":<num>,\"reasoning\":\"...\"}]}\n")
	sb.WriteString("  * result MUST be a JSON string (use fmt.Sprintf(\"%g\", val) to convert float64 to string)\n")
	sb.WriteString("  * ai_nodes contains one entry per AIComputeMathOperandsToFloat64Op vertex, with 'output' from its Result wire and 'reasoning' from its Reasoning wire\n")
	sb.WriteString("  * ai_nodes is [] if no AI op was used\n")
	sb.WriteString("  * calls log.Fatal on any error\n")

	resp, err := model.GenerateContent(ctx, genai.Text(sb.String()))
	if err != nil {
		return fmt.Errorf("fallback generate content: %w", err)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return fmt.Errorf("fallback: no candidates in response")
	}

	var raw string
	for _, part := range resp.Candidates[0].Content.Parts {
		raw += fmt.Sprintf("%v", part)
	}

	var files map[string]string
	if err := json.Unmarshal([]byte(raw), &files); err != nil {
		return fmt.Errorf("fallback: parse generated JSON: %w\nraw: %s", err, raw)
	}
	if files["main_go"] == "" {
		return fmt.Errorf("fallback: generated JSON missing main_go\nraw: %s", raw)
	}
	log.Printf("[DEBUG] FallbackOp: received fallback main_go (%d bytes)", len(files["main_go"]))

	// Write fallback files to a separate directory so the initial solution is not clobbered.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("UserHomeDir: %w", err)
	}
	fallbackDir := filepath.Join(home, ".dag-ai", "solution_fallback")
	if err := os.RemoveAll(fallbackDir); err != nil {
		return fmt.Errorf("remove fallback dir: %w", err)
	}
	if err := os.MkdirAll(fallbackDir, 0755); err != nil {
		return fmt.Errorf("mkdir fallback dir: %w", err)
	}

	modPath := filepath.ToSlash(op.dagAIModulePath)
	goMod := fmt.Sprintf("module solution\n\ngo 1.24\n\nrequire github.com/akennis/clawdag-go v0.0.0\n\nreplace github.com/akennis/clawdag-go => %s\n", modPath)
	if err := os.WriteFile(filepath.Join(fallbackDir, "go.mod"), []byte(goMod), 0644); err != nil {
		return fmt.Errorf("write fallback go.mod: %w", err)
	}
	if err := os.WriteFile(filepath.Join(fallbackDir, "main.go"), []byte(files["main_go"]), 0644); err != nil {
		return fmt.Errorf("write fallback main.go: %w", err)
	}

	log.Printf("[DEBUG] FallbackOp: gofmt syntax check")
	fmtCmd := exec.CommandContext(ctx, "gofmt", "-e", filepath.Join(fallbackDir, "main.go"))
	if out, err := fmtCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("fallback syntax error in main.go:\n%s", out)
	}

	log.Printf("[DEBUG] FallbackOp: running go mod tidy")
	tidy := exec.CommandContext(ctx, "go", "mod", "tidy")
	tidy.Dir = fallbackDir
	tidy.Env = os.Environ()
	if out, err := tidy.CombinedOutput(); err != nil {
		return fmt.Errorf("fallback go mod tidy: %w\n%s", err, out)
	}

	// Compile the fallback. Unlike the initial CompileOp, failure here is a hard DAG error.
	binName := "solution_bin"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(fallbackDir, binName)
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, "./...")
	buildCmd.Dir = fallbackDir
	buildCmd.Env = os.Environ()
	var errBuf strings.Builder
	buildCmd.Stderr = &errBuf
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("fallback compile failed:\n%s", errBuf.String())
	}

	op.BinPath = binPath
	log.Printf("[DEBUG] FallbackOp: fallback compile OK, bin=%s", op.BinPath)
	return nil
}
