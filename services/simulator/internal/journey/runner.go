package journey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tchoupi663/weave/services/simulator/internal/client"
	"github.com/tchoupi663/weave/services/simulator/internal/config"
	"github.com/tchoupi663/weave/services/simulator/internal/report"
	"github.com/tchoupi663/weave/services/simulator/internal/topology"
)

const (
	gracePeriod     = 60 * time.Second
	preflightWindow = 60 * time.Second
)

var assetPattern = regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`)

// Options tunes the Runner.
type Options struct {
	LogRequests  bool
	SourceIP     string
	StepPauseMin time.Duration
	StepPauseMax time.Duration
}

// Runner executes one Run.
type Runner struct {
	cfg       config.Config
	expected  topology.Topology
	opts      Options
	agg       *report.Aggregator
	userSeq   atomic.Int64
	accounts  []string
	transport *http.Transport
	sourceIP  string
}

// New returns a Runner.
func New(cfg config.Config, expected topology.Topology, opts Options) *Runner {
	if opts.StepPauseMax <= 0 {
		opts.StepPauseMin = 100 * time.Millisecond
		opts.StepPauseMax = 500 * time.Millisecond
	}
	if opts.StepPauseMin < 0 {
		opts.StepPauseMin = 0
	}
	sourceIP := opts.SourceIP
	if sourceIP == "" {
		sourceIP = detectSourceIP()
	}

	accounts := make([]string, cfg.Run.Accounts.Pool)
	for i := range accounts {
		accounts[i] = fmt.Sprintf("user%04d", i+1)
	}
	rng := rand.New(rand.NewSource(cfg.Run.Seed))
	rng.Shuffle(len(accounts), func(i, j int) { accounts[i], accounts[j] = accounts[j], accounts[i] })

	return &Runner{
		cfg:       cfg,
		expected:  expected,
		opts:      opts,
		agg:       report.NewAggregator(opts.LogRequests),
		accounts:  accounts,
		transport: client.NewTransport(cfg.Run.TLS.InsecureSkipVerify),
		sourceIP:  sourceIP,
	}
}

type user struct {
	index   int64
	rng     *rand.Rand
	client  *client.Client
	account string
}

type stepError struct {
	step string
	err  error
}

func (e *stepError) Error() string { return e.step + ": " + e.err.Error() }
func (e *stepError) Unwrap() error { return e.err }

// Run performs preflight, reset, and the measured traffic window.
func (r *Runner) Run(ctx context.Context) (*report.Report, error) {
	defer r.transport.CloseIdleConnections()
	if err := r.preflight(ctx); err != nil {
		return nil, err
	}
	if r.cfg.Run.Reset {
		if err := r.reset(ctx); err != nil {
			return nil, err
		}
	}

	started := time.Now().UTC()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	admitCtx, stopAdmit := context.WithCancel(runCtx)
	defer stopAdmit()

	var wg sync.WaitGroup
	for i := 0; i < r.cfg.Run.Concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if ramp := r.cfg.Run.Ramp.D(); ramp > 0 {
				delay := time.Duration(int64(ramp) * int64(i) / int64(r.cfg.Run.Concurrency))
				if err := sleepCtx(admitCtx, delay); err != nil {
					return
				}
			}
			for admitCtx.Err() == nil {
				seq := r.userSeq.Add(1)
				u, err := r.newUser(seq)
				if err != nil {
					slog.Error("create user", "err", err)
					return
				}
				r.runUser(runCtx, u)
			}
		}(i)
	}

	timer := time.AfterFunc(r.cfg.Run.Duration.D(), stopAdmit)
	defer timer.Stop()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(gracePeriod):
		slog.Warn("grace period elapsed; cancelling active users")
		cancel()
		<-done
	}

	ended := time.Now().UTC()
	rep := r.agg.Build(
		r.cfg.Run.ID,
		r.cfg.Run.Seed,
		r.cfg,
		started,
		ended,
		r.sourceIP,
		r.expected,
	)
	return &rep, nil
}

func (r *Runner) preflight(ctx context.Context) error {
	c, err := client.NewWithTransport(r.cfg.Run.Target, r.transport)
	if err != nil {
		return fmt.Errorf("build client: %w", err)
	}
	deadline := time.Now().Add(preflightWindow)
	var lastErr error
	var lastStatus int
	for {
		resp, err := c.Get(ctx, "/health")
		if err == nil && resp.Status == httpStatusOK {
			break
		}
		lastErr = err
		if resp != nil {
			lastStatus = resp.Status
		} else {
			lastStatus = 0
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("target health check failed: status %d: %v", lastStatus, lastErr)
		}
		if err := sleepCtx(ctx, 500*time.Millisecond); err != nil {
			return err
		}
	}
	resp, err := c.Get(ctx, "/api/products")
	if err != nil {
		return fmt.Errorf("target preflight request failed: %w", err)
	}
	if resp.Status != httpStatusOK {
		return fmt.Errorf("target preflight returned status %d", resp.Status)
	}
	return nil
}

func (r *Runner) reset(ctx context.Context) error {
	c, err := client.NewWithTransport(r.cfg.Run.Target, r.transport)
	if err != nil {
		return err
	}
	resp, err := c.Post(ctx, "/admin/reset", nil)
	if err != nil {
		return fmt.Errorf("reset target: %w", err)
	}
	if resp.Status != httpStatusOK {
		return fmt.Errorf("reset target returned status %d", resp.Status)
	}
	return nil
}

func (r *Runner) newUser(seq int64) (*user, error) {
	c, err := client.NewWithTransport(r.cfg.Run.Target, r.transport)
	if err != nil {
		return nil, err
	}
	idx := (seq - 1) % int64(len(r.accounts))
	account := r.accounts[int(idx)]
	return &user{
		index:   seq,
		rng:     rand.New(rand.NewSource(r.cfg.Run.Seed + seq*7919)),
		client:  c,
		account: account,
	}, nil
}

func (r *Runner) runUser(ctx context.Context, u *user) {
	name := pickJourney(u.rng, r.cfg.Run.Mix)
	r.agg.StartJourney(name)
	if err := r.execute(ctx, u, name); err != nil {
		if ctx.Err() != nil {
			return
		}
		r.agg.FailJourney(name)
		step := "unknown"
		var se *stepError
		if errors.As(err, &se) {
			step = se.step
		}
		r.agg.RecordFailure(u.index, name, step, err.Error())
		return
	}
	r.agg.CompleteJourney(name)
}

func (r *Runner) execute(ctx context.Context, u *user, name string) error {
	switch name {
	case "bounce":
		return r.bounce(ctx, u)
	case "browse":
		return r.browse(ctx, u)
	case "order":
		return r.order(ctx, u)
	default:
		return fmt.Errorf("unknown journey %q", name)
	}
}

func (r *Runner) bounce(ctx context.Context, u *user) error {
	html, err := r.get(ctx, u, "bounce", "landing", "/")
	if err != nil {
		return err
	}
	return r.assets(ctx, u, "bounce", html)
}

func (r *Runner) browse(ctx context.Context, u *user) error {
	html, err := r.get(ctx, u, "browse", "landing", "/")
	if err != nil {
		return err
	}
	if err := r.assets(ctx, u, "browse", html); err != nil {
		return err
	}
	if err := r.think(ctx, u); err != nil {
		return err
	}
	products, err := r.products(ctx, u, "browse")
	if err != nil {
		return err
	}
	for _, p := range pickProducts(u.rng, products) {
		if err := r.think(ctx, u); err != nil {
			return err
		}
		if _, err := r.get(ctx, u, "browse", "product.page", "/products/"+p.ID); err != nil {
			return err
		}
		if _, err := r.apiGet(ctx, u, "browse", "product.detail", "/api/products/"+p.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) order(ctx context.Context, u *user) error {
	html, err := r.get(ctx, u, "order", "landing", "/")
	if err != nil {
		return err
	}
	if err := r.assets(ctx, u, "order", html); err != nil {
		return err
	}
	if err := r.think(ctx, u); err != nil {
		return err
	}

	body, err := r.apiPost(ctx, u, "order", "auth.login", "/api/auth/login", map[string]string{
		"username": u.account,
		"password": "password",
	})
	if err != nil {
		return err
	}
	var login struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &login); err != nil {
		return &stepError{step: "auth.login", err: err}
	}
	if login.Token == "" {
		return &stepError{step: "auth.login", err: fmt.Errorf("empty token")}
	}
	u.client.SetToken(login.Token)

	if err := r.think(ctx, u); err != nil {
		return err
	}
	products, err := r.products(ctx, u, "order")
	if err != nil {
		return err
	}

	type orderLine struct {
		Product  product
		Quantity int
	}
	lines := make([]orderLine, 0, 4)
	total := 0.0
	for _, p := range pickProducts(u.rng, products) {
		if err := r.think(ctx, u); err != nil {
			return err
		}
		if _, err := r.get(ctx, u, "order", "product.page", "/products/"+p.ID); err != nil {
			return err
		}
		if _, err := r.apiGet(ctx, u, "order", "product.detail", "/api/products/"+p.ID); err != nil {
			return err
		}
		quantity := 1 + u.rng.Intn(2)
		lines = append(lines, orderLine{Product: p, Quantity: quantity})
		total += p.Price * float64(quantity)
	}
	total = math.Round(total*100) / 100

	if err := r.think(ctx, u); err != nil {
		return err
	}
	if _, err := r.get(ctx, u, "order", "cart.page", "/cart"); err != nil {
		return err
	}
	if err := r.think(ctx, u); err != nil {
		return err
	}
	if _, err := r.get(ctx, u, "order", "checkout.page", "/checkout"); err != nil {
		return err
	}
	if err := r.think(ctx, u); err != nil {
		return err
	}

	items := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		items = append(items, map[string]any{
			"productId": line.Product.ID,
			"quantity":  line.Quantity,
		})
	}
	body, err = r.apiPost(ctx, u, "order", "order.create", "/api/orders", map[string]any{
		"items": items,
		"total": total,
	})
	if err != nil {
		return err
	}
	var created struct {
		OrderID string  `json:"orderId"`
		Total   float64 `json:"total"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		return &stepError{step: "order.create", err: err}
	}
	if created.OrderID == "" {
		return &stepError{step: "order.create", err: fmt.Errorf("empty order id")}
	}
	amount := total
	if created.Total > 0 {
		amount = created.Total
	}

	if err := r.think(ctx, u); err != nil {
		return err
	}
	if _, err := r.get(ctx, u, "order", "payment.page", "/payment"); err != nil {
		return err
	}
	if err := r.think(ctx, u); err != nil {
		return err
	}
	if _, err := r.apiPost(ctx, u, "order", "payment.create", "/api/payments", map[string]any{
		"orderId": created.OrderID,
		"amount":  amount,
		"card":    "4242424242424242",
	}); err != nil {
		return err
	}
	if err := r.think(ctx, u); err != nil {
		return err
	}
	if _, err := r.get(ctx, u, "order", "order.page", "/orders/"+created.OrderID); err != nil {
		return err
	}
	if _, err := r.apiGet(ctx, u, "order", "order.confirmation", "/api/orders/"+created.OrderID); err != nil {
		return err
	}
	return nil
}

