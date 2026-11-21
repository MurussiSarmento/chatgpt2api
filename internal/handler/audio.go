package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/basketikun/chatgpt2api/internal/service"
	"github.com/gin-gonic/gin"
)

// AudioHandler provides OpenAI-compatible audio endpoints:
//
//	POST /v1/audio/speech          text-to-speech (ChatGPT web voice model)
//	POST /v1/audio/transcriptions  speech-to-text
type AudioHandler struct {
	service *service.AudioService
	logs    *service.LogService
}

func NewAudioHandler(audio *service.AudioService, logs *service.LogService) *AudioHandler {
	return &AudioHandler{service: audio, logs: logs}
}

func (h *AudioHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.POST("/audio/speech", h.speech)
	router.POST("/audio/transcriptions", h.transcriptions)
}

func (h *AudioHandler) speech(c *gin.Context) {
	var body struct {
		Model          string `json:"model"`
		Input          string `json:"input"`
		Voice          string `json:"voice"`
		ResponseFormat string `json:"response_format"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, http.StatusBadRequest, errors.New("request body is not valid JSON"))
		return
	}
	if strings.TrimSpace(body.Input) == "" {
		writeError(c, http.StatusBadRequest, errors.New("input is required"))
		return
	}
	if body.Model == "" {
		body.Model = "gpt-tts"
	}
	callLog := h.logs.NewCallLog(identity(c), c.Request.URL.Path, body.Model, "Speech synthesis", body.Input, time.Now())
	data, contentType, err := h.service.Speech(c.Request.Context(), service.SpeechRequest{Input: body.Input, Voice: body.Voice, Model: body.Model, Format: body.ResponseFormat}, callLog)
	if err != nil {
		callLog.Save("Speech synthesis failed", "failed", map[string]any{"error": err.Error()})
		writeError(c, http.StatusBadGateway, err)
		return
	}
	callLog.Save("Speech synthesis complete", "success", map[string]any{"bytes": len(data), "content_type": contentType})
	c.Data(http.StatusOK, contentType, data)
}

func (h *AudioHandler) transcriptions(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		writeError(c, http.StatusBadRequest, errors.New("file is required"))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	if len(data) == 0 {
		writeError(c, http.StatusBadRequest, errors.New("the uploaded audio is empty"))
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	callLog := h.logs.NewCallLog(identity(c), c.Request.URL.Path, c.DefaultPostForm("model", "gpt-transcribe"), "Transcription", header.Filename, time.Now())
	text, err := h.service.Transcribe(c.Request.Context(), data, header.Filename, contentType, callLog)
	if err != nil {
		callLog.Save("Transcription failed", "failed", map[string]any{"error": err.Error()})
		writeError(c, http.StatusBadGateway, err)
		return
	}
	callLog.Save("Transcription complete", "success", map[string]any{"chars": len([]rune(text))})
	if c.PostForm("response_format") == "text" {
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(text))
		return
	}
	c.JSON(http.StatusOK, gin.H{"text": text})
}
