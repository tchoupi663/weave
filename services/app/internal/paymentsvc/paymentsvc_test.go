package paymentsvc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreatePayment(t *testing.T) {
	var calls int
	external := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req chargeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if req.OrderID != "o1" || req.Amount != 19.99 {
			http.Error(w, "unexpected charge", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chargeResponse{ChargeID: "c1", Status: "succeeded"})
	}))
	defer external.Close()

	h := New(external.URL, external.Client()).Handler()
	body := `{"orderId":"o1","amount":19.99,"card":"4242424242424242"}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(body))
	req.Header.Set("X-User", "user0001")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out struct {
		PaymentID string `json:"paymentId"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.PaymentID == "" || out.Status != "succeeded" {
		t.Fatalf("unexpected payment response: %+v", out)
	}
	if calls != 1 {
		t.Fatalf("external calls = %d, want 1", calls)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(body))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing user status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(`{}`))
	req.Header.Set("X-User", "user0001")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad body status = %d", rec.Code)
	}
}

func TestExternalFailure(t *testing.T) {
	external := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer external.Close()

	h := New(external.URL, external.Client()).Handler()
	body := `{"orderId":"o1","amount":19.99,"card":"4242424242424242"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(body))
	req.Header.Set("X-User", "user0001")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
