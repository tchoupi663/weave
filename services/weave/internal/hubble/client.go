package hubble

import (
	"context"
	"log/slog"
	"time"

	observerpb "github.com/cilium/cilium/api/v1/observer"
	"github.com/tchoupi663/weave/services/weave/internal/topology"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Client follows Hubble Relay and hands every observed flow to the topology
// store. Reconnects resume from the last observed flow; the store's UUID
// dedupe absorbs any replay overlap.
type Client struct {
	addr  string
	store *topology.Store
	log   *slog.Logger
}

func New(addr string, store *topology.Store, log *slog.Logger) *Client {
	return &Client{addr: addr, store: store, log: log}
}

func (c *Client) Run(ctx context.Context) {
	since := time.Now().Add(-2 * time.Second)
	backoff := time.Second

	for ctx.Err() == nil {
		connected, last, err := c.stream(ctx, since)
		if ctx.Err() != nil {
			return
		}
		c.store.SetHubble(false, err)
		if connected {
			backoff = time.Second
		} else if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
		if last.After(since) {
			since = last.Add(time.Nanosecond)
		}
		c.log.Warn("hubble stream ended", "error", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (c *Client) stream(ctx context.Context, since time.Time) (bool, time.Time, error) {
	conn, err := grpc.NewClient(c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUserAgent("weave/0.1"),
	)
	if err != nil {
		return false, since, err
	}
	defer conn.Close()

	client := observerpb.NewObserverClient(conn)
	stream, err := client.GetFlows(ctx, &observerpb.GetFlowsRequest{
		Follow: true,
		Since:  timestamppb.New(since),
	})
	if err != nil {
		return false, since, err
	}
	c.store.SetHubble(true, nil)
	c.log.Info("following hubble relay", "addr", c.addr, "since", since)

	last := since
	for {
		response, err := stream.Recv()
		if err != nil {
			return true, last, err
		}
		flow := response.GetFlow()
		if flow == nil {
			continue
		}
		c.store.Ingest(flow)
		if ts := flow.GetTime(); ts != nil {
			at := ts.AsTime()
			if at.After(last) {
				last = at
			}
		}
	}
}
