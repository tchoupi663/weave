// weave-bench runs paired confined and unconfined investigations.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tchoupi663/weave/services/agent/internal/benchmark"
)

func main() {
	flags := flag.NewFlagSet("weave-bench", flag.ExitOnError)
	profilePath := flags.String("profile", "deploy/targets/target-app/profile.yaml", "target profile path")
	weaveURL := flags.String("weave-url", "http://127.0.0.1:8090", "Weave API origin")
	agentBinary := flags.String("agent-bin", ".cache/weave-agent", "weave-agent binary")
	outDir := flags.String("out", "", "output directory (default reports/agent-benchmark/<timestamp>)")
	kubeConfig := flags.String("kubeconfig", "", "kubeconfig for fault injection")
	readOnlyKubeConfig := flags.String("readonly-kubeconfig", "", "read-only kubeconfig for the agent MCP servers")
	warmup := flags.Int("warmup-seconds", 0, "override the profile baseline before each fault")
	reps := flags.Int("repetitions", 3, "repetitions per scenario")
	pilot := flags.Bool("pilot", false, "pilot: first three scenarios, one repetition")
	scenarios := flags.String("scenarios", "", "comma-separated scenario ids (default: profile)")
	maxToolCalls := flags.Int("max-tool-calls", 20, "investigator tool-call cap")
	maxTokens := flags.Int("max-tokens", 160000, "investigator token cap")
	maxWall := flags.Duration("max-wall", 10*time.Minute, "investigator wall cap")
	_ = flags.Parse(os.Args[1:])
	ids := []string{}
	if strings.TrimSpace(*scenarios) != "" {
		for _, id := range strings.Split(*scenarios, ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	runner := benchmark.New(benchmark.Options{
		ProfilePath: *profilePath, WeaveURL: *weaveURL, AgentBinary: *agentBinary, OutDir: *outDir,
		KubeConfig: *kubeConfig, ReadOnlyKubeConfig: *readOnlyKubeConfig, ScenarioIDs: ids, Repetitions: *reps, Pilot: *pilot,
		WarmupSeconds: *warmup, MaxToolCalls: *maxToolCalls, MaxTokens: *maxTokens, MaxWall: *maxWall,
	})
	if err := runner.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
