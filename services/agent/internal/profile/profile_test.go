package profile

import (
	"path/filepath"
	"testing"
)

func TestLoadTargetProfile(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "deploy", "targets", "target-app", "profile.yaml")
	profile, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "target-app" || len(profile.Scenarios) != 9 {
		t.Fatalf("profile = %+v", profile)
	}
	classes := map[string]bool{}
	for _, id := range profile.Scenarios {
		scenario, err := profile.Scenario(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if len(scenario.GroundTruth.RootCauseWorkloads) == 0 || scenario.FaultDurationSeconds < 30 {
			t.Fatalf("scenario %s incomplete: %+v", id, scenario)
		}
		if scenario.Template == "" {
			t.Fatalf("scenario %s has no template", id)
		}
		classes[scenario.Template] = true
	}
	if len(classes) != 4 {
		t.Fatalf("expected 4 covered classes, got %v", classes)
	}
	if len(profile.SourceMap()["catalog"]) == 0 {
		t.Fatalf("catalog source map = %+v", profile.SourceMap())
	}
}

func TestScenarioRejectsUnknownWorkload(t *testing.T) {
	profile, err := Load(filepath.Join("..", "..", "..", "..", "deploy", "targets", "target-app", "profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	scenario := Scenario{ID: "x", FaultDurationSeconds: 30, Injection: Injection{Mechanism: "http_chaos", Fixture: "x"}, GroundTruth: GroundTruth{RootCauseWorkloads: []string{"nope"}}}
	if err := profile.validateScenario(scenario); err == nil {
		t.Fatal("unknown workload accepted")
	}
}
