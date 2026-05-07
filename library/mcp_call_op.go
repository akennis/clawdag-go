package library

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/wwz16/dagor"
	"github.com/wwz16/dagor/config"
)

// MCPArgsFormatter is an optional interface for In types to control how they
// are marshaled into the MCP tool's "arguments" object. If unimplemented, the
// dereferenced *Input value is passed to the SDK and JSON-marshaled as-is.
type MCPArgsFormatter interface {
	FormatMCPArgs() (any, error)
}

// MCPResponseParser is an optional interface for Out types to fully control
// parsing of the MCP tool result. If implemented, MCPCallOp hands it the
// concatenated text content and the raw structured-content JSON (nil if the
// tool didn't emit any) and skips the built-in dispatch.
type MCPResponseParser interface {
	ParseMCPResponse(text string, structured json.RawMessage) error
}

// MCPCallOp is a generic operator that wraps a single MCP server tool as a
// dagor node. Each Run spawns a fresh subprocess, completes the MCP handshake,
// invokes the tool, and tears down the subprocess.
//
// Vertex params:
//
//	command         — server executable (e.g. "npx", "uvx", "/abs/path"). Required.
//	args            — comma-separated CLI args passed to the server. Optional.
//	                  Note: values containing commas are not supported.
//	env             — comma-separated KEY=VALUE pairs added to the subprocess env. Optional.
//	                  Note: values containing commas are not supported.
//	tool_name       — MCP tool to invoke. Required.
//	init_timeout_ms — handshake timeout in ms (default "10000").
//	call_timeout_ms — single tool call timeout in ms (default "30000").
//	max_retries     — transient-error retries (default "3").
//
// Do not register MCPCallOp directly — declare a concrete variant such as
// MCPFilesystemReadFileOp{ MCPCallOp[FilesystemReadArgs, string] }.
type MCPCallOp[In, Out any] struct {
	Input  *In
	Result Out

	command     string
	args        []string
	env         []string
	toolName    string
	initTimeout time.Duration
	callTimeout time.Duration
	maxRetries  int
}

func (op *MCPCallOp[In, Out]) Setup(params *config.Params) error {
	op.command = params.GetString("command", "")
	if op.command == "" {
		return fmt.Errorf("MCPCallOp: 'command' param is required")
	}
	if _, err := exec.LookPath(op.command); err != nil {
		return fmt.Errorf("MCPCallOp: command %q not found on PATH: %w", op.command, err)
	}
	op.args = mcpSplitCSV(params.GetString("args", ""))
	op.env = nil
	for _, kv := range mcpSplitCSV(params.GetString("env", "")) {
		if strings.Contains(kv, "=") {
			op.env = append(op.env, kv)
		}
	}
	op.toolName = params.GetString("tool_name", "")
	if op.toolName == "" {
		return fmt.Errorf("MCPCallOp: 'tool_name' param is required")
	}
	op.initTimeout = mcpParseDurationMs(params, "init_timeout_ms", 10000)
	op.callTimeout = mcpParseDurationMs(params, "call_timeout_ms", 30000)
	op.maxRetries = 3
	if s := params.GetString("max_retries", ""); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			op.maxRetries = n
		}
	}
	return nil
}

func (op *MCPCallOp[In, Out]) Reset() error { return nil }

func (op *MCPCallOp[In, Out]) InputFields() map[string]any {
	return map[string]any{"Input": &op.Input}
}

func (op *MCPCallOp[In, Out]) OutputFields() map[string]any {
	return map[string]any{"Result": &op.Result}
}

func (op *MCPCallOp[In, Out]) SetInputField(field string, value any) error {
	switch field {
	case "Input":
		val, ok := value.(*In)
		if !ok {
			return fmt.Errorf("field Input: expected *%T, got %T", op.Input, value)
		}
		op.Input = val
	default:
		return fmt.Errorf("field %s is not defined", field)
	}
	return nil
}

func (op *MCPCallOp[In, Out]) ResetFields() {
	var zeroIn *In
	op.Input = zeroIn
	var zeroOut Out
	op.Result = zeroOut
}

