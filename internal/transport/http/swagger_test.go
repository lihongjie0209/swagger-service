package httptransport

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lihongjie0209/swagger-service/internal/catalog"
	"github.com/lihongjie0209/swagger-service/internal/config"
)

func TestSwaggerConsoleAPIUsesEnvelopeAndForwardsAuthorization(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer console-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"openapi":"3.0.3","info":{"title":"Identity","version":"1"}}`)
	}))
	t.Cleanup(upstream.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := catalog.NewRegistry(config.Config{Aggregation: config.Aggregation{
		FetchTimeout: time.Second,
		CacheTTL:     time.Minute,
		MaxBytes:     1 << 20,
		Static:       []config.SwaggerSource{{Name: "identity", Title: "Identity API", URL: upstream.URL}},
	}}, logger)
	handler := NewSwaggerHandler(registry, logger)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/services", handler.ListServices)
	router.POST("/spec", handler.GetSpec)

	services := httptest.NewRecorder()
	router.ServeHTTP(services, httptest.NewRequest(http.MethodPost, "/services", strings.NewReader(`{}`)))
	if services.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", services.Code, services.Body.String())
	}
	var listResponse struct {
		Code int                 `json:"code"`
		Body SwaggerServicesBody `json:"body"`
	}
	if err := json.Unmarshal(services.Body.Bytes(), &listResponse); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listResponse.Code != 0 || len(listResponse.Body.Items) != 1 || listResponse.Body.Items[0].Name != "identity" {
		t.Fatalf("unexpected list response: %+v", listResponse)
	}

	specRequest := httptest.NewRequest(http.MethodPost, "/spec", strings.NewReader(`{"name":" identity "}`))
	specRequest.Header.Set("Content-Type", "application/json")
	specRequest.Header.Set("Authorization", "Bearer console-token")
	spec := httptest.NewRecorder()
	router.ServeHTTP(spec, specRequest)
	if spec.Code != http.StatusOK {
		t.Fatalf("spec status = %d, body = %s", spec.Code, spec.Body.String())
	}
	var specResponse struct {
		Code int             `json:"code"`
		Body SwaggerSpecBody `json:"body"`
	}
	if err := json.Unmarshal(spec.Body.Bytes(), &specResponse); err != nil {
		t.Fatalf("decode spec response: %v", err)
	}
	if specResponse.Code != 0 || specResponse.Body.Name != "identity" || !json.Valid(specResponse.Body.Document) {
		t.Fatalf("unexpected spec response: %+v", specResponse)
	}
}

func TestSwaggerConsoleAPIMapsInvalidAndMissingSources(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := catalog.NewRegistry(config.Config{Aggregation: config.Aggregation{
		FetchTimeout: time.Second,
		CacheTTL:     time.Minute,
		MaxBytes:     1 << 20,
	}}, logger)
	handler := NewSwaggerHandler(registry, logger)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/spec", handler.GetSpec)

	for name, test := range map[string]struct {
		body   string
		status int
	}{
		"invalid": {body: `{"name":" "}`, status: http.StatusBadRequest},
		"missing": {body: `{"name":"missing"}`, status: http.StatusNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/spec", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d, body = %s", response.Code, test.status, response.Body.String())
			}
		})
	}
}
