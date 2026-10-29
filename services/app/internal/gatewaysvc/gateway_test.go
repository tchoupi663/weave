package gatewaysvc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRoutesAuthAndReset(t *testing.T) {
	var authResets, catalogResets, ordersResets atomic.Int32

	authMux := http.NewServeMux()
	authMux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "t"})
	})
	authMux.HandleFunc("POST /api/auth/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"valid": true, "username": "user0001"})
	})
	authMux.HandleFunc("POST /internal/reset", func(w http.ResponseWriter, _ *http.Request) {
		authResets.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	authSrv := httptest.NewServer(authMux)
	defer authSrv.Close()

	catalogMux := http.NewServeMux()
	catalogMux.HandleFunc("GET /api/products", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"p001"}]`))
	})
	catalogMux.HandleFunc("GET /api/products/p001", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"p001"}`))
	})
	catalogMux.HandleFunc("POST /internal/reset", func(w http.ResponseWriter, _ *http.Request) {
		catalogResets.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	catalogSrv := httptest.NewServer(catalogMux)
	defer catalogSrv.Close()

	ordersMux := http.NewServeMux()
	ordersMux.HandleFunc("GET /api/orders/o1", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-User") != "user0001" {
			http.Error(w, "missing user", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"orderId":"o1","status":"created","total":19.99}`))
	})
	ordersMux.HandleFunc("POST /internal/reset", func(w http.ResponseWriter, _ *http.Request) {
		ordersResets.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	ordersSrv := httptest.NewServer(ordersMux)
	defer ordersSrv.Close()

	paymentMux := http.NewServeMux()
	paymentMux.HandleFunc("POST /api/payments", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-User") != "user0001" {
			http.Error(w, "missing user", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"paymentId":"pay1","status":"succeeded"}`))
	})
	paymentSrv := httptest.NewServer(paymentMux)
	defer paymentSrv.Close()

	gw := New(Config{
		AuthURL:    authSrv.URL,
		CatalogURL: catalogSrv.URL,
		OrdersURL:  ordersSrv.URL,
		PaymentURL: paymentSrv.URL,
	}, &http.Client{Timeout: 2 * time.Second})
	h := gw.Handler()

	rec := request(t, h, http.MethodGet, "/api/products", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("products status = %d", rec.Code)
	}

	rec = request(t, h, http.MethodGet, "/api/orders/o1", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no auth status = %d", rec.Code)
	}

	rec = requestWithAuth(t, h, http.MethodGet, "/api/orders/o1", "", "Bearer good")
	if rec.Code != http.StatusOK {
		t.Fatalf("order status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = requestWithAuth(t, h, http.MethodGet, "/api/orders/o1", "", "Bearer bad")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad auth status = %d", rec.Code)
	}

	rec = requestWithAuth(t, h, http.MethodPost, "/api/payments", `{"orderId":"o1"}`, "Bearer good")
	if rec.Code != http.StatusOK {
		t.Fatalf("payment status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = request(t, h, http.MethodPost, "/api/auth/login", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d", rec.Code)
	}

	rec = request(t, h, http.MethodPost, "/admin/reset", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if authResets.Load() != 1 || catalogResets.Load() != 1 || ordersResets.Load() != 1 {
		t.Fatalf("resets = auth:%d catalog:%d orders:%d", authResets.Load(), catalogResets.Load(), ordersResets.Load())
	}

	rec = request(t, h, http.MethodGet, "/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d", rec.Code)
	}

	rec = request(t, h, http.MethodGet, "/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown status = %d", rec.Code)
	}
}

func request(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func requestWithAuth(t *testing.T, h http.Handler, method, path, body, auth string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", auth)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
