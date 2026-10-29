package catalogsvc

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/tchoupi663/weave/services/app/internal/httpx"
)

const productCount = 20

// Product is a catalog item.
type Product struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

// Service is an in-memory Catalog service.
type Service struct {
	mu       sync.RWMutex
	products []Product
}

// New returns a seeded Catalog service.
func New() *Service {
	s := &Service{}
	s.Reset()
	return s
}

// Reset restores the fixed product set.
func (s *Service) Reset() {
	products := make([]Product, 0, productCount)
	for i := 1; i <= productCount; i++ {
		products = append(products, Product{
			ID:    fmt.Sprintf("p%03d", i),
			Name:  fmt.Sprintf("Product %02d", i),
			Price: 9.99 + float64(i-1),
		})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.products = products
}

// List returns a copy of the product list.
func (s *Service) List() []Product {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Product, len(s.products))
	copy(out, s.products)
	return out
}

// Get returns a product by ID.
func (s *Service) Get(id string) (Product, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.products {
		if p.ID == id {
			return p, true
		}
	}
	return Product{}, false
}

// Handler returns the Catalog HTTP handler.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/products", s.handleList)
	mux.HandleFunc("GET /api/products/{id}", s.handleGet)
	mux.HandleFunc("POST /internal/reset", s.handleReset)
	mux.HandleFunc("GET /health", httpx.Health)
	return mux
}

func (s *Service) handleList(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, s.List())
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	product, ok := s.Get(r.PathValue("id"))
	if !ok {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	httpx.JSON(w, http.StatusOK, product)
}

func (s *Service) handleReset(w http.ResponseWriter, _ *http.Request) {
	s.Reset()
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "reset"})
}
