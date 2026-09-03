package catalog

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lihongjie0209/swagger-service/internal/config"
)

func TestRegistryFetchesCachesAndServesStaleDocument(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		calls++
		if calls > 1 {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"openapi":"3.0.3","info":{"title":"Users","version":"1"}}`)
	}))
	t.Cleanup(server.Close)
	cfg := testConfig()
	cfg.Aggregation.CacheTTL = time.Millisecond
	cfg.Aggregation.Static = []config.SwaggerSource{{Name: "users", URL: server.URL}}
	registry := NewRegistry(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	first, err := registry.SpecAuthorized(context.Background(), "users", "Bearer test-token")
	if err != nil || len(first) == 0 {
		t.Fatalf("first fetch: body=%s err=%v", first, err)
	}
	time.Sleep(2 * time.Millisecond)
	stale, err := registry.SpecAuthorized(context.Background(), "users", "Bearer test-token")
	if err != nil || string(stale) != string(first) || calls != 2 {
		t.Fatalf("stale fallback: calls=%d body=%s err=%v", calls, stale, err)
	}
}

func TestRegistryRejectsNonOpenAPIDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"value":true}`) }))
	t.Cleanup(server.Close)
	cfg := testConfig()
	cfg.Aggregation.Static = []config.SwaggerSource{{Name: "bad", URL: server.URL}}
	registry := NewRegistry(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := registry.Spec(context.Background(), "bad"); err == nil {
		t.Fatal("expected invalid OpenAPI document to fail")
	}
}

func TestRegistrySearchIsFilteredPagedAndOverflowSafe(t *testing.T) {
	t.Parallel()
	registry := NewRegistry(config.Config{Aggregation: config.Aggregation{Static: []config.SwaggerSource{
		{Name: "tenant", Title: "Tenant API"},
		{Name: "identity", Title: "Identity API"},
		{Name: "authorization", Title: "Authorization API"},
	}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	items, total := registry.Search(" API ", 2, 1)
	if total != 3 || len(items) != 1 || items[0].Name != "identity" {
		t.Fatalf("Search() = (%+v, %d)", items, total)
	}
	items, total = registry.Search("tenant", int(^uint(0)>>1), 100)
	if total != 1 || len(items) != 0 {
		t.Fatalf("overflow Search() = (%+v, %d)", items, total)
	}
}

func testConfig() config.Config {
	return config.Config{Aggregation: config.Aggregation{FetchTimeout: time.Second, CacheTTL: time.Minute, MaxBytes: 1 << 20}}
}
