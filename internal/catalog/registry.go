package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lihongjie0209/swagger-service/internal/config"
)

type Source struct {
	Name      string    `json:"name"`
	Title     string    `json:"title"`
	URL       string    `json:"-"`
	Origin    string    `json:"origin"`
	Available bool      `json:"available"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type cachedSpec struct {
	body      []byte
	expiresAt time.Time
	updatedAt time.Time
}

type Registry struct {
	mu       sync.RWMutex
	sources  map[string]Source
	cache    map[string]cachedSpec
	client   *http.Client
	cacheTTL time.Duration
	maxBytes int64
	logger   *slog.Logger
}

func NewRegistry(cfg config.Config, logger *slog.Logger) *Registry {
	r := &Registry{sources: make(map[string]Source), cache: make(map[string]cachedSpec), client: &http.Client{Timeout: cfg.Aggregation.FetchTimeout}, cacheTTL: cfg.Aggregation.CacheTTL, maxBytes: cfg.Aggregation.MaxBytes, logger: logger}
	for _, source := range cfg.Aggregation.Static {
		title := strings.TrimSpace(source.Title)
		if title == "" {
			title = source.Name
		}
		r.sources[source.Name] = Source{Name: source.Name, Title: title, URL: source.URL, Origin: "static"}
	}
	return r
}

func (r *Registry) ReplaceKubernetes(values []Source) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, source := range r.sources {
		if source.Origin == "kubernetes" {
			delete(r.sources, name)
			delete(r.cache, name)
		}
	}
	for _, source := range values {
		source.Origin = "kubernetes"
		r.sources[source.Name] = source
	}
}

func (r *Registry) List() []Source {
	r.mu.RLock()
	defer r.mu.RUnlock()
	values := make([]Source, 0, len(r.sources))
	for _, source := range r.sources {
		if cached, ok := r.cache[source.Name]; ok {
			source.Available = true
			source.UpdatedAt = cached.updatedAt
		}
		values = append(values, source)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	return values
}

func (r *Registry) Spec(ctx context.Context, name string) ([]byte, error) {
	return r.SpecAuthorized(ctx, name, "")
}

func (r *Registry) SpecAuthorized(ctx context.Context, name, authorization string) ([]byte, error) {
	r.mu.RLock()
	source, exists := r.sources[name]
	cached, hasCached := r.cache[name]
	r.mu.RUnlock()
	if !exists {
		return nil, ErrNotFound
	}
	if hasCached && time.Now().Before(cached.expiresAt) {
		return append([]byte(nil), cached.body...), nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return r.staleOrError(name, cached, hasCached, err)
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return r.staleOrError(name, cached, hasCached, fmt.Errorf("fetch OpenAPI document: %w", err))
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			r.logger.Warn("close upstream OpenAPI response", "service", name, "error", closeErr)
		}
	}()
	if response.StatusCode != http.StatusOK {
		return r.staleOrError(name, cached, hasCached, fmt.Errorf("fetch OpenAPI document: status %d", response.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, r.maxBytes+1))
	if err != nil {
		return r.staleOrError(name, cached, hasCached, fmt.Errorf("read OpenAPI document: %w", err))
	}
	if int64(len(body)) > r.maxBytes {
		return r.staleOrError(name, cached, hasCached, errors.New("OpenAPI document exceeds configured maximum size"))
	}
	var header struct {
		OpenAPI string `json:"openapi"`
		Swagger string `json:"swagger"`
	}
	if err := json.Unmarshal(body, &header); err != nil || (header.OpenAPI == "" && header.Swagger == "") {
		return r.staleOrError(name, cached, hasCached, errors.New("upstream response is not an OpenAPI document"))
	}
	now := time.Now()
	r.mu.Lock()
	r.cache[name] = cachedSpec{body: append([]byte(nil), body...), expiresAt: now.Add(r.cacheTTL), updatedAt: now}
	r.mu.Unlock()
	return body, nil
}

func (r *Registry) staleOrError(name string, cached cachedSpec, hasCached bool, err error) ([]byte, error) {
	if hasCached {
		r.logger.Warn("serving stale OpenAPI document", "service", name, "error", err)
		return append([]byte(nil), cached.body...), nil
	}
	return nil, err
}

var ErrNotFound = errors.New("OpenAPI source not found")
