package history

import (
	"context"
	"time"
)

const inactiveAfter = 30 * time.Minute

func (h *History) lifecycleLoop(ctx context.Context) {
	if h.argo == nil || h.store == nil {
		h.log.Info("history lifecycle disabled")
		return
	}
	h.reconcile(ctx)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.reconcile(ctx)
		}
	}
}

func (h *History) reconcile(ctx context.Context) {
	desiredWorkloads, err := h.argo.Workloads(ctx)
	if err != nil {
		h.log.Error("read declared workloads", "error", err)
		return
	}
	observed := h.store.ObservedWorkloads()
	now := time.Now().UTC()

	desired := make(map[string]DeclaredWorkload, len(desiredWorkloads))
	for _, workload := range desiredWorkloads {
		desired[workload.Name] = workload
	}
	observedByName := make(map[string]time.Time, len(observed))
	for _, workload := range observed {
		observedByName[workload.Workload] = workload.LastSeen
	}

	first := !h.lifecycleInit
	if !first {
		for name, workload := range desired {
			if _, existed := h.desired[name]; !existed {
				h.recordChange(ctx, now, "declared", "declared_added", name, workload.Kind, map[string]any{"kind": workload.Kind})
			}
		}
		for name, kind := range h.desired {
			if _, exists := desired[name]; !exists {
				h.recordChange(ctx, now, "declared", "declared_removed", name, kind, map[string]any{"kind": kind})
			}
		}
	}

	for name, workload := range desired {
		lastSeen, observedBefore := observedByName[name]
		state := "Declared"
		if observedBefore {
			if now.Sub(lastSeen) <= inactiveAfter {
				state = "Active"
				if h.states[name] == "" {
					h.recordChange(ctx, now, "observed", "observed_first_seen", name, workload.Kind, map[string]any{"last_seen": lastSeen})
				}
			} else {
				state = "Inactive"
			}
		}
		if !workload.Live {
			state = "Removed"
			if first || h.live[name] {
				h.recordChange(ctx, now, "live", "live_missing", name, workload.Kind, map[string]any{"kind": workload.Kind})
			}
		}
		h.setLive(name, workload.Live)
		h.transition(ctx, now, name, workload.Kind, state)
	}

	for name, lastSeen := range observedByName {
		if _, declared := desired[name]; declared {
			continue
		}
		state := "Undeclared"
		if now.Sub(lastSeen) > inactiveAfter {
			state = "Removed"
		}
		h.transition(ctx, now, name, "", state)
	}

	for name := range h.desired {
		if _, exists := desired[name]; exists {
			continue
		}
		if _, observedNow := observedByName[name]; !observedNow {
			h.transition(ctx, now, name, h.desired[name], "Removed")
		}
	}

	for name, workload := range desired {
		h.desired[name] = workload.Kind
	}
	for name := range h.desired {
		if _, exists := desired[name]; !exists {
			delete(h.desired, name)
		}
	}
	h.lifecycleInit = true
}

func (h *History) setLive(name string, live bool) {
	h.live[name] = live
}

func (h *History) transition(ctx context.Context, now time.Time, workload, kind, state string) {
	previous := h.states[workload]
	if previous == state {
		return
	}
	h.states[workload] = state
	if _, err := h.sqlite.ExecContext(ctx,
		`INSERT INTO workload_states (workload, state, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(workload) DO UPDATE SET state=excluded.state, updated_at=excluded.updated_at`,
		workload, state, now.Format(time.RFC3339Nano)); err != nil {
		h.log.Error("store workload state", "error", err, "workload", workload)
	}
	h.recordChange(ctx, now, lifecycleSource(state), "state_changed", workload, kind, map[string]any{
		"from": previous,
		"to":   state,
	})
}

func lifecycleSource(state string) string {
	switch state {
	case "Declared":
		return "declared"
	case "Undeclared":
		return "observed"
	case "Removed":
		return "live"
	default:
		return "observed"
	}
}
