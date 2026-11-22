package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/basketikun/chatgpt2api/internal/upstream"
	"github.com/gin-gonic/gin"
)

// VetaHandler exposes Veta IA video generation. It is only registered when a
// VETA_TOKEN is configured, so it adds no surface when Veta is disabled.
type VetaHandler struct {
	client *upstream.VetaClient
}

func NewVetaHandler(token string) *VetaHandler {
	return &VetaHandler{client: upstream.NewVetaClient(token)}
}

func (h *VetaHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.POST("/videos/generations", h.videos)
}

func (h *VetaHandler) videos(c *gin.Context) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	prompt, _ := body["prompt"].(string)
	if prompt == "" {
		writeError(c, http.StatusBadRequest, errors.New("prompt is required"))
		return
	}
	model, _ := body["model"].(string)
	if model == "" || model == "auto" {
		model = "veo_3_1"
	}
	aspect, _ := body["aspect_ratio"].(string)
	if aspect == "" {
		aspect = aspectFromSize(stringField(body, "size"))
	}
	quality, _ := body["quality"].(string)
	duration := durationField(body)
	startImage := firstStringField(body, "image_url", "image", "start_image_url")

	media, err := h.client.GenerateVideo(c.Request.Context(), prompt, model, aspect, quality, duration, startImage)
	if err != nil {
		writeError(c, http.StatusBadGateway, err)
		return
	}
	item := gin.H{"url": media.URL}
	if media.DownloadURL != "" {
		item["download_url"] = media.DownloadURL
	}
	c.JSON(http.StatusOK, gin.H{
		"created": time.Now().Unix(),
		"model":   firstNonEmpty(media.Model, model),
		"data":    []any{item},
	})
}

func durationField(body map[string]any) string {
	switch value := body["duration"].(type) {
	case string:
		return value
	case float64:
		return strconv.Itoa(int(value))
	}
	if value, ok := body["seconds"].(float64); ok {
		return strconv.Itoa(int(value))
	}
	return ""
}

func stringField(body map[string]any, key string) string {
	value, _ := body[key].(string)
	return value
}

func firstStringField(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := body[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// aspectFromSize maps an OpenAI-style WxH size to Veta's aspect ratio enum.
func aspectFromSize(size string) string {
	switch size {
	case "1024x1792", "1080x1920", "720x1280":
		return "9:16"
	case "1024x1024":
		return "1:1"
	default:
		return "16:9"
	}
}
