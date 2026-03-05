// Command c4-mcp is an MCP (Model Context Protocol) server that exposes
// c4 operations as tools for AI assistants like Claude Code.
//
// It communicates via JSON-RPC 2.0 over stdio using newline-delimited JSON.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// JSON-RPC 2.0 types

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any         `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP protocol types

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    capability `json:"capabilities"`
	ServerInfo      serverInfo `json:"serverInfo"`
}

type capability struct {
	Tools *struct{} `json:"tools,omitempty"`
}

type toolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

type toolsListResult struct {
	Tools []toolDef `json:"tools"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 10*1024*1024), 10*1024*1024) // 10MB max message

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("invalid JSON: %v", err)
			continue
		}

		// Notifications have no ID — don't respond
		if req.ID == nil {
			continue
		}

		resp := handle(req)
		out, _ := json.Marshal(resp)
		fmt.Fprintf(os.Stdout, "%s\n", out)
	}
}

func handle(req request) response {
	switch req.Method {
	case "initialize":
		return respond(req.ID, initResult{
			ProtocolVersion: "2024-11-05",
			Capabilities:    capability{Tools: &struct{}{}},
			ServerInfo:      serverInfo{Name: "c4-mcp", Version: "1.0.0"},
		})

	case "tools/list":
		return respond(req.ID, toolsListResult{Tools: allTools()})

	case "tools/call":
		var p toolCallParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return errResp(req.ID, -32602, "invalid params: "+err.Error())
		}
		return respond(req.ID, callTool(p.Name, p.Arguments))

	case "ping":
		return respond(req.ID, struct{}{})

	default:
		return errResp(req.ID, -32601, "method not found: "+req.Method)
	}
}

func respond(id json.RawMessage, result any) response {
	return response{JSONRPC: "2.0", ID: id, Result: result}
}

func errResp(id json.RawMessage, code int, msg string) response {
	return response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}
