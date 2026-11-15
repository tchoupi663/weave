package investigation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tchoupi663/weave/services/agent/internal/mcpclient"
	"github.com/tchoupi663/weave/services/agent/internal/model"
)

const (
	ArmConfined   = "confined"
	ArmUnconfined = "unconfined"

	statusCompleted   = "completed"
	statusFailedCap   = "failed_cap"
	statusFailedParse = "failed_parse"
	statusFailedModel = "failed_model"
)

// ChatModel is the local model interface used by the loop.
type ChatModel interface {
	Chat(ctx context.Context, messages []model.Message, tools []model.Tool) (*model.ChatResponse, error)
}

// ToolManager is the MCP tool interface used by the loop.
type ToolManager interface {
	ModelTools() []model.Tool
	Call(ctx context.Context, name string, arguments map[string]any) (mcpclient.CallResult, error)
}

type Diagnosis struct {
	RootCause struct {
		Workload    string `json:"workload"`
		FailureMode string `json:"failure_mode"`
	} `json:"root_cause"`
	Affected struct {
		EdgeID string `json:"edge_id,omitempty"`
		Route  string `json:"route,omitempty"`
	} `json:"affected"`
	Evidence []struct {
		Source      string `json:"source"`
		Reference   string `json:"reference"`
		Observation string `json:"observation"`
	} `json:"evidence"`
	Confidence float64 `json:"confidence"`
	Summary    string  `json:"summary"`
}

type Caps struct {
	MaxToolCalls int           `json:"max_tool_calls"`
	MaxTokens    int           `json:"max_tokens"`
	MaxWall      time.Duration `json:"max_wall"`
}

type Usage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

type RunRecord struct {
	RunID            string         `json:"run_id"`
	IncidentID       string         `json:"incident_id"`
	Arm              string         `json:"arm"`
	PromptVersion    string         `json:"prompt_version"`
	ModelName        string         `json:"model_name"`
	ModelDigest      string         `json:"model_digest,omitempty"`
	Status           string         `json:"status"`
	StartedAt        time.Time      `json:"started_at"`
	EndedAt          time.Time      `json:"ended_at"`
	DurationMS       int64          `json:"duration_ms"`
	Tokens           Usage          `json:"tokens"`
	ToolCalls        int            `json:"tool_calls"`
	ToolPayloadBytes int            `json:"tool_payload_bytes"`
	SearchBreadth    map[string]int `json:"search_breadth"`
	Expansions       int            `json:"expansions"`
	Denials          int            `json:"denials"`
	Caps             Caps           `json:"caps"`
	Diagnosis        *Diagnosis     `json:"diagnosis,omitempty"`
	Error            string         `json:"error,omitempty"`
	Transcript       string         `json:"transcript,omitempty"`
}

type RunOptions struct {
	Incident      Incident
	RegionJSON    []byte
	Arm           string
	Model         ChatModel
	Tools         ToolManager
	ModelName     string
	ModelDigest   string
	PromptVersion string
	RunDir        string
	RunID         string
	Caps          Caps
}

