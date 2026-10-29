package catalogsvc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListGetAndReset(t *testing.T) {
	h := New().Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	var products []Product
	if err := json.Unmarshal(rec.Body.Bytes(), &products); err != nil {
		t.Fatal(err)
	}
	if len(products) != 20 {
		t.Fatalf("products = %d, want 20", len(products))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products/p001", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d", rec.Code)
	}
	var product Product
	if err := json.Unmarshal(rec.Body.Bytes(), &product); err != nil {
		t.Fatal(err)
	}
	if product.ID != "p001" || product.Price <= 0 {
		t.Fatalf("unexpected product: %+v", product)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/reset", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d", rec.Code)
	}
}
