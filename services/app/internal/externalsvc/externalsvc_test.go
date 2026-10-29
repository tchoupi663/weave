package externalsvc

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCharge(t *testing.T) {
	h := New().Handler()
	body := `{"orderId":"o1","amount":19.99,"card":"4242424242424242"}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/charge", strings.NewReader(body))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ChargeID string `json:"chargeId"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.ChargeID == "" || out.Status != "succeeded" {
		t.Fatalf("unexpected charge response: %+v", out)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/charge", strings.NewReader(`{}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad body status = %d", rec.Code)
	}
}

func TestSelfSignedTLSConfig(t *testing.T) {
	cfg, err := SelfSignedTLSConfig("external", "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("certificates = %d, want 1", len(cfg.Certificates))
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("min version = %d", cfg.MinVersion)
	}
}