// Run executes one investigation loop. Infrastructure errors are returned as
// an error; model/tool/parse failures are recorded in the returned record.
func Run(ctx context.Context, options RunOptions) (*RunRecord, error) {
	if options.Arm != ArmConfined && options.Arm != ArmUnconfined {
		return nil, fmt.Errorf("arm must be confined or unconfined")
	}
	if options.Model == nil || options.Tools == nil {
		return nil, fmt.Errorf("model and tools are required")
	}
	if options.Caps.MaxToolCalls <= 0 {
		options.Caps.MaxToolCalls = 20
	}
	if options.Caps.MaxTokens <= 0 {
		options.Caps.MaxTokens = 80000
	}
	if options.Caps.MaxWall <= 0 {
		options.Caps.MaxWall = 10 * time.Minute
	}
	runID := options.RunID
	if runID == "" {
		runID = time.Now().UTC().Format("20060102T150405Z") + "-" + options.Arm
	}
	record := &RunRecord{
		RunID:         runID,
		IncidentID:    options.Incident.ID,
		Arm:           options.Arm,
		PromptVersion: options.PromptVersion,
		ModelName:     options.ModelName,
		ModelDigest:   options.ModelDigest,
		StartedAt:     time.Now().UTC(),
		SearchBreadth: map[string]int{},
		Caps:          options.Caps,
	}
	if record.PromptVersion == "" {
		record.PromptVersion = PromptVersion
	}
	var transcript *transcriptWriter
	if options.RunDir != "" {
		if err := os.MkdirAll(options.RunDir, 0o755); err != nil {
			return nil, err
		}
		path := filepath.Join(options.RunDir, "transcript.jsonl")
		writer, err := newTranscriptWriter(path)
		if err != nil {
			return nil, err
		}
		transcript = writer
		defer transcript.Close()
		record.Transcript = path
	}
	writeEvent := func(kind string, data any) {
		if transcript != nil {
			_ = transcript.Write(kind, data)
		}
	}

	deadlineCtx, cancel := context.WithTimeout(ctx, options.Caps.MaxWall)
	defer cancel()

	messages := []model.Message{
		{Role: "system", Content: SystemPrompt(options.Arm)},
		{Role: "user", Content: UserPrompt(options.Incident, options.RegionJSON)},
	}
	tools := options.Tools.ModelTools()
	repairAttempts := 0
	var lastErr string
	record.Status = statusCompleted
	forceFinal := false
	for {
		if deadlineCtx.Err() != nil {
			record.Status = statusFailedCap
			record.Error = "wall time cap reached"
			break
		}
		response, err := options.Model.Chat(deadlineCtx, messages, tools)
		if err != nil {
			if errors.Is(deadlineCtx.Err(), context.DeadlineExceeded) {
				record.Status = statusFailedCap
				record.Error = "wall time cap reached while calling the model"
			} else {
				record.Status = statusFailedModel
				record.Error = err.Error()
			}
			writeEvent("error", map[string]any{"error": record.Error})
			break
		}
		record.Tokens.Prompt += response.PromptEvalCount
		record.Tokens.Completion += response.EvalCount
		record.Tokens.Total = record.Tokens.Prompt + record.Tokens.Completion
		if record.Tokens.Total > options.Caps.MaxTokens {
			record.Status = statusFailedCap
			record.Error = "token cap reached"
			writeEvent("cap", map[string]any{"tokens": record.Tokens, "cap": options.Caps.MaxTokens})
			break
		}
		writeEvent("model", map[string]any{
			"content": response.Message.Content, "thinking": response.Message.Thinking,
			"tool_calls": response.Message.ToolCalls, "tokens": map[string]int{
				"prompt": response.PromptEvalCount, "completion": response.EvalCount,
			},
		})

		assistant := model.Message{Role: "assistant", Content: response.Message.Content, ToolCalls: response.Message.ToolCalls}
		messages = append(messages, assistant)

		if len(response.Message.ToolCalls) > 0 && record.Tokens.Total >= options.Caps.MaxTokens*4/5 {
			if completeFinalAnswer(deadlineCtx, options, record, messages, writeEvent) {
				break
			}
			record.Status = statusFailedCap
			record.Error = "token reserve reached and final answer was invalid"
			break
		}

		if len(response.Message.ToolCalls) == 0 {
			diagnosis, err := parseDiagnosis(response.Message.Content)
			if err != nil {
				lastErr = err.Error()
				if repairAttempts >= 1 {
					record.Status = statusFailedParse
					record.Error = lastErr
					writeEvent("error", map[string]any{"error": record.Error})
					break
				}
				repairAttempts++
				messages = append(messages, model.Message{
					Role:    "user",
					Content: "Your previous answer was not valid diagnosis JSON: " + err.Error() + ". Return only the JSON object from the required schema.",
				})
				writeEvent("repair", map[string]any{"error": err.Error()})
				continue
			}
			record.Diagnosis = diagnosis
			record.Status = statusCompleted
			writeEvent("diagnosis", diagnosis)
			break
		}

		for _, call := range response.Message.ToolCalls {
			if record.ToolCalls >= options.Caps.MaxToolCalls {
				forceFinal = true
				break
			}
			record.ToolCalls++
			writeEvent("tool_call", map[string]any{"name": call.Function.Name, "arguments": call.Function.Arguments})
			result, err := options.Tools.Call(deadlineCtx, call.Function.Name, call.Function.Arguments)
			if err != nil {
				record.Status = statusFailedModel
				record.Error = err.Error()
				writeEvent("tool_error", map[string]any{"name": call.Function.Name, "error": err.Error()})
				continue
			}
			record.ToolPayloadBytes += len(result.Text)
			if result.Denied {
				record.Denials++
			}
			if !result.IsError && call.Function.Name == "weave__expand_scope" {
				record.Expansions++
			}
			addSearchBreadth(record.SearchBreadth, call.Function.Arguments)
			text := result.Text
			if result.IsError {
				text = "tool error: " + text
			}
			writeEvent("tool_result", map[string]any{
				"name": call.Function.Name, "is_error": result.IsError, "denied": result.Denied, "result": result.Text,
			})
			messages = append(messages, model.Message{Role: "tool", Content: truncateText(text, 6000), ToolName: call.Function.Name})
		}
		if !forceFinal {
			continue
		}
		// Tool budget is exhausted: one final call with no tools, asking for
		// the best diagnosis from the evidence already gathered.
		if !completeFinalAnswer(deadlineCtx, options, record, messages, writeEvent) {
			record.Status = statusFailedCap
			record.Error = "tool cap reached and final answer was invalid"
		}
		break
	}
	record.EndedAt = time.Now().UTC()
	record.DurationMS = record.EndedAt.Sub(record.StartedAt).Milliseconds()
	if record.Status == "" {
		record.Status = statusCompleted
	}
	writeEvent("finish", map[string]any{"status": record.Status, "error": record.Error})
	return record, nil
}

