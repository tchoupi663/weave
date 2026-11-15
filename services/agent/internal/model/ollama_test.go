package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatParsesToolCallsAndUsage(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"qwen3.5:4b",
			"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"ping","arguments":{"value":42}}}]},
			"done":true,
			"prompt_eval_count":123,
			"eval_count":45
		}`))
	}))
	defer server.Close()
	client := New(Config{BaseURL: server.URL, Model: "qwen3.5:4b", NumCtx: 32768, NumPredict: 2000})
	response, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, []Tool{{
		Type: "function", Function: FunctionDef{Name: "ping", Description: "ping"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if response.PromptEvalCount != 123 || response.EvalCount != 45 {
		t.Fatalf("usage = %+v", response)
	}
	if len(response.Message.ToolCalls) != 1 || response.Message.ToolCalls[0].Function.Name != "ping" {
		t.Fatalf("tool calls = %+v", response.Message.ToolCalls)
	}
	if response.Message.ToolCalls[0].ID == "" {
		t.Fatal("tool call id not generated")
	}
	if captured["model"] != "qwen3.5:4b" || captured["stream"] != false {
		t.Fatalf("request = %+v", captured)
	}
	if _, ok := captured["options"].(map[string]any)["num_ctx"]; !ok {
		t.Fatalf("options missing: %+v", captured["options"])
	}
}

func TestChatErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer server.Close()
	client := New(Config{BaseURL: server.URL})
	if _, err := client.Chat(context.Background(), nil, nil); err == nil {
		t.Fatal("expected error")
	}
}
