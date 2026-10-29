package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/tchoupi663/weave/services/simulator/internal/config"
	"github.com/tchoupi663/weave/services/simulator/internal/journey"
	"github.com/tchoupi663/weave/services/simulator/internal/report"
	"github.com/tchoupi663/weave/services/simulator/internal/topology"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "run":
		os.Exit(runCommand(os.Args[2:]))
	case "validate":
		os.Exit(validateCommand(os.Args[2:]))
	case "help", "-h", "--help":
		usage()
		os.Exit(0)
	default:
		usage()
		os.Exit(2)
	}
}

func runCommand(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the run spec YAML")
	reportPath := fs.String("report", "", "write the JSON report to this path instead of stdout")
	logRequests := fs.Bool("log-requests", false, "include a full per-request log in the report")
	sourceIP := fs.String("source-ip", "", "override the reported source IP")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "run: --config is required")
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		return 2
	}
	expected, err := topology.Load(cfg.Run.ExpectedTopology)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: expected topology: %v\n", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := journey.New(cfg, expected, journey.Options{
		LogRequests: *logRequests,
		SourceIP:    *sourceIP,
	})
	rep, err := runner.Run(ctx)
	if err != nil {
		slog.Error("run failed", "err", err)
		return 2
	}
	if err := writeReport(*reportPath, rep); err != nil {
		fmt.Fprintf(os.Stderr, "run: write report: %v\n", err)
		return 2
	}

	printSummary(rep)
	if failedJourneys(rep) > 0 || !covered(rep, cfg) {
		return 1
	}
	return 0
}

func validateCommand(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	configPath := fs.String("config", "", "path to the run spec YAML")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "validate: --config is required")
		return 2
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "validate: %v\n", err)
		return 2
	}
	if _, err := topology.Load(cfg.Run.ExpectedTopology); err != nil {
		fmt.Fprintf(os.Stderr, "validate: expected topology: %v\n", err)
		return 2
	}
	fmt.Printf("ok: run %s, target %s\n", cfg.Run.ID, cfg.Run.Target)
	return 0
}

func writeReport(path string, rep *report.Report) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if path == "" {
		_, err = os.Stdout.Write(append(data, '\n'))
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func printSummary(rep *report.Report) {
	fmt.Fprintf(os.Stderr, "run %s: source_ip=%s\n", rep.RunID, rep.SourceIP)
	for _, name := range []string{"bounce", "browse", "order"} {
		stats := rep.Journeys[name]
		fmt.Fprintf(os.Stderr, "  %-7s started=%d completed=%d failed=%d\n",
			name, stats.Started, stats.Completed, stats.Failed)
	}
	fmt.Fprintf(os.Stderr, "  failures=%d\n", len(rep.Failures))
}

func failedJourneys(rep *report.Report) int {
	total := 0
	for _, stats := range rep.Journeys {
		total += stats.Failed
	}
	return total
}

var requiredSteps = map[string][]string{
	"bounce": {"landing"},
	"browse": {"landing", "products.list", "product.page", "product.detail"},
	"order":  {"landing", "auth.login", "products.list", "order.create", "payment.create", "order.confirmation"},
}

func covered(rep *report.Report, cfg config.Config) bool {
	ok := true
	for name, share := range cfg.Run.Mix {
		if share <= 0 {
			continue
		}
		if rep.Journeys[name].Started == 0 {
			fmt.Fprintf(os.Stderr, "coverage: journey %q never ran\n", name)
			ok = false
			continue
		}
		for _, step := range requiredSteps[name] {
			if rep.Steps[step].Count == 0 {
				fmt.Fprintf(os.Stderr, "coverage: journey %q never exercised step %q\n", name, step)
				ok = false
			}
		}
	}
	return ok
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  sim run --config run.yaml [--report report.json] [--log-requests] [--source-ip IP]
  sim validate --config run.yaml`)
}
