package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"
)

// Response is the result of one HTTP request.
type Response struct {
	Status   int
	Body     []byte
	Duration time.Duration
	Bytes    int64
	Method   string
	URL      string
}

// Client is one simulated User's HTTP session.
type Client struct {
	base  *url.URL
	http  *http.Client
	token string
}

// NewTransport returns a shared transport for Target App requests.
func NewTransport(insecure bool) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 200
	transport.MaxIdleConnsPerHost = 32
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // testbed self-signed cert
	}
	return transport
}

// New creates a client with its own transport.
func New(target string, insecure bool) (*Client, error) {
	return NewWithTransport(target, NewTransport(insecure))
}

// NewWithTransport creates a client that shares transport.
func NewWithTransport(target string, transport http.RoundTripper) (*Client, error) {
	base, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, fmt.Errorf("unsupported target scheme %q", base.Scheme)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &Client{
		base: base,
		http: &http.Client{
			Timeout:   5 * time.Second,
			Transport: transport,
			Jar:       jar,
		},
	}, nil
}

// SetToken sets the bearer token used for subsequent requests.
func (c *Client) SetToken(token string) {
	c.token = token
}

// Get performs a GET and retries once on transport errors or 5xx responses.
func (c *Client) Get(ctx context.Context, path string) (*Response, error) {
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err == nil && resp.Status < 500 {
		return resp, nil
	}
	if ctx.Err() != nil {
		return resp, err
	}
	select {
	case <-time.After(50 * time.Millisecond):
	case <-ctx.Done():
		return resp, ctx.Err()
	}
	return c.do(ctx, http.MethodGet, path, nil)
}

// Post performs a POST with a JSON body and no retry.
func (c *Client) Post(ctx context.Context, path string, body any) (*Response, error) {
	return c.do(ctx, http.MethodPost, path, body)
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*Response, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.urlFor(path), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "weave-sim/0.1")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	duration := time.Since(start)
	if err != nil {
		return &Response{Method: method, URL: path, Duration: duration}, err
	}
	defer resp.Body.Close() //nolint:errcheck
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return &Response{Status: resp.StatusCode, Method: method, URL: path, Duration: duration}, err
	}
	return &Response{
		Status:   resp.StatusCode,
		Body:     data,
		Duration: duration,
		Bytes:    int64(len(data)),
		Method:   method,
		URL:      path,
	}, nil
}

func (c *Client) urlFor(path string) string {
	u := *c.base
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