func completeFinalAnswer(ctx context.Context, options RunOptions, record *RunRecord, messages []model.Message, writeEvent func(string, any)) bool {
	messages = append(append([]model.Message(nil), messages...), model.Message{
		Role:    "user",
		Content: "The tool budget is exhausted or nearly exhausted. Return the diagnosis JSON now using only the evidence already gathered. Do not call tools.",
	})
	finalResponse, err := options.Model.Chat(ctx, messages, nil)
	if err != nil {
		record.Error = "final answer failed: " + err.Error()
		return false
	}
	record.Tokens.Prompt += finalResponse.PromptEvalCount
	record.Tokens.Completion += finalResponse.EvalCount
	record.Tokens.Total = record.Tokens.Prompt + record.Tokens.Completion
	writeEvent("model", map[string]any{
		"content": finalResponse.Message.Content, "thinking": finalResponse.Message.Thinking,
		"tool_calls": finalResponse.Message.ToolCalls, "final_without_tools": true,
		"tokens": map[string]int{"prompt": finalResponse.PromptEvalCount, "completion": finalResponse.EvalCount},
	})
	diagnosis, parseErr := parseDiagnosis(finalResponse.Message.Content)
	if parseErr != nil {
		record.Error = "final output was invalid: " + parseErr.Error()
		return false
	}
	record.Diagnosis = diagnosis
	record.Status = statusCompleted
	writeEvent("diagnosis", diagnosis)
	return true
}

func parseDiagnosis(content string) (*Diagnosis, error) {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		for len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
			lines = lines[1:]
		}
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1]
		}
		trimmed = strings.Join(lines, "\n")
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON object in the response")
	}
	var diagnosis Diagnosis
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &diagnosis); err != nil {
		return nil, err
	}
	if strings.TrimSpace(diagnosis.RootCause.Workload) == "" {
		return nil, fmt.Errorf("root_cause.workload is required")
	}
	switch diagnosis.RootCause.FailureMode {
	case "http_error", "latency", "dependency_unavailable", "bad_deployment", "resource_saturation", "network_fault", "other":
	default:
		return nil, fmt.Errorf("root_cause.failure_mode is invalid")
	}
	if diagnosis.Confidence < 0 || diagnosis.Confidence > 1 {
		return nil, fmt.Errorf("confidence must be between 0 and 1")
	}
	if strings.TrimSpace(diagnosis.Summary) == "" {
		return nil, fmt.Errorf("summary is required")
	}
	return &diagnosis, nil
}

func addSearchBreadth(breadth map[string]int, arguments map[string]any) {
	add := func(kind, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		breadth[kind]++
	}
	for key, raw := range arguments {
		value, ok := raw.(string)
		if !ok {
			continue
		}
		switch key {
		case "node_id":
			add("nodes", value)
		case "edge_id":
			add("edges", value)
		case "workload", "deployment", "statefulset":
			add("workloads", value)
		case "pod":
			add("pods", value)
		case "path":
			if strings.HasPrefix(value, "/") {
				add("routes", value)
			}
		default:
		}
	}
}

func truncateText(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "\n[truncated]"
}

type transcriptWriter struct {
	file    *os.File
	encoder *json.Encoder
}

func newTranscriptWriter(path string) (*transcriptWriter, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &transcriptWriter{file: file, encoder: json.NewEncoder(file)}, nil
}

func (writer *transcriptWriter) Write(kind string, data any) error {
	return writer.encoder.Encode(map[string]any{
		"at":   time.Now().UTC(),
		"type": kind,
		"data": data,
	})
}

func (writer *transcriptWriter) Close() {
	_ = writer.file.Close()
}
