// weave-tool is a thin CLI over the read-only MCP servers, used by external
// agents that only have shell access. It enforces the same scope environment
// as the investigator's MCP children.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: weave-tool <list|call> <server> [tool] [json-arguments]")
		os.Exit(2)
	}
	mode, server := os.Args[1], os.Args[2]
	agentBinary := strings.TrimSpace(os.Getenv("WEAVE_AGENT_BIN"))
	if agentBinary == "" {
		agentBinary = ".cache/weave-agent"
	}
	command := exec.Command(agentBinary, "mcp", server)
	command.Env = os.Environ()
	client := mcp.NewClient(&mcp.Implementation{Name: "weave-tool", Version: "0.1.0"}, nil)
	ctx := context.Background()
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		os.Exit(1)
	}
	defer session.Close()

	switch mode {
	case "list":
		result, err := session.ListTools(ctx, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		type tool struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Schema      any    `json:"input_schema"`
		}
		tools := make([]tool, 0, len(result.Tools))
		for _, item := range result.Tools {
			tools = append(tools, tool{Name: item.Name, Description: item.Description, Schema: item.InputSchema})
		}
		raw, _ := json.MarshalIndent(map[string]any{"server": server, "tools": tools}, "", "  ")
		fmt.Println(string(raw))
	case "call":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: weave-tool call <server> <tool> [json-arguments]")
			os.Exit(2)
		}
		toolName := os.Args[3]
		arguments := map[string]any{}
		if len(os.Args) > 4 && strings.TrimSpace(os.Args[4]) != "" {
			if err := json.Unmarshal([]byte(os.Args[4]), &arguments); err != nil {
				fmt.Fprintln(os.Stderr, "arguments must be a JSON object:", err)
				os.Exit(2)
			}
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: arguments})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if result.StructuredContent != nil {
			raw, _ := json.MarshalIndent(result.StructuredContent, "", "  ")
			fmt.Println(string(raw))
		} else {
			for _, content := range result.Content {
				if text, ok := content.(*mcp.TextContent); ok {
					fmt.Println(text.Text)
				}
			}
		}
		if result.IsError {
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "mode must be list or call")
		os.Exit(2)
	}
}
