package investigation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tchoupi663/weave/services/agent/internal/mcpclient"
	"github.com/tchoupi663/weave/services/agent/internal/model"
)

type fakeModel struct {
	responses []*model.ChatResponse
}

func (f *fakeModel) Chat(context.Context, []model.Message, []model.Tool) (*model.ChatResponse, error) {
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

type fakeTools struct {
	calls []string
}

func (f *fakeTools) ModelTools() []model.Tool {
	return []model.Tool{{Type: "function", Function: model.FunctionDef{Name: "weave__get_incident"}}}
}

func (f *fakeTools) Call(_ context.Context, name string, _ map[string]any) (mcpclient.CallResult, error) {
	f.calls = append(f.calls, name)
	return mcpclient.CallResult{Text: `{"id":"incident-1"}`}, nil
}

func diagnosisJSON() string {
	return `{"root_cause":{"workload":"catalog","failure_mode":"http_error"},"affected":{"route":"GET /api/products"},"evidence":[{"source":"metrics","reference":"x","observation":"5xx"}],"confidence":0.9,"summary":"Catalog returns 500."}`
}

func TestRunCompletesWithToolCallAndDiagnosis(t *testing.T) {
	modelClient := &fakeModel{responses: []*model.ChatResponse{
		{PromptEvalCount: 100, EvalCount: 10, Message: model.Message{ToolCalls: []model.ToolCall{{
			ID: "call-1", Function: model.FunctionCall{Name: "weave__get_incident", Arguments: map[string]any{"incident_id": "incident-1", "workload": "catalog"}},
		}}}},
		{PromptEvalCount: 200, EvalCount: 20, Message: model.Message{Content: diagnosisJSON()}},
	}}
	tools := &fakeTools{}
	record, err := Run(context.Background(), RunOptions{
		Incident: Incident{ID: "incident-1", WindowStart: time.Now().Add(-time.Minute), WindowEnd: time.Now()},
		Arm:      ArmUnconfined,
		Model:    modelClient,
		Tools:    tools,
		Caps:     Caps{MaxToolCalls: 5, MaxTokens: 10000, MaxWall: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != statusCompleted || record.Diagnosis == nil || record.Diagnosis.RootCause.Workload != "catalog" {
		t.Fatalf("record = %+v", record)
	}
	if record.Tokens.Total != 330 || record.ToolCalls != 1 || len(tools.calls) != 1 {
		t.Fatalf("record = %+v calls=%v", record, tools.calls)
	}
	if record.SearchBreadth["workloads"] != 1 {
		t.Fatalf("search breadth = %+v", record.SearchBreadth)
	}
}

func TestRunFailsMalformedThenRepairs(t *testing.T) {
	modelClient := &fakeModel{responses: []*model.ChatResponse{
		{PromptEvalCount: 10, Message: model.Message{Content: "not json"}},
		{PromptEvalCount: 10, EvalCount: 5, Message: model.Message{Content: diagnosisJSON()}},
	}}
	record, err := Run(context.Background(), RunOptions{
		Incident: Incident{ID: "incident-1"}, Arm: ArmUnconfined,
		Model: modelClient, Tools: &fakeTools{},
		Caps: Caps{MaxToolCalls: 5, MaxTokens: 10000, MaxWall: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != statusCompleted || record.Diagnosis == nil {
		t.Fatalf("record = %+v", record)
	}
}

func TestRunToolCap(t *testing.T) {
	modelClient := &fakeModel{responses: []*model.ChatResponse{
		{PromptEvalCount: 10, Message: model.Message{ToolCalls: []model.ToolCall{
			{Function: model.FunctionCall{Name: "weave__get_incident", Arguments: map[string]any{}}},
			{Function: model.FunctionCall{Name: "weave__get_topology", Arguments: map[string]any{}}},
		}}},
		{PromptEvalCount: 10, Message: model.Message{Content: diagnosisJSON()}},
	}}
	record, err := Run(context.Background(), RunOptions{
		Incident: Incident{ID: "incident-1"}, Arm: ArmUnconfined,
		Model: modelClient, Tools: &fakeTools{},
		Caps: Caps{MaxToolCalls: 1, MaxTokens: 10000, MaxWall: time.Minute},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != statusCompleted || record.Diagnosis == nil {
		t.Fatalf("forced final answer = %+v", record)
	}
	if record.ToolCalls != 1 {
		t.Fatalf("tool calls = %d", record.ToolCalls)
	}
}

func TestParseDiagnosisValidation(t *testing.T) {
	if _, err := parseDiagnosis("no json"); err == nil {
		t.Fatal("expected no-json error")
	}
	invalidMode := `{"root_cause":{"workload":"catalog","failure_mode":"nope"},"confidence":0.5,"summary":"x"}`
	if _, err := parseDiagnosis(invalidMode); err == nil {
		t.Fatal("expected failure mode error")
	}
	if _, err := parseDiagnosis("```json\n" + diagnosisJSON() + "\n```"); err != nil {
		t.Fatal(err)
	}
	var _ = json.Marshal
}