func (op *MCPCallOp[In, Out]) Run(ctx context.Context) error {
	slog.DebugContext(ctx, "MCPCallOp.run", "run_id", dagor.RunID(ctx), "command", op.command, "tool", op.toolName)

	args, err := op.encodeArgs()
	if err != nil {
		return err
	}

	delay := 500 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt <= op.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay = min(delay*2, 30*time.Second)
		}

		outcome, callErr := op.runOnce(ctx, args)
		if callErr != nil {
			lastErr = callErr
			slog.WarnContext(ctx, "MCPCallOp.attempt_failed", "attempt", attempt+1, "of", op.maxRetries, "err", callErr)
			continue
		}
		if outcome.isToolError {
			return fmt.Errorf("MCPCallOp: tool %q reported error: %s", op.toolName, strings.TrimSpace(outcome.text))
		}
		return op.decodeResult(outcome)
	}
	return fmt.Errorf("MCPCallOp: all %d attempts failed; last error: %w", op.maxRetries+1, lastErr)
}

func (op *MCPCallOp[In, Out]) runOnce(ctx context.Context, args any) (mcpCallOutcome, error) {
	sess, err := startMCPSession(ctx, op.command, op.args, op.env, op.initTimeout)
	if err != nil {
		return mcpCallOutcome{}, err
	}
	defer func() {
		if cerr := sess.close(); cerr != nil {
			slog.WarnContext(ctx, "MCPCallOp.close_warn", "err", cerr)
		}
	}()
	return sess.callTool(ctx, op.toolName, args, op.callTimeout)
}

func (op *MCPCallOp[In, Out]) encodeArgs() (any, error) {
	if op.Input == nil {
		return map[string]any{}, nil
	}
	if f, ok := any(op.Input).(MCPArgsFormatter); ok {
		return f.FormatMCPArgs()
	}
	return *op.Input, nil
}

func (op *MCPCallOp[In, Out]) decodeResult(outcome mcpCallOutcome) error {
	if p, ok := any(&op.Result).(MCPResponseParser); ok {
		return p.ParseMCPResponse(outcome.text, outcome.structured)
	}
	if len(outcome.structured) > 0 {
		if err := json.Unmarshal(outcome.structured, &op.Result); err == nil {
			return nil
		}
	}
	return op.parseResultText(outcome.text)
}

func (op *MCPCallOp[In, Out]) parseResultText(raw string) error {
	raw = strings.TrimSpace(raw)
	switch v := any(&op.Result).(type) {
	case *string:
		*v = raw
	case *float64:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return fmt.Errorf("expected float64, got %q: %w", raw, err)
		}
		*v = f
	case *int:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("expected int, got %q: %w", raw, err)
		}
		*v = n
	case *bool:
		switch strings.ToLower(raw) {
		case "true", "yes":
			*v = true
		case "false", "no":
			*v = false
		default:
			return fmt.Errorf("expected bool (true/false), got %q", raw)
		}
	case *[]float64:
		if raw == "" {
			*v = nil
			return nil
		}
		parts := strings.Split(raw, ",")
		s := make([]float64, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			f, err := strconv.ParseFloat(p, 64)
			if err != nil {
				return fmt.Errorf("expected []float64 CSV, got %q: %w", raw, err)
			}
			s = append(s, f)
		}
		*v = s
	case *[]int:
		if raw == "" {
			*v = nil
			return nil
		}
		parts := strings.Split(raw, ",")
		s := make([]int, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			n, err := strconv.Atoi(p)
			if err != nil {
				return fmt.Errorf("expected []int CSV, got %q: %w", raw, err)
			}
			s = append(s, n)
		}
		*v = s
	case *[]string:
		if raw == "" {
			*v = nil
			return nil
		}
		parts := strings.Split(raw, ",")
		s := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				s = append(s, p)
			}
		}
		*v = s
	case *map[string]string:
		if raw == "" {
			*v = map[string]string{}
			return nil
		}
		m := make(map[string]string)
		for _, pair := range strings.Split(raw, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			idx := strings.IndexByte(pair, '=')
			if idx < 0 {
				return fmt.Errorf("expected key=value pair, got %q", pair)
			}
			m[strings.TrimSpace(pair[:idx])] = strings.TrimSpace(pair[idx+1:])
		}
		*v = m
	default:
		if err := json.Unmarshal([]byte(raw), &op.Result); err != nil {
			return fmt.Errorf("MCPCallOp: unsupported output type %T (got %q): implement MCPResponseParser", op.Result, raw)
		}
	}
	return nil
}

func mcpSplitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mcpParseDurationMs(p *config.Params, key string, defaultMs int64) time.Duration {
	s := p.GetString(key, "")
	if s == "" {
		return time.Duration(defaultMs) * time.Millisecond
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return time.Duration(defaultMs) * time.Millisecond
	}
	return time.Duration(n) * time.Millisecond
}
