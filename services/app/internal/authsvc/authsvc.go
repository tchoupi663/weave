package authsvc

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/tchoupi663/weave/services/app/internal/httpx"
)

const (
	accountCount = 2000
	password     = "password"
)

// Service is an in-memory Auth service with a fixed account pool.
type Service struct {
	mu     sync.RWMutex
	users  map[string]string
	tokens map[string]string
}

// New returns a seeded Auth service.
func New() *Service {
	s := &Service{}
	s.Reset()
	return s
}

// Reset clears tokens and reseeds the account pool.
func (s *Service) Reset() {
	users := make(map[string]string, accountCount)
	for i := 1; i <= accountCount; i++ {
		users[fmt.Sprintf("user%04d", i)] = password
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users = users
	s.tokens = make(map[string]string)
}

// Login validates credentials and returns an opaque token.
func (s *Service) Login(username, pass string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.users[username]
	if !ok || stored != pass {
		return "", false
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", false
	}
	token := hex.EncodeToString(b[:])
	s.tokens[token] = username
	return token, true
}

// Validate returns the username for a token.
func (s *Service) Validate(token string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	username, ok := s.tokens[token]
	return username, ok
}

// Handler returns the Auth HTTP handler.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/validate", s.handleValidate)
	mux.HandleFunc("POST /internal/reset", s.handleReset)
	mux.HandleFunc("GET /health", httpx.Health)
	return mux
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !httpx.ReadJSON(w, r, &req) {
		return
	}
	if req.Username == "" || req.Password == "" {
		httpx.Error(w, http.StatusBadRequest, "username and password are required")
		return
	}
	token, ok := s.Login(req.Username, req.Password)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"token": token})
}

func (s *Service) handleValidate(w http.ResponseWriter, r *http.Request) {
	token := bearer(r)
	if token == "" {
		httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
		return
	}
	username, ok := s.Validate(token)
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"valid": true, "username": username})
}

func (s *Service) handleReset(w http.ResponseWriter, _ *http.Request) {
	s.Reset()
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func bearer(r *http.Request) string {
	header := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(token)
}
