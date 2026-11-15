// weave-agent runs the local investigator and its five read-only MCP servers.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tchoupi663/weave/services/agent/internal/config"
	"github.com/tchoupi663/weave/services/agent/internal/investigation"
	"github.com/tchoupi663/weave/services/agent/internal/mcpclient"
	"github.com/tchoupi663/weave/services/agent/internal/model"
	"github.com/tchoupi663/weave/services/agent/internal/scope"
	"github.com/tchoupi663/weave/services/agent/internal/tools/kube"
	"github.com/tchoupi663/weave/services/agent/internal/tools/logs"
	"github.com/tchoupi663/weave/services/agent/internal/tools/metrics"
	"github.com/tchoupi663/weave/services/agent/internal/tools/source"
	"github.com/tchoupi663/weave/services/agent/internal/tools/weave"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: weave-agent <mcp NAME|investigator> [flags]")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "mcp":
		runMCP(os.Args[2:])
	case "investigator":
		runInvestigator(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}

func runMCP(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: weave-agent mcp <kube|metrics|logs|source|weave>")
		os.Exit(1)
	}
	cfg := config.FromEnv()
	guard := scope.GuardFromEnv()
	server := mcp.NewServer(&mcp.Implementation{Name: "weave-" + args[0], Version: "0.1.0"}, nil)
	switch args[0] {
	case "kube":
		kube.Register(server, kube.Config{KubeConfig: cfg.KubeConfig, Namespace: cfg.Namespace, Guard: guard})
	case "metrics":
		metrics.Register(server, metrics.Config{WeaveURL: cfg.WeaveURL, Guard: guard})
	case "logs":
		logs.Register(server, logs.Config{KubeConfig: cfg.KubeConfig, Namespace: cfg.Namespace, Guard: guard})
	case "source":
		source.Register(server, source.Config{SourceRoots: cfg.SourceRoots, Guard: guard})
	case "weave":
		weave.Register(server, weave.Config{WeaveURL: cfg.WeaveURL, Guard: guard})
	default:
		fmt.Fprintf(os.Stderr, "unknown MCP server %q\n", args[0])
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !strings.Contains(err.Error(), "closed") {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runInvestigator(args []string) {
	flags := flag.NewFlagSet("investigator", flag.ExitOnError)
	incidentID := flags.String("incident", "", "incident id to investigate")
	arm := flags.String("arm", investigation.ArmConfined, "confined or unconfined")
	regionFile := flags.String("region-file", "", "confined region payload file")
	runDir := flags.String("run-dir", "", "directory for run.json and transcript.jsonl")
	weaveURL := flags.String("weave-url", "", "Weave API base URL override")
	ollamaURL := flags.String("ollama-url", "", "Ollama base URL override")
	modelName := flags.String("model", "", "model name override")
	scopeFile := flags.String("scope-file", "", "scope file override")
	kubeConfig := flags.String("kubeconfig", "", "kubeconfig override")
	namespace := flags.String("namespace", "", "watched namespace override")
	maxToolCalls := flags.Int("max-tool-calls", 0, "tool call cap override")
	maxTokens := flags.Int("max-tokens", 0, "token cap override")
	maxWall := flags.Duration("max-wall", 0, "wall time cap override")
	_ = flags.Parse(args)
	if *incidentID == "" {
		fmt.Fprintln(os.Stderr, "--incident is required")
		os.Exit(2)
	}
	if *arm != investigation.ArmConfined && *arm != investigation.ArmUnconfined {
		fmt.Fprintln(os.Stderr, "--arm must be confined or unconfined")
		os.Exit(2)
	}
	cfg := config.FromEnv()
	if *weaveURL != "" {
		cfg.WeaveURL = *weaveURL
	}
	if *ollamaURL != "" {
		cfg.OllamaURL = *ollamaURL
	}
	if *modelName != "" {
		cfg.ModelName = *modelName
	}
	if *scopeFile != "" {
		cfg.ScopeFile = *scopeFile
	}
	if *kubeConfig != "" {
		cfg.KubeConfig = *kubeConfig
	}
	if *namespace != "" {
		cfg.Namespace = *namespace
	}
	if *maxToolCalls > 0 {
		cfg.MaxToolCalls = *maxToolCalls
	}
	if *maxTokens > 0 {
		cfg.MaxTokens = *maxTokens
	}
	if *maxWall > 0 {
		cfg.MaxWall = *maxWall
	}
	if *runDir == "" {
		*runDir = filepath.Join(".cache", "agent-runs", *incidentID)
	}
	if *arm == investigation.ArmConfined && *regionFile == "" {
		fmt.Fprintln(os.Stderr, "--region-file is required for a confined run")
		os.Exit(2)
	}
	if *arm == investigation.ArmUnconfined {
		cfg.ScopeFile = ""
	}

	incident, err := fetchIncident(cfg.WeaveURL, *incidentID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var regionJSON []byte
	if *arm == investigation.ArmConfined {
		regionJSON, err = os.ReadFile(*regionFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	binary, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	childEnv := []string{
		"WEAVE_URL=" + cfg.WeaveURL,
		"OLLAMA_URL=" + cfg.OllamaURL,
		"WEAVE_NAMESPACE=" + cfg.Namespace,
		"WEAVE_SOURCE_ROOTS=" + strings.Join(cfg.SourceRoots, ","),
		"WEAVE_MAX_TOOL_CALLS=" + fmt.Sprintf("%d", cfg.MaxToolCalls),
		"WEAVE_MAX_TOKENS=" + fmt.Sprintf("%d", cfg.MaxTokens),
		"WEAVE_MAX_WALL=" + cfg.MaxWall.String(),
	}
	if cfg.KubeConfig != "" {
		childEnv = append(childEnv, "KUBECONFIG="+cfg.KubeConfig)
	}
	if cfg.ScopeFile != "" {
		childEnv = append(childEnv, "WEAVE_SCOPE_FILE="+cfg.ScopeFile)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	manager, err := mcpclient.Start(ctx, binary, childEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer manager.Close()

	think := false
	localModel := model.New(model.Config{
		BaseURL: cfg.OllamaURL, Model: cfg.ModelName, Temperature: cfg.Temperature,
		NumCtx: cfg.NumCtx, NumPredict: cfg.NumPredict, Think: &think,
	})
	record, err := investigation.Run(ctx, investigation.RunOptions{
		Incident:      incident,
		RegionJSON:    regionJSON,
		Arm:           *arm,
		Model:         localModel,
		Tools:         manager,
		ModelName:     cfg.ModelName,
		ModelDigest:   cfg.ModelDigest,
		PromptVersion: cfg.PromptVersion,
		RunDir:        *runDir,
		Caps: investigation.Caps{
			MaxToolCalls: cfg.MaxToolCalls, MaxTokens: cfg.MaxTokens, MaxWall: cfg.MaxWall,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*runDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(*runDir, "run.json"), raw, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if record.Diagnosis != nil {
		diagnosis, _ := json.MarshalIndent(record.Diagnosis, "", "  ")
		fmt.Println(string(diagnosis))
	}
	fmt.Fprintf(os.Stderr, "status=%s tokens=%d tool_calls=%d denials=%d expansions=%d\n",
		record.Status, record.Tokens.Total, record.ToolCalls, record.Denials, record.Expansions)
	if record.Status != statusCompleted {
		os.Exit(2)
	}
}

func fetchIncident(baseURL, id string) (investigation.Incident, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(strings.TrimRight(baseURL, "/") + "/api/incidents/" + urlPathEscape(id))
	if err != nil {
		return investigation.Incident{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return investigation.Incident{}, fmt.Errorf("fetch incident: %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var incident investigation.Incident
	if err := json.NewDecoder(response.Body).Decode(&incident); err != nil {
		return investigation.Incident{}, err
	}
	return incident, nil
}

func urlPathEscape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "/", "%2F"), " ", "%20")
}

// statusCompleted mirrors investigation's completion status without exporting
// it from the package.
const statusCompleted = "completed"
