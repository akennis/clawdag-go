package library

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpSession owns a connected MCP client and (when started via
// startMCPSession) the subprocess that backs it.
type mcpSession struct {
	client  *mcp.Client
	session *mcp.ClientSession
	cmd     *exec.Cmd
}

// startMCPSession spawns command(args...) and connects an MCP client over the
// child's stdin/stdout. Pass initTimeout=0 for no handshake timeout.
func startMCPSession(ctx context.Context, command string, args, env []string, initTimeout time.Duration) (*mcpSession, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	transport := &mcp.CommandTransport{Command: cmd}
	client := mcp.NewClient(&mcp.Implementation{Name: "clawdag-go", Version: "0.0.0"}, nil)

	connectCtx := ctx
	if initTimeout > 0 {
		var cancel context.CancelFunc
		connectCtx, cancel = context.WithTimeout(ctx, initTimeout)
		defer cancel()
	}
	sess, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", command, err)
	}
	return &mcpSession{client: client, session: sess, cmd: cmd}, nil
}

// mcpCallOutcome carries a tool-call result after content has been split into
// text vs. structured forms.
type mcpCallOutcome struct {
	text        string
	structured  json.RawMessage // nil if the tool emitted none
	isToolError bool
}

// callTool invokes the named tool with the given arguments. Returns a
// transport error on protocol failure; tool-level errors surface via
// mcpCallOutcome.isToolError.
func (s *mcpSession) callTool(ctx context.Context, name string, args any, callTimeout time.Duration) (mcpCallOutcome, error) {
	callCtx := ctx
	if callTimeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, callTimeout)
		defer cancel()
	}
	res, err := s.session.CallTool(callCtx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return mcpCallOutcome{}, err
	}

	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(tc.Text)
		}
		// Non-text content (Image/Audio/EmbeddedResource/ResourceLink) is
		// ignored in v1; consumers that need it must implement
		// MCPResponseParser and read StructuredContent.
	}

	out := mcpCallOutcome{
		text:        sb.String(),
		isToolError: res.IsError,
	}
	if res.StructuredContent != nil {
		b, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return mcpCallOutcome{}, fmt.Errorf("marshal structured content: %w", err)
		}
		out.structured = b
	}
	return out, nil
}

// close shuts down the client session and (via the transport) the subprocess.
// Safe to call on a zero-value or partially-initialized session.
func (s *mcpSession) close() error {
	if s == nil || s.session == nil {
		return nil
	}
	return s.session.Close()
}
