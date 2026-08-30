package httptransport

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lihongjie0209/swagger-service/internal/catalog"
)

type SwaggerHandler struct{ registry *catalog.Registry }

func NewSwaggerHandler(registry *catalog.Registry) *SwaggerHandler {
	return &SwaggerHandler{registry: registry}
}

func (h *SwaggerHandler) Index(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerIndex))
}

func (h *SwaggerHandler) Services(c *gin.Context) { c.JSON(http.StatusOK, h.registry.List()) }

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

const swaggerIndex = "<!doctype html><html><head><meta charset=\"utf-8\"><title>Platform APIs</title>" +
	"<link rel=\"stylesheet\" href=\"/swagger-assets/swagger-ui.css\"></head><body><div id=\"swagger-ui\"></div>" +
	"<script src=\"/swagger-assets/swagger-ui-bundle.js\"></script><script src=\"/swagger-assets/swagger-ui-standalone-preset.js\"></script><script>" +
	"var token=localStorage.getItem('platform_swagger_token')||'';function headers(){return token?{Authorization:'Bearer '+token}:{}};" +
	"function load(){fetch('/swagger/services',{headers:headers()}).then(function(r){if(r.status===401){token=prompt('JWT Access Token')||'';localStorage.setItem('platform_swagger_token',token);return load()}return r.json()}).then(function(s){if(!s)return;SwaggerUIBundle({dom_id:'#swagger-ui',deepLinking:true," +
	"urls:s.map(function(x){return {name:x.title,url:'/swagger/spec/'+encodeURIComponent(x.name)}}),presets:[SwaggerUIBundle.presets.apis,SwaggerUIStandalonePreset]," +
	"layout:'StandaloneLayout',persistAuthorization:true,requestInterceptor:function(r){if(token)r.headers.Authorization='Bearer '+token;return r}});});}load();</script></body></html>"
