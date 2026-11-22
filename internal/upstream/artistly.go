package upstream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Artistly integration via MCP-over-SSE. The mapping document
// (artistly_mcp_mapeamento.md) confirms an API key exists but no official REST
// contract has been published yet. This client implements the proposed tool
// names from that document as stubs that return structured errors until real
// endpoints are confirmed by Artistly support. Replace the stub bodies with
// actual callTool invocations once the API contract is available.
const (
	artistlyBaseURL = "https://mcp.artistly.ai"
	artistlySSEPath = "/mcp/sse"
)

// ArtistlyClient calls Artistly MCP tools. Safe for concurrent use; each call
// opens its own short-lived MCP session.
type ArtistlyClient struct {
	token string
}

// NewArtistlyClient creates a client ready to call Artistly MCP tools.
func NewArtistlyClient(token string) *ArtistlyClient {
	return &ArtistlyClient{token: token}
}

// ArtistlyMedia is the normalized result of an image generation.
type ArtistlyMedia struct {
	URL         string
	DownloadURL string
	Model       string
	MediaType   string
	JobID       string
	Status      string
}

// GenerateImage calls artistly_image_generate (stub). Returns a descriptive
// error until the real API contract is confirmed. The signature mirrors the
// proposed tool inputs from the mapping document so callers do not need to
// change when the implementation lands.
func (a *ArtistlyClient) GenerateImage(ctx context.Context, prompt, category, quality, aspectRatio string, count int, folder, visibility string) (ArtistlyMedia, error) {
	if a.token == "" {
		return ArtistlyMedia{}, errors.New("artistly: API key not configured")
	}
	// TODO: replace with real callTool("artistly_image_generate", args) once
	// the API contract is confirmed. The mapping document lists these params:
	// category, prompt, quality, aspect_ratio, count, folder, visibility.
	// The SSE+JSON-RPC handshake will mirror VetaClient.callTool exactly.
	_ = ctx
	_ = prompt
	_ = category
	_ = quality
	_ = aspectRatio
	_ = count
	_ = folder
	_ = visibility
	return ArtistlyMedia{}, fmt.Errorf("artistly: image generation not yet available — awaiting API contract from support@artistly.ai (see artistly_mcp_mapeamento.md §8)")
}

// CheckJob polls artistly_job_get (stub). Mirrors the proposed polling
// pattern from the mapping document §5.
func (a *ArtistlyClient) CheckJob(ctx context.Context, jobID string) (ArtistlyMedia, bool, error) {
	if a.token == "" {
		return ArtistlyMedia{}, false, errors.New("artistly: API key not configured")
	}
	// TODO: replace with real callTool("artistly_job_get", {"job_id": jobID})
	_ = ctx
	_ = jobID
	return ArtistlyMedia{}, false, fmt.Errorf("artistly: job polling not yet available — awaiting API contract")
}

// ConnectionStatus checks artistly_connection_status (stub). Useful for the
// frontend to show whether the configured key is valid.
func (a *ArtistlyClient) ConnectionStatus(ctx context.Context) (map[string]any, error) {
	if a.token == "" {
		return map[string]any{"connected": false, "error": "API key not configured"}, nil
	}
	// TODO: replace with real callTool("artistly_connection_status", {})
	_ = ctx
	return map[string]any{
		"connected": false,
		"status":    "stub",
		"message":   "Artistly MCP client is scaffolded but not connected. Awaiting API contract from support@artistly.ai.",
	}, nil
}

// ArtistlyImageModels returns the model identifiers this client advertises.
// These map to Artistly's AI Image Designer v6 categories and are used by the
// catalog handler and the ai.go router. When the real API supports multiple
// backends, expand this list accordingly.
func ArtistlyImageModels() []string {
	return []string{
		"artistly_v6",
		"artistly_photorealism",
		"artistly_artistic",
		"artistly_logo",
		"artistly_pattern",
	}
}

// ArtistlyAspectRatios returns the ratios documented in the mapping §AI Image
// Designer v6. Used to validate incoming requests before sending upstream.
func ArtistlyAspectRatios() map[string]string {
	return map[string]string{
		"1:1":  "1024x1024",
		"16:9": "1344x768",
		"21:9": "1536x640",
		"3:2":  "1216x832",
		"2:3":  "832x1216",
		"4:5":  "896x1088",
		"5:4":  "1088x896",
		"3:4":  "896x1152",
		"4:3":  "1152x896",
		"9:16": "768x1344",
		"9:21": "640x1536",
	}
}

// IsArtistlyModel reports whether a model name routes to Artistly.
func IsArtistlyModel(model string) bool {
	model = strings.ToLower(model)
	for _, m := range ArtistlyImageModels() {
		if model == m {
			return true
		}
	}
	return false
}

// ArtistlyPollTimeout is the maximum time to wait for an async Artistly job.
// Matches the mapping document's recommendation of long-running tasks (15–30
// min for storybooks; images should be faster). Adjust when real SLAs are known.
const ArtistlyPollTimeout = 10 * time.Minute