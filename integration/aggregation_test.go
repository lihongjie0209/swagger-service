//go:build integration

package integration

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lihongjie0209/swagger-service/internal/app"
	"github.com/lihongjie0209/swagger-service/internal/config"
)

func TestSwaggerAggregationEndToEnd(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/swagger/doc.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"swagger":"2.0","info":{"title":"Identity","version":"1"},"paths":{}}`)
	}))
	t.Cleanup(upstream.Close)
	address := availableAddress(t)
	cfg := config.Config{
		Runtime: config.Runtime{ActiveProfile: "integration"}, App: config.App{Name: "swagger-service", Env: "integration", ShutdownTimeout: 5 * time.Second},
		HTTP: config.HTTP{Address: address, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, RequestTimeout: 5 * time.Second, MaxBodyBytes: 1 << 20},
		GRPC: config.GRPC{Enabled: false}, Log: config.Log{Level: "error", Format: "json"}, Swagger: config.Swagger{Enabled: true},
		Health:      config.Health{DatabaseTimeout: time.Second, RedisTimeout: time.Second},
		Aggregation: config.Aggregation{FetchTimeout: time.Second, CacheTTL: time.Minute, MaxBytes: 1 << 20, Static: []config.SwaggerSource{{Name: "identity-service", Title: "Identity", URL: upstream.URL + "/swagger/doc.json"}}, Kubernetes: config.KubernetesDiscovery{ResyncPeriod: time.Minute}},
		Cron:        config.Cron{Enabled: false, Timezone: "Asia/Shanghai"},
	}
	application := app.New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if err := application.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Stop(context.Background()) })

	for path, content := range map[string]string{"/swagger/index.html": "Platform APIs", "/swagger/services": "identity-service", "/swagger/spec/identity-service": `"title":"Identity"`} {
		response, err := http.Get("http://" + address + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || !strings.Contains(string(body), content) {
			t.Fatalf("GET %s status=%d body=%s", path, response.StatusCode, body)
		}
	}
}

func availableAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	return address
}
