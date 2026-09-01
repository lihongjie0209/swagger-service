package httptransport

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lihongjie0209/swagger-service/internal/apperror"
	"github.com/lihongjie0209/swagger-service/internal/catalog"
)

type SwaggerHandler struct {
	registry *catalog.Registry
	logger   *slog.Logger
}

func NewSwaggerHandler(registry *catalog.Registry, logger *slog.Logger) *SwaggerHandler {
	return &SwaggerHandler{registry: registry, logger: logger}
}

type ListSwaggerServicesRequest struct{}

type SwaggerServicesBody struct {
	Items []catalog.Source `json:"items"`
}

type GetSwaggerSpecRequest struct {
	Name string `json:"name" binding:"required" example:"platform--identity-service"`
}

type SwaggerSpecBody struct {
	Name     string         `json:"name"`
	Document map[string]any `json:"document"`
}

func (h *SwaggerHandler) Index(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerIndex))
}

func (h *SwaggerHandler) Services(c *gin.Context) { c.JSON(http.StatusOK, h.registry.List()) }

// ListServices godoc
// @Summary List discovered OpenAPI services
// @Tags OpenAPI aggregation
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body ListSwaggerServicesRequest true "Empty request"
// @Success 200 {object} Response{body=SwaggerServicesBody}
// @Failure 401 {object} Response "Code 20001: unauthorized"
// @Failure 403 {object} Response "Code 20003: forbidden"
// @Router /api/v1/swagger/services [post]
func (h *SwaggerHandler) ListServices(c *gin.Context) {
	OK(c, SwaggerServicesBody{Items: h.registry.List()})
}

func (h *SwaggerHandler) Spec(c *gin.Context) {
	body, err := h.registry.SpecAuthorized(c.Request.Context(), strings.TrimSpace(c.Param("name")), c.GetHeader("Authorization"))
	if errors.Is(err, catalog.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": "OpenAPI source not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "OpenAPI source unavailable"})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// GetSpec godoc
// @Summary Get one aggregated OpenAPI document
// @Tags OpenAPI aggregation
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body GetSwaggerSpecRequest true "Service source name"
// @Success 200 {object} Response{body=SwaggerSpecBody}
// @Failure 400 {object} Response "Code 10001: invalid request"
// @Failure 401 {object} Response "Code 20001: unauthorized"
// @Failure 403 {object} Response "Code 20003: forbidden"
// @Failure 404 {object} Response "Code 10004: source not found"
// @Failure 503 {object} Response "Code 50003: source unavailable"
// @Router /api/v1/swagger/spec [post]
func (h *SwaggerHandler) GetSpec(c *gin.Context) {
	var request GetSwaggerSpecRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, h.logger, apperror.Invalid("invalid request", err))
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		Fail(c, h.logger, apperror.Invalid("service name is required", nil))
		return
	}
	body, err := h.registry.SpecAuthorized(c.Request.Context(), request.Name, c.GetHeader("Authorization"))
	if errors.Is(err, catalog.ErrNotFound) {
		Fail(c, h.logger, apperror.NotFound("OpenAPI source not found"))
		return
	}
	if err != nil {
		Fail(c, h.logger, apperror.Unavailable("OpenAPI source unavailable", err))
		return
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		Fail(c, h.logger, apperror.Unavailable("OpenAPI source unavailable", err))
		return
	}
	OK(c, SwaggerSpecBody{Name: request.Name, Document: document})
}

const swaggerIndex = "<!doctype html><html><head><meta charset=\"utf-8\"><title>Platform APIs</title>" +
	"<link rel=\"stylesheet\" href=\"/swagger-assets/swagger-ui.css\"></head><body><div id=\"swagger-ui\"></div>" +
	"<script src=\"/swagger-assets/swagger-ui-bundle.js\"></script><script src=\"/swagger-assets/swagger-ui-standalone-preset.js\"></script><script>" +
	"var token=localStorage.getItem('platform_swagger_token')||'';function headers(){return token?{Authorization:'Bearer '+token}:{}};" +
	"function load(){fetch('/swagger/services',{headers:headers()}).then(function(r){if(r.status===401){token=prompt('JWT Access Token')||'';localStorage.setItem('platform_swagger_token',token);return load()}return r.json()}).then(function(s){if(!s)return;SwaggerUIBundle({dom_id:'#swagger-ui',deepLinking:true," +
	"urls:s.map(function(x){return {name:x.title,url:'/swagger/spec/'+encodeURIComponent(x.name)}}),presets:[SwaggerUIBundle.presets.apis,SwaggerUIStandalonePreset]," +
	"layout:'StandaloneLayout',persistAuthorization:true,requestInterceptor:function(r){if(token)r.headers.Authorization='Bearer '+token;return r}});});}load();</script></body></html>"
