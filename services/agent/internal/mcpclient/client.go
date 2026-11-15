// Package mcpclient starts the five read-only MCP servers and exposes their
// tools to the investigator.
package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tchoupi663/weave/services/agent/internal/model"
)

// Server names in the order they are started.
var ServerNames = []string{"kube", "metrics", "logs", "source", "weave"}

type Tool struct {
	PrefixedName string
	Server       string
	ServerTool   string
	Description  string
	Schema       json.RawMessage
}

type Session struct {
	Name   string
	client *mcp.ClientSession
}

type Manager struct {
	binary    string
	env       []string
	delegates map[string]*Session
	Tools     []Tool
}

// Start launches every MCP server as a child process of binary (the
// weave-agent executable itself) over stdio and lists its tools.
func Start(ctx context.Context, binary string, env []string) (*Manager, error) {
	manager := &Manager{binary: binary, env: env, delegates: map[string]*Session{}}
	for _, name := range ServerNames {
		command := exec.Command(binary, "mcp", name)
		command.Env = append(os.Environ(), env...)
		client := mcp.NewClient(&mcp.Implementation{Name: "weave-agent", Version: "0.1.0"}, nil)
		session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
		if err != nil {
			manager.Close()
			return nil, fmt.Errorf("start %s MCP server: %w", name, err)
		}
		manager.delegates[name] = &Session{Name: name, client: session}
		result, err := session.ListTools(ctx, nil)
		if err != nil {
			manager.Close()
			return nil, fmt.Errorf("list %s tools: %w", name, err)
		}
		for _, tool := range result.Tools {
			schema, err := json.Marshal(tool.InputSchema)
			if err != nil {
				continue
			}
			manager.Tools = append(manager.Tools, Tool{
				PrefixedName: name + "__" + tool.Name,
				Server:       name,
				ServerTool:   tool.Name,
				Description:  tool.Description,
				Schema:       normalizeSchema(schema),
			})
		}
	}
	return manager, nil
}

func (manager *Manager) Close() {
	for _, session := range manager.delegates {
		_ = session.client.Close()
	}
	manager.delegates = map[string]*Session{}
}

// normalizeSchema makes tool input schemas acceptable to Ollama, which
// requires every object schema to carry a properties object.
func normalizeSchema(raw json.RawMessage) json.RawMessage {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	if schema["type"] == nil {
		schema["type"] = "object"
	}
	if schema["type"] == "object" {
		if _, ok := schema["properties"]; !ok {
			schema["properties"] = map[string]any{}
		}
	}
	delete(schema, "$schema")
	normalized, err := json.Marshal(schema)
	if err != nil {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return normalized
}

// ModelTools converts the MCP tools to Ollama tool definitions.
func (manager *Manager) ModelTools() []model.Tool {
	result := make([]model.Tool, 0, len(manager.Tools))
	for _, tool := range manager.Tools {
		result = append(result, model.Tool{
			Type: "function",
			Function: model.FunctionDef{
				Name:        tool.PrefixedName,
				Description: tool.Description,
				Parameters:  tool.Schema,
			},
		})
	}
	return result
}

type CallResult struct {
	Text    string
	IsError bool
	Denied  bool
}

// Call invokes a prefixed tool and flattens the result into text for the model.
func (manager *Manager) Call(ctx context.Context, prefixedName string, arguments map[string]any) (CallResult, error) {
	var selected *Tool
	for index := range manager.Tools {
		if manager.Tools[index].PrefixedName == prefixedName {
			selected = &manager.Tools[index]
			break
		}
	}
	if selected == nil {
		return CallResult{}, fmt.Errorf("unknown tool %q", prefixedName)
	}
	delegate := manager.delegates[selected.Server]
	if delegate == nil {
		return CallResult{}, fmt.Errorf("MCP server %q is not running", selected.Server)
	}
	result, err := delegate.client.CallTool(ctx, &mcp.CallToolParams{Name: selected.ServerTool, Arguments: arguments})
	if err != nil {
		return CallResult{}, err
	}
	call := CallResult{IsError: result.IsError}
	if result.StructuredContent != nil {
		raw, marshalErr := json.Marshal(result.StructuredContent)
		if marshalErr == nil {
			call.Text = string(raw)
		}
	}
	if call.Text == "" {
		parts := make([]string, 0, len(result.Content))
		for _, content := range result.Content {
			if text, ok := content.(*mcp.TextContent); ok {
				parts = append(parts, text.Text)
			}
		}
		call.Text = strings.Join(parts, "\n")
	}
	if result.IsError {
		if strings.Contains(strings.ToLower(call.Text), "scope_denied") {
			call.Denied = true
		}
	}
	return call, nil
}
