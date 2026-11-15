// Package model is the local Ollama chat client used by the investigator.
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Tool struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id,omitempty"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Index     int            `json:"index,omitempty"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content,omitempty"`
	Thinking  string     `json:"thinking,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
}

type ChatOptions struct {
	Temperature float64 `json:"temperature"`
	NumCtx      int     `json:"num_ctx"`
	NumPredict  int     `json:"num_predict"`
	Seed        int     `json:"seed"`
}

type chatRequest struct {
	Model    string      `json:"model"`
	Messages []Message   `json:"messages"`
	Tools    []Tool      `json:"tools,omitempty"`
	Stream   bool        `json:"stream"`
	Think    *bool       `json:"think,omitempty"`
	Options  ChatOptions `json:"options"`
}

type ChatResponse struct {
	Model           string  `json:"model"`
	Message         Message `json:"message"`
	Done            bool    `json:"done"`
	DoneReason      string  `json:"done_reason,omitempty"`
	PromptEvalCount int     `json:"prompt_eval_count"`
	EvalCount       int     `json:"eval_count"`
}

type Config struct {
	BaseURL     string
	Model       string
	Temperature float64
	NumCtx      int
	NumPredict  int
	Think       *bool
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	if cfg.Model == "" {
		cfg.Model = "qwen3.5:4b"
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: 5 * time.Minute}}
}

func (c *Client) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	request := chatRequest{
		Model:    c.cfg.Model,
		Messages: messages,
		Tools:    tools,
		Stream:   false,
		Think:    c.cfg.Think,
		Options: ChatOptions{
			Temperature: c.cfg.Temperature,
			NumCtx:      c.cfg.NumCtx,
			NumPredict:  c.cfg.NumPredict,
			Seed:        0,
		},
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("ollama chat returned %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	var chatResponse ChatResponse
	if err := json.NewDecoder(response.Body).Decode(&chatResponse); err != nil {
		return nil, err
	}
	for index := range chatResponse.Message.ToolCalls {
		if chatResponse.Message.ToolCalls[index].ID == "" {
			chatResponse.Message.ToolCalls[index].ID = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), index)
		}
		if chatResponse.Message.ToolCalls[index].Function.Arguments == nil {
			chatResponse.Message.ToolCalls[index].Function.Arguments = map[string]any{}
		}
	}
	return &chatResponse, nil
}
