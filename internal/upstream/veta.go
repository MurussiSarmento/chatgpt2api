package upstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Veta IA integration. Veta is reached over the Model Context Protocol using the
// legacy HTTP+SSE transport: a GET opens an event stream that first emits an
// "endpoint" event with the URL to POST JSON-RPC messages to; responses arrive
// back as "message" events on the stream. The API key is sent in the x-api-key
// header (never in the URL) so it does not leak into logs or proxies.
const (
	vetaBaseURL = "https://mcp.vetabot.cloud"
	vetaSSEPath = "/mcp/veta-ia/sse"
)

// VetaClient calls the Veta MCP tools. It is safe for concurrent use; each call
// opens its own short-lived MCP session.
type VetaClient struct {
	token  string
	client *http.Client
}

func NewVetaClient(token string) *VetaClient {
	return &VetaClient{token: token, client: &http.Client{Timeout: 12 * time.Minute}}
}

type sseEvent struct {
	event string
	data  string
}

// callTool runs one full MCP session (connect → initialize → tools/call) and
// returns the tool result object.
func (v *VetaClient) callTool(ctx context.Context, tool string, args map[string]any) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, vetaBaseURL+vetaSSEPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", v.token)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("veta sse connect failed: status %d", resp.StatusCode)
	}

	events := make(chan sseEvent, 32)
	go readSSE(resp.Body, events)

	// First event announces the messages endpoint.
	endpoint, err := waitEndpoint(ctx, events, 20*time.Second)
	if err != nil {
		return nil, err
	}
	messagesURL := vetaBaseURL + endpoint

	if err := v.post(ctx, messagesURL, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "chatgpt2api", "version": "2.0.0"}},
	}); err != nil {
		return nil, err
	}
	if _, err := waitResponse(ctx, events, 1, 30*time.Second); err != nil {
		return nil, err
	}
	if err := v.post(ctx, messagesURL, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		return nil, err
	}
	if err := v.post(ctx, messagesURL, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	}); err != nil {
		return nil, err
	}
	result, err := waitResponse(ctx, events, 2, 10*time.Minute)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (v *VetaClient) post(ctx context.Context, url string, payload any) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", v.token)
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("veta post failed: status %d", resp.StatusCode)
	}
	return nil
}

func readSSE(body io.Reader, out chan<- sseEvent) {
	defer close(out)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var event string
	var data strings.Builder
	flush := func() {
		if data.Len() == 0 && event == "" {
			return
		}
		out <- sseEvent{event: event, data: data.String()}
		event = ""
		data.Reset()
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, ":"):
			// comment / keepalive, ignore
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

func waitEndpoint(ctx context.Context, events <-chan sseEvent, timeout time.Duration) (string, error) {
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return "", errors.New("veta stream closed before endpoint")
			}
			if ev.event == "endpoint" && ev.data != "" {
				return ev.data, nil
			}
		case <-deadline:
			return "", errors.New("veta endpoint handshake timed out")
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func waitResponse(ctx context.Context, events <-chan sseEvent, id float64, timeout time.Duration) (map[string]any, error) {
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return nil, errors.New("veta stream closed before response")
			}
			if ev.data == "" || !strings.HasPrefix(ev.data, "{") {
				continue
			}
			var msg map[string]any
			if json.Unmarshal([]byte(ev.data), &msg) != nil {
				continue
			}
			msgID, ok := msg["id"].(float64)
			if !ok || msgID != id {
				continue
			}
			if errObj, ok := msg["error"]; ok {
				return nil, fmt.Errorf("veta rpc error: %s", compactJSON(errObj))
			}
			result, _ := msg["result"].(map[string]any)
			return result, nil
		case <-deadline:
			return nil, errors.New("veta response timed out")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func compactJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

// VetaMedia is the normalized result of an image or video generation.
type VetaMedia struct {
	URL         string
	DownloadURL string
	Model       string
	MediaType   string
}

// GenerateImage runs veta_generate_image and resolves the final media URL.
func (v *VetaClient) GenerateImage(ctx context.Context, prompt, model, aspectRatio, resolution string) (VetaMedia, error) {
	args := map[string]any{"prompt": prompt}
	putIfSet(args, "model", model)
	putIfSet(args, "aspect_ratio", aspectRatio)
	putIfSet(args, "resolution", resolution)
	return v.run(ctx, "veta_generate_image", args)
}

// GenerateVideo runs veta_generate_video. startImageURL (optional) uses a prior
// image as the opening frame via the "frames" media mode.
func (v *VetaClient) GenerateVideo(ctx context.Context, prompt, model, aspectRatio, quality, duration, startImageURL string) (VetaMedia, error) {
	args := map[string]any{"prompt": prompt}
	putIfSet(args, "model", model)
	putIfSet(args, "aspect_ratio", aspectRatio)
	putIfSet(args, "quality", quality)
	putIfSet(args, "duration", duration)
	if startImageURL != "" {
		args["media_mode"] = "frames"
		args["start_image_url"] = startImageURL
	}
	return v.run(ctx, "veta_generate_video", args)
}

// run calls a generation tool and, if it returns a background job, polls
// veta_check_job until the media URL is ready.
func (v *VetaClient) run(ctx context.Context, tool string, args map[string]any) (VetaMedia, error) {
	result, err := v.callTool(ctx, tool, args)
	if err != nil {
		return VetaMedia{}, err
	}
	media, done, jobID := parseVetaResult(result)
	if done {
		return media, nil
	}
	if jobID == "" {
		return VetaMedia{}, errors.New("veta returned no media and no job id")
	}
	for attempt := 0; attempt < 80; attempt++ {
		select {
		case <-ctx.Done():
			return VetaMedia{}, ctx.Err()
		case <-time.After(8 * time.Second):
		}
		check, err := v.callTool(ctx, "veta_check_job", map[string]any{"job_id": jobID})
		if err != nil {
			return VetaMedia{}, err
		}
		// Only trust a check that refers to our job (check_job can echo an
		// unrelated recent job otherwise).
		checkStruct, _ := check["structuredContent"].(map[string]any)
		if checkID := structString(checkStruct, "job_id"); checkID != "" && checkID != jobID {
			continue
		}
		media, done, _ = parseVetaResult(check)
		if done {
			return media, nil
		}
	}
	return VetaMedia{}, errors.New("veta generation timed out while polling")
}

// parseVetaResult pulls the media URL / job id out of a tool result's
// structuredContent. done is true once a final media URL is present.
func parseVetaResult(result map[string]any) (VetaMedia, bool, string) {
	structured, _ := result["structuredContent"].(map[string]any)
	if structured == nil {
		structured = map[string]any{}
	}
	media := VetaMedia{
		URL:         firstString(structured, "image_url", "video_url", "media_url", "url"),
		DownloadURL: firstString(structured, "download_url"),
		Model:       firstString(structured, "model"),
		MediaType:   firstString(structured, "media_type"),
	}
	jobID := structString(structured, "job_id")
	status := strings.ToLower(firstString(structured, "status"))
	// Fallback: field names vary across image/video, so scan the whole result
	// for a media URL if the known fields were empty.
	if media.URL == "" {
		media.URL = mediaURLPattern.FindString(compactJSON(result))
	}
	done := media.URL != "" && status != "processing" && status != "pending" && status != "queued"
	return media, done, jobID
}

var mediaURLPattern = regexp.MustCompile(`https?://[^"'\s\\]+\.(?:mp4|mov|webm|jpg|jpeg|png|webp)`)

func putIfSet(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func structString(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return value
}
