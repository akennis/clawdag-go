package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/wwz16/dagor"
	"github.com/wwz16/dagor/graph"
	"github.com/wwz16/dagor/operator"
)

// SolutionOutput is the JSON structure written to stdout by the solution binary.
type SolutionOutput struct {
	Result  string       `json:"result"`
	AINodes []AINodeDiag `json:"ai_nodes"`
}

// AINodeDiag contains diagnostics for a single AI-powered node.
type AINodeDiag struct {
	Op        string         `json:"op"`
	Inputs    map[string]any `json:"inputs"`
	Output    any            `json:"output"`
	Reasoning string         `json:"reasoning"`
}

// buildDriverDAG constructs the driver DAG JSON.
//
// Data flow:
//
//	PromptOp ─────────────────────────────────────────────────────────────────────┐
//	                                                                               ▼
//	LibraryScanOp ──────────────────────────────────────────────────────► GenerateOp
//	                                                                               │
//	                                                                       WriteFilesOp
//	                                                                               │
//	                                                             CompileOp (on_error: continue)
//	                                                                               │
//	                                                             FallbackOp ◄──────┘
//	                                                             (no-op if compile OK;
//	                                                              else re-generates + recompiles;
//	                                                              hard DAG error if both fail)
//	                                                                               │
//	                                                                             RunOp
//	                                                                               │
//	                                                                           OutputOp
func buildDriverDAG(dagAIModulePath string) string {
	modPathJSON, _ := json.Marshal(dagAIModulePath)

	return fmt.Sprintf(`{
		"name": "driver_dag",
		"vertices": {
			"prompt": {
				"op": "PromptOp",
				"outputs": { "Prompt": "user_prompt" }
			},
			"libscan": {
				"op": "LibraryScanOp",
				"outputs": { "LibraryDescription": "lib_desc" }
			},
			"generate": {
				"op": "GenerateOp",
				"inputs": { "Prompt": "user_prompt", "LibraryDescription": "lib_desc" },
				"outputs": { "GoFiles": "go_files" }
			},
			"write": {
				"op": "WriteFilesOp",
				"params": { "dag_ai_module_path": %s },
				"inputs": { "GoFiles": "go_files" },
				"outputs": { "TempDir": "temp_dir" }
			},
			"compile": {
				"op": "CompileOp",
				"on_error": "continue",
				"inputs": { "TempDir": "temp_dir" },
				"outputs": { "BinPath": "bin_path", "ExitCode": "compile_exit", "Stderr": "compile_stderr" }
			},
			"fallback": {
				"op": "FallbackOp",
				"params": { "dag_ai_module_path": %s },
				"inputs": {
					"Prompt": "user_prompt",
					"LibraryDescription": "lib_desc",
					"CompileExitCode": "compile_exit",
					"CompileStderr": "compile_stderr",
					"GoFilesOriginal": "go_files",
					"InitialBinPath": "bin_path"
				},
				"outputs": { "BinPath": "final_bin_path", "Stderr": "final_compile_stderr" }
			},
			"run": {
				"op": "RunOp",
				"on_error": "continue",
				"inputs": { "BinPath": "final_bin_path", "CompileStderr": "final_compile_stderr" },
				"outputs": { "Stdout": "run_stdout", "Stderr": "run_stderr", "ExitCode": "run_exit" }
			},
			"output": {
				"op": "OutputOp",
				"inputs": { "RawStdout": "run_stdout", "RawStderr": "run_stderr", "ExitCode": "run_exit" },
				"outputs": { "Result": "final_result", "AINodes": "final_ai_nodes", "ErrorMsg": "run_error" }
			}
		}
	}`, modPathJSON, modPathJSON)
}

func printResults(result string, aiNodesRaw any) {
	fmt.Println("\n--- Result ---")
	fmt.Println(result)

	// aiNodesRaw is *string containing JSON array
	nodesPtr, ok := aiNodesRaw.(*string)
	if !ok || nodesPtr == nil || *nodesPtr == "" || *nodesPtr == "null" {
		return
	}

	var nodes []AINodeDiag
	if err := json.Unmarshal([]byte(*nodesPtr), &nodes); err != nil || len(nodes) == 0 {
		return
	}

	fmt.Println("\n--- AI-Powered Node Diagnostics ---")
	for _, n := range nodes {
		fmt.Println(n.Op)
		var inputParts []string
		for k, v := range n.Inputs {
			inputParts = append(inputParts, fmt.Sprintf("%s=%v", k, v))
		}
		fmt.Printf("  Inputs:    %s\n", strings.Join(inputParts, ", "))
		fmt.Printf("  Output:    %v\n", n.Output)
		fmt.Printf("  Reasoning: %s\n", n.Reasoning)
	}
}

func registerDriverOps() {
	operator.RegisterOp[PromptOp]()
	operator.RegisterOp[LibraryScanOp]()
	operator.RegisterOp[GenerateOp]()
	operator.RegisterOp[WriteFilesOp]()
	operator.RegisterOp[CodegenOp]()
	operator.RegisterOp[CompileOp]()
	operator.RegisterOp[FallbackOp]()
	operator.RegisterOp[RunOp]()
	operator.RegisterOp[OutputOp]()
}

func main() {
	registerDriverOps()

	// During `go run .`, use the source dir as the dag-ai module path for the replace directive.
	modulePath, err := filepath.Abs(".")
	if err != nil {
		log.Fatalf("filepath.Abs: %v", err)
	}

	pool, err := ants.NewPool(10)
	if err != nil {
		log.Fatalf("ants.NewPool: %v", err)
	}
	defer pool.Release()

	dagJSON := buildDriverDAG(modulePath)
	g, err := graph.NewGraphFromJson(json.RawMessage(dagJSON))
	if err != nil {
		log.Fatalf("NewGraphFromJson: %v", err)
	}
	eng, err := dagor.NewEngine(g, pool)
	if err != nil {
		log.Fatalf("NewEngine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	runErr := eng.Run(ctx)

	resultRaw, _ := eng.GetOutput("final_result")
	errMsgRaw, _ := eng.GetOutput("run_error")
	aiNodesRaw, _ := eng.GetOutput("final_ai_nodes")

	result := ""
	if resultRaw != nil {
		result = *(resultRaw.(*string))
	}
	errMsg := ""
	if errMsgRaw != nil {
		errMsg = *(errMsgRaw.(*string))
	}

	cancel()
	eng.Close(ctx)

	if errMsg == "" && result != "" {
		printResults(result, aiNodesRaw)
		return
	}

	// OutputOp never ran (upstream stage failed before it could set errMsg).
	if errMsg == "" {
		if runErr != nil {
			errMsg = fmt.Sprintf("pipeline error: %v", runErr)
		} else {
			errMsg = "pipeline produced no output (check debug logs)"
		}
	}

	fmt.Fprintf(os.Stderr, "Failed: %s\n", errMsg)
	os.Exit(1)
}
