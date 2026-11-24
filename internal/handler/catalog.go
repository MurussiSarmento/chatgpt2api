package handler

import (
	"net/http"
	"os"
	"strconv"

	"github.com/basketikun/chatgpt2api/internal/config"
	"github.com/basketikun/chatgpt2api/internal/service"
	upstream "github.com/basketikun/chatgpt2api/internal/upstream"
	"github.com/gin-gonic/gin"
)

type CatalogHandler struct {
	service *service.CatalogService
	config  *config.Config
}

func NewCatalogHandler(service *service.CatalogService, config *config.Config) *CatalogHandler {
	return &CatalogHandler{service: service, config: config}
}

func (h *CatalogHandler) RegisterRoutes(router *gin.RouterGroup) { router.GET("/models", h.list) }

func (h *CatalogHandler) list(c *gin.Context) {
	refresh, _ := strconv.ParseBool(c.Query("refresh"))
	ids, err := h.service.List(c.Request.Context(), refresh)
	if err != nil {
		writeError(c, http.StatusBadGateway, err)
		return
	}
	data := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		item := gin.H{"id": id, "object": "model", "created": 0, "owned_by": "chatgpt2api", "permission": []any{}, "root": id, "parent": nil}
		if id == h.config.ImageModelName {
			item["type"] = "image"
		}
		data = append(data, item)
	}
	// Advertise Veta IA image models when a token is configured so the
	// frontend can offer them in the image-generation picker.
	if os.Getenv("VETA_TOKEN") != "" {
		for _, model := range []string{"nano_banana_pro", "nano_banana_fast"} {
			data = append(data, gin.H{"id": model, "object": "model", "created": 0, "owned_by": "veta-ia", "permission": []any{}, "root": model, "parent": nil, "type": "image"})
		}
	}
	// Advertise Artistly image models when an API key is configured.
	// Models map to AI Image Designer v6 categories per the mapping document.
	if os.Getenv("ARTISTLY_EMAIL") != "" {
		for _, model := range upstream.ArtistlyImageModels() {
			data = append(data, gin.H{"id": model, "object": "model", "created": 0, "owned_by": "artistly", "permission": []any{}, "root": model, "parent": nil, "type": "image"})
		}
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}
