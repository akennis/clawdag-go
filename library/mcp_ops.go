package library

import "github.com/wwz16/dagor/operator"

// FilesystemReadArgs is the input shape for the read-file tool exposed by
// @modelcontextprotocol/server-filesystem.
type FilesystemReadArgs struct {
	Path string `json:"path"`
}

const MCPFilesystemReadFileOpDescription = `MCPFilesystemReadFileOp: read a UTF-8 file via the @modelcontextprotocol/server-filesystem MCP server.
  Params:   command         — server executable (typically "npx" or "uvx").
            args            — comma-separated CLI args (e.g. "-y,@modelcontextprotocol/server-filesystem,/abs/root").
            env             — comma-separated KEY=VALUE additions to subprocess env (optional).
            tool_name       — MCP tool name to invoke (e.g. "read_text_file" or "read_file"; varies by server build).
            init_timeout_ms — handshake timeout in ms (default "10000").
            call_timeout_ms — call timeout in ms (default "30000").
            max_retries     — transient-error retries (default "3").
  Inputs:   Input *FilesystemReadArgs (Path string — must be inside an allowed root configured on the server).
  Outputs:  Result string — the file contents.`

// MCPFilesystemReadFileOp wraps the filesystem MCP server's "read file" tool.
// The exact tool name is configurable via the "tool_name" param so users can
// target read_text_file, read_file, or whichever variant their server build
// exposes.
type MCPFilesystemReadFileOp struct {
	MCPCallOp[FilesystemReadArgs, string]
}

func init() {
	operator.RegisterOp[MCPFilesystemReadFileOp]()
}
