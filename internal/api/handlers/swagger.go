package handlers

import (
	"net/http"
	"os"
)

type SwaggerHandler struct {
	openAPIPath string
	htmlPath    string
}

func NewSwaggerHandler(openAPIPath, htmlPath string) *SwaggerHandler {
	return &SwaggerHandler{
		openAPIPath: openAPIPath,
		htmlPath:    htmlPath,
	}
}

func (h *SwaggerHandler) ServeOpenAPI(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(h.openAPIPath)
	if err != nil {
		http.Error(w, "OpenAPI specification not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (h *SwaggerHandler) ServeUI(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(h.htmlPath)
	if err != nil {
		http.Error(w, "Swagger UI not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