type product struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

func (r *Runner) products(ctx context.Context, u *user, journeyName string) ([]product, error) {
	body, err := r.apiGet(ctx, u, journeyName, "products.list", "/api/products")
	if err != nil {
		return nil, err
	}
	var products []product
	if err := json.Unmarshal(body, &products); err != nil {
		return nil, &stepError{step: "products.list", err: err}
	}
	if len(products) == 0 {
		return nil, &stepError{step: "products.list", err: fmt.Errorf("empty catalog")}
	}
	return products, nil
}

func pickProducts(rng *rand.Rand, products []product) []product {
	count := 2 + rng.Intn(3)
	if count > len(products) {
		count = len(products)
	}
	indexes := rng.Perm(len(products))[:count]
	out := make([]product, 0, count)
	for _, idx := range indexes {
		out = append(out, products[idx])
	}
	return out
}

func pickJourney(rng *rand.Rand, mix map[string]int) string {
	n := rng.Intn(100)
	cumulative := 0
	for _, name := range []string{"bounce", "browse", "order"} {
		cumulative += mix[name]
		if n < cumulative {
			return name
		}
	}
	return "bounce"
}

func (r *Runner) assets(ctx context.Context, u *user, journeyName string, html []byte) error {
	for _, path := range parseAssets(html) {
		if _, err := r.get(ctx, u, journeyName, "asset", path); err != nil {
			return err
		}
	}
	return nil
}

