package live

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/hubble"
	"github.com/tchoupi663/weave/services/weave/internal/topology"
)

// Config is the weave-live service configuration.
type Config struct {
	ListenAddr      string
	HubbleRelayAddr string
	WatchNamespace  string
	Window          time.Duration
	HistoryURL      string
	FeedInterval    time.Duration
	FeedBatch       int
}

// Live owns the observed graph, the two ingest paths, and the observation feed
// to weave-history. It has no durable state.
type Live struct {
	cfg   Config
	log   *slog.Logger
	store *topology.Store
	feed  *Feed
}

// New builds the live engine. It does not start goroutines until Start.
func New(cfg Config, log *slog.Logger) *Live {
	store := topology.NewStore(cfg.WatchNamespace, cfg.Window)
	live := &Live{cfg: cfg, log: log, store: store}
	live.feed = NewFeed(cfg.HistoryURL, cfg.FeedInterval, cfg.FeedBatch, log)
	if live.feed.Enabled() {
		store.SetRecorder(live.feed)
	}
	return live
}

// Start launches the observation feed and the Hubble stream follower.
func (l *Live) Start(ctx context.Context) {
	if l.feed.Enabled() {
		go l.feed.Run(ctx)
	} else {
		l.log.Warn("observation feed disabled: HISTORY_URL is empty")
	}
	client := hubble.New(l.cfg.HubbleRelayAddr, l.store, l.log)
	go client.Run(ctx)
}

// Store exposes the live graph to the HTTP handlers and tests.
func (l *Live) Store() *topology.Store { return l.store }

// FeedStatus reports the delivery state of the observation feed.
func (l *Live) FeedStatus() FeedStatus { return l.feed.Status() }

// Handler returns the weave-live HTTP handler.
func (l *Live) Handler() http.Handler { return newHTTPHandler(l) }

// Stop flushes buffered observations once and stops the feed.
func (l *Live) Stop() { l.feed.Stop() }
