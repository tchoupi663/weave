package authsvc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func request(t *testing.T, h http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLoginValidateReset(t *testing.T) {
	h := New().Handler()

	rec := request(t, h, http.MethodPost, "/api/auth/login", `{"username":"user0001","password":"password"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var login struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if login.Token == "" {
		t.Fatal("empty token")
	}

	rec = request(t, h, http.MethodPost, "/api/auth/validate", "", map[string]string{
		"Authorization": "Bearer " + login.Token,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("validate status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var validate struct {
		Valid    bool   `json:"valid"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &validate); err != nil {
		t.Fatal(err)
	}
	if !validate.Valid || validate.Username != "user0001" {
		t.Fatalf("unexpected validate response: %+v", validate)
	}

	rec = request(t, h, http.MethodPost, "/api/auth/login", `{"username":"user0001","password":"wrong"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d", rec.Code)
	}

	rec = request(t, h, http.MethodPost, "/internal/reset", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d", rec.Code)
	}
	rec = request(t, h, http.MethodPost, "/api/auth/validate", "", map[string]string{
		"Authorization": "Bearer " + login.Token,
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("token after reset status = %d", rec.Code)
	}
}
