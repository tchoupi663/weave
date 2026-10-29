package journey

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tchoupi663/weave/services/simulator/internal/config"
	"github.com/tchoupi663/weave/services/simulator/internal/topology"
)

func TestRunnerWithFakeTarget(t *testing.T) {
	srv := fakeTarget(t)
	defer srv.Close()

	cfg := config.Default()
	cfg.Run.ID = "test"
	cfg.Run.Seed = 7
	cfg.Run.Target = srv.URL
	cfg.Run.ExpectedTopology = "unused"
	cfg.Run.Duration = config.Duration(300 * time.Millisecond)
	cfg.Run.Concurrency = 30
	cfg.Run.Ramp = 0
	cfg.Run.ThinkTime = config.ThinkTime{
		Min: config.Duration(2 * time.Millisecond),
		Max: config.Duration(3 * time.Millisecond),
	}
	cfg.Run.Mix = map[string]int{"bounce": 30, "browse": 50, "order": 20}
	cfg.Run.Accounts.Pool = 200
	cfg.Run.Reset = true
	cfg.Run.TLS.InsecureSkipVerify = false

	topo := topology.Topology{
		Nodes: []topology.Node{{ID: "source-ip"}, {ID: "frontend"}},
		Edges: []topology.Edge{{From: "source-ip", To: "frontend", Protocol: "HTTPS"}},
	}

	runner := New(cfg, topo, Options{
		SourceIP:     "test",
		StepPauseMin: time.Nanosecond,
		StepPauseMax: time.Nanosecond,
	})
	rep, err := runner.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if rep.SourceIP != "test" {
		t.Fatalf("source ip = %q", rep.SourceIP)
	}
	for _, name := range []string{"bounce", "browse", "order"} {
		if rep.Journeys[name].Started == 0 {
			t.Fatalf("journey %q never started", name)
		}
	}
	if len(rep.Failures) != 0 {
		t.Fatalf("failures = %d: %v", len(rep.Failures), rep.Failures[:min(5, len(rep.Failures))])
	}
	for _, step := range []string{"landing", "products.list", "auth.login", "order.create", "payment.create"} {
		if rep.Steps[step].Count == 0 {
			t.Fatalf("step %q was never exercised", step)
		}
	}
	if len(rep.ExpectedTopology.Edges) != 1 {
		t.Fatalf("expected topology edges = %d", len(rep.ExpectedTopology.Edges))
	}
}

func TestParseAssets(t *testing.T) {
	html := []byte(`<html><head>
<link rel="stylesheet" href="/assets/app.css">
</head><body>
<script src="/assets/app.js"></script>
<script src="/assets/app.js"></script>
<script src="https://cdn.example/app.js"></script>
</body></html>`)
	got := parseAssets(html)
	if len(got) != 2 {
		t.Fatalf("assets = %v", got)
	}
	if got[0] != "/assets/app.css" || got[1] != "/assets/app.js" {
		t.Fatalf("assets = %v", got)
	}
}

func fakeTarget(t *testing.T) *httptest.Server {
	t.Helper()

	var products strings.Builder
	products.WriteString("[")
	for i := 1; i <= 20; i++ {
		if i > 1 {
			products.WriteString(",")
		}
		fmt.Fprintf(&products, `{"id":"p%03d","name":"Product %02d","price":%d}`, i, i, i)
	}
	products.WriteString("]")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/admin/reset":
			_, _ = w.Write([]byte(`{"status":"reset"}`))
		case r.URL.Path == "/api/products":
			_, _ = w.Write([]byte(products.String()))
		case strings.HasPrefix(r.URL.Path, "/api/products/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/products/")
			fmt.Fprintf(w, `{"id":%q,"name":"Product","price":1}`, id)
		case strings.HasPrefix(r.URL.Path, "/products/"):
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html></html>"))
		case r.URL.Path == "/api/auth/login":
			_, _ = w.Write([]byte(`{"token":"token"}`))
		case r.URL.Path == "/api/orders" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"orderId":"o1","status":"created","total":19.99}`))
		case strings.HasPrefix(r.URL.Path, "/api/orders/"):
			_, _ = w.Write([]byte(`{"orderId":"o1","status":"created","total":19.99}`))
		case r.URL.Path == "/api/payments":
			_, _ = w.Write([]byte(`{"paymentId":"pay1","status":"succeeded"}`))
		case r.URL.Path == "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><link rel="stylesheet" href="/assets/app.css"></head><body><script src="/assets/app.js"></script></body></html>`))
		case strings.HasPrefix(r.URL.Path, "/assets/"):
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = w.Write([]byte("console.log('x')"))
		case r.URL.Path == "/cart", r.URL.Path == "/checkout", r.URL.Path == "/payment",
			strings.HasPrefix(r.URL.Path, "/orders/"):
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html></html>"))
		default:
			http.NotFound(w, r)
		}
	})

	return httptest.NewServer(handler)
}
