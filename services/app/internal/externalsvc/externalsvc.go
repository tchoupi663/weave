package externalsvc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/tchoupi663/weave/services/app/internal/httpx"
	"github.com/tchoupi663/weave/services/app/internal/ids"
)

// Service is the dummy External Payment Provider.
type Service struct{}

// New returns an External Payment Provider service.
func New() *Service {
	return &Service{}
}

// Handler returns the External Payment Provider HTTP handler.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /charge", s.handleCharge)
	mux.HandleFunc("GET /health", httpx.Health)
	return mux
}

type chargeRequest struct {
	OrderID string  `json:"orderId"`
	Amount  float64 `json:"amount"`
	Card    string  `json:"card"`
}

func (s *Service) handleCharge(w http.ResponseWriter, r *http.Request) {
	var req chargeRequest
	if !httpx.ReadJSON(w, r, &req) {
		return
	}
	if req.OrderID == "" || req.Amount <= 0 || req.Card == "" {
		httpx.Error(w, http.StatusBadRequest, "orderId, a positive amount, and card are required")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{
		"chargeId": ids.New(),
		"status":   "succeeded",
	})
}

// SelfSignedTLSConfig builds a TLS config for the dummy external provider.
func SelfSignedTLSConfig(hosts ...string) (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "external"},
		DNSNames:     hosts,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
