package orderssvc

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Item is a product line in an Order.
type Item struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

// Order is a created order.
type Order struct {
	ID        string    `json:"orderId"`
	UserID    string    `json:"-"`
	Status    string    `json:"status"`
	Total     float64   `json:"total"`
	Items     []Item    `json:"items"`
	CreatedAt time.Time `json:"createdAt"`
}

// ErrNotFound is returned when an order does not exist for the user.
var ErrNotFound = errors.New("order not found")

// Store persists Orders.
type Store interface {
	Create(ctx context.Context, o Order) error
	Get(ctx context.Context, id, userID string) (Order, error)
	Reset(ctx context.Context) error
	Ping(ctx context.Context) error
}

// MemoryStore is an in-memory Store for tests.
type MemoryStore struct {
	mu     sync.RWMutex
	orders map[string]Order
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{orders: make(map[string]Order)}
}

func (m *MemoryStore) Create(_ context.Context, o Order) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	o.Items = append([]Item(nil), o.Items...)
	m.orders[o.ID] = o
	return nil
}

func (m *MemoryStore) Get(_ context.Context, id, userID string) (Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.orders[id]
	if !ok || o.UserID != userID {
		return Order{}, ErrNotFound
	}
	o.Items = append([]Item(nil), o.Items...)
	return o, nil
}

func (m *MemoryStore) Reset(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orders = make(map[string]Order)
	return nil
}

func (m *MemoryStore) Ping(_ context.Context) error {
	return nil
}
