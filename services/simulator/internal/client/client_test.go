package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestGetRetriesOnce(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "try again", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(context.Background(), "/api/products")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != http.StatusOK || string(resp.Body) != "ok" {
		t.Fatalf("status=%d body=%q", resp.Status, resp.Body)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestGetDoesNotRetry404(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.NotFound(w, nil)
	}))
	defer srv.Close()

	c, err := New(srv.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(context.Background(), "/missing")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != http.StatusNotFound {
		t.Fatalf("status = %d", resp.Status)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestPostDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := New(srv.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Post(context.Background(), "/api/payments", map[string]string{"x": "y"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != http.StatusInternalServerError {
		t.Fatalf("status = %d", resp.Status)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}
