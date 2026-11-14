package history

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

// ArgoClient reads one Argo CD Application's desired manifests and live
// resource status with a read-only token.
type ArgoClient struct {
	base      string
	token     string
	app       string
	namespace string
	client    *http.Client
}

// DeclaredWorkload is one workload in the Argo desired state.
type DeclaredWorkload struct {
	Name string
	Kind string
	Live bool
}

func NewArgoClient(base, token, app, namespace string) *ArgoClient {
	return &ArgoClient{
		base:      strings.TrimRight(base, "/"),
		token:     token,
		app:       app,
		namespace: namespace,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *ArgoClient) Workloads(ctx context.Context) ([]DeclaredWorkload, error) {
	manifests, err := c.manifests(ctx)
	if err != nil {
		return nil, err
	}
	live, liveKnown, err := c.liveResources(ctx)
	if err != nil {
		return nil, err
	}

	workloads := make(map[string]DeclaredWorkload)
	for _, manifest := range manifests {
		var meta struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name      string `yaml:"name"`
				Namespace string `yaml:"namespace"`
			} `yaml:"metadata"`
		}
		if err := yaml.Unmarshal([]byte(manifest), &meta); err != nil {
			continue
		}
		switch meta.Kind {
		case "Deployment", "StatefulSet", "DaemonSet":
		default:
			continue
		}
		if meta.Metadata.Name == "" {
			continue
		}
		namespace := meta.Metadata.Namespace
		if namespace == "" {
			namespace = c.namespace
		}
		if namespace != c.namespace {
			continue
		}
		key := meta.Kind + "/" + meta.Metadata.Name
		workload := DeclaredWorkload{Name: meta.Metadata.Name, Kind: meta.Kind, Live: true}
		if liveKnown {
			workload.Live = live[key]
		}
		workloads[key] = workload
	}

	result := make([]DeclaredWorkload, 0, len(workloads))
	for _, workload := range workloads {
		result = append(result, workload)
	}
	return result, nil
}

func (c *ArgoClient) manifests(ctx context.Context) ([]string, error) {
	var response struct {
		Manifests []string `json:"manifests"`
	}
	if err := c.getJSON(ctx, "/api/v1/applications/"+url.PathEscape(c.app)+"/manifests", &response); err != nil {
		return nil, err
	}
	return response.Manifests, nil
}

func (c *ArgoClient) liveResources(ctx context.Context) (map[string]bool, bool, error) {
	var response struct {
		Status struct {
			Resources []struct {
				Kind      string `json:"kind"`
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
				Health    *struct {
					Status string `json:"status"`
				} `json:"health"`
			} `json:"resources"`
		} `json:"status"`
	}
	if err := c.getJSON(ctx, "/api/v1/applications/"+url.PathEscape(c.app), &response); err != nil {
		return nil, false, err
	}
	known := len(response.Status.Resources) > 0
	result := make(map[string]bool, len(response.Status.Resources))
	for _, resource := range response.Status.Resources {
		if resource.Namespace != "" && resource.Namespace != c.namespace {
			continue
		}
		health := ""
		if resource.Health != nil {
			health = resource.Health.Status
		}
		result[resource.Kind+"/"+resource.Name] = health != "Missing"
	}
	return result, known, nil
}

func (c *ArgoClient) getJSON(ctx context.Context, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("argo %s: %s: %s", path, response.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode argo response: %w", err)
	}
	return nil
}