func parseAssets(html []byte) []string {
	matches := assetPattern.FindAllSubmatch(html, -1)
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		path := string(match[1])
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func (r *Runner) get(ctx context.Context, u *user, journeyName, step, path string) ([]byte, error) {
	resp, err := u.client.Get(ctx, path)
	r.record(u, journeyName, step, resp, err)
	if err != nil {
		return nil, &stepError{step: step, err: err}
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return nil, &stepError{step: step, err: fmt.Errorf("status %d", resp.Status)}
	}
	return resp.Body, nil
}

func (r *Runner) apiGet(ctx context.Context, u *user, journeyName, step, path string) ([]byte, error) {
	body, err := r.get(ctx, u, journeyName, step, path)
	if err != nil {
		return nil, err
	}
	if err := r.pause(ctx, u); err != nil {
		return nil, err
	}
	return body, nil
}

func (r *Runner) apiPost(ctx context.Context, u *user, journeyName, step, path string, payload any) ([]byte, error) {
	resp, err := u.client.Post(ctx, path, payload)
	r.record(u, journeyName, step, resp, err)
	if err != nil {
		return nil, &stepError{step: step, err: err}
	}
	if resp.Status < 200 || resp.Status >= 300 {
		return nil, &stepError{step: step, err: fmt.Errorf("status %d", resp.Status)}
	}
	if err := r.pause(ctx, u); err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (r *Runner) record(u *user, journeyName, step string, resp *client.Response, err error) {
	status := 0
	ms := 0.0
	bytes := int64(0)
	method := ""
	url := ""
	if resp != nil {
		status = resp.Status
		ms = float64(resp.Duration.Microseconds()) / 1000
		bytes = resp.Bytes
		method = resp.Method
		url = resp.URL
	}
	failed := err != nil || status < 200 || status >= 300
	r.agg.RecordStep(step, status, ms, bytes, failed)
	if r.opts.LogRequests {
		r.agg.RecordRequest(report.Request{
			User:    u.index,
			Journey: journeyName,
			Step:    step,
			Method:  method,
			URL:     url,
			Status:  status,
			MS:      ms,
			Bytes:   bytes,
		})
	}
}

func (r *Runner) think(ctx context.Context, u *user) error {
	return sleepRange(ctx, u.rng, r.cfg.Run.ThinkTime.Min.D(), r.cfg.Run.ThinkTime.Max.D())
}

func (r *Runner) pause(ctx context.Context, u *user) error {
	return sleepRange(ctx, u.rng, r.opts.StepPauseMin, r.opts.StepPauseMax)
}

func sleepRange(ctx context.Context, rng *rand.Rand, min, max time.Duration) error {
	if max <= 0 {
		return nil
	}
	if max <= min {
		return sleepCtx(ctx, min)
	}
	delta := int64(max - min)
	d := min + time.Duration(rng.Int63n(delta+1))
	return sleepCtx(ctx, d)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func detectSourceIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() || ipNet.IP.To4() == nil {
			continue
		}
		return ipNet.IP.String()
	}
	return ""
}

const httpStatusOK = 200
