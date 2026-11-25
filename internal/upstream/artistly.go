package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	artistlyBaseURL    = "https://app.artistly.ai"
	artistlyLoginPath  = "/login"
	artistlyStorePath  = "/ai/image-designer-v6/store"
	artistlyUserAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

// ArtistlyClient authenticates via email/password session (Laravel Inertia.js)
// and generates images by POSTing to the AI Image Designer v6 store endpoint.
// Safe for concurrent use; sessions are shared and re-authenticated on expiry.
type ArtistlyClient struct {
	email    string
	password string

	mu       sync.Mutex
	cookies  []*http.Cookie
	xsrf     string
	version  string
	loggedIn bool
	client   *http.Client
}

// NewArtistlyClient creates a client that logs in with email/password.
// The token parameter is ignored; credentials come from ARTISTLY_EMAIL and
// ARTISTLY_PASSWORD env vars (passed by the service layer).
func NewArtistlyClient(_ string) *ArtistlyClient {
	jar, _ := cookiejar.New(nil)
	return &ArtistlyClient{
		client: &http.Client{
			Timeout: 5 * time.Minute,
			Jar:     jar,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("too many redirects")
				}
				return nil
			},
		},
	}
}

// SetCredentials configures the login credentials. Called by the service layer
// after reading env vars.
func (a *ArtistlyClient) SetCredentials(email, password string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.email = email
	a.password = password
	a.loggedIn = false
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

// ensureSession logs in if needed and returns fresh cookies + XSRF token.
func (a *ArtistlyClient) ensureSession(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.loggedIn && a.xsrf != "" {
		return nil
	}
	if a.email == "" || a.password == "" {
		return errors.New("artistly: credentials not configured (set ARTISTLY_EMAIL and ARTISTLY_PASSWORD)")
	}

	// Step 1: GET /login to obtain session cookie + XSRF token + Inertia version
	loginPage, err := a.doRequest(ctx, http.MethodGet, artistlyBaseURL+artistlyLoginPath, nil, nil)
	if err != nil {
		return fmt.Errorf("artistly: login page fetch failed: %w", err)
	}
	defer loginPage.Body.Close()

	body, err := io.ReadAll(loginPage.Body)
	if err != nil {
		return fmt.Errorf("artistly: login page read failed: %w", err)
	}

	// Extract XSRF token from cookies. The Set-Cookie value is URL-encoded
	// (ends in %3D); Laravel's VerifyCsrfToken expects the decoded form in
	// the X-XSRF-TOKEN header, so decode it once here.
	var xsrf string
	for _, c := range loginPage.Cookies() {
		if c.Name == "XSRF-TOKEN" {
			xsrf, _ = url.QueryUnescape(c.Value)
		}
	}
	if xsrf == "" {
		return errors.New("artistly: XSRF-TOKEN cookie not found on login page")
	}

	// Extract Inertia version from data-page attribute
	version := extractInertiaVersion(string(body))

	// Store cookies from login page
	a.cookies = loginPage.Cookies()
	a.xsrf = xsrf
	a.version = version

	// Step 2: POST /login with credentials as JSON (Laravel Inertia frontend
	// serialises the form before POSTing).
	loginBody, _ := json.Marshal(map[string]any{
		"email":    a.email,
		"password": a.password,
		"remember": false,
	})

	headers := map[string]string{
		"X-XSRF-TOKEN":     a.xsrf,
		"Content-Type":     "application/json",
		"Accept":           "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          artistlyBaseURL + artistlyLoginPath,
		"Origin":           artistlyBaseURL,
	}

	resp, err := a.doRequestWithCookies(ctx, http.MethodPost, artistlyBaseURL+artistlyLoginPath, bytes.NewReader(loginBody), headers)
	if err != nil {
		return fmt.Errorf("artistly: login POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnprocessableEntity {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("artistly: login failed (422): %s", string(respBody))
	}
	if resp.StatusCode == http.StatusConflict {
		// 409 = Inertia version mismatch; clear session and re-fetch login page
		a.mu.Lock()
		a.loggedIn = false
		a.xsrf = ""
		a.version = ""
		a.cookies = nil
		a.mu.Unlock()
		return fmt.Errorf("artistly: login failed (409) - version mismatch, will retry")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("artistly: login failed with status %d", resp.StatusCode)
	}

	// Update cookies from login response (session may have changed). Laravel
	// rotates the XSRF-TOKEN on login, so refresh the stored token from the
	// response — sending the pre-login token with the new session yields 419.
	for _, c := range resp.Cookies() {
		a.setCookie(c)
		if c.Name == "XSRF-TOKEN" {
			if decoded, err := url.QueryUnescape(c.Value); err == nil && decoded != "" {
				a.xsrf = decoded
			}
		}
	}

	// Verify authentication by checking redirect or response
	if resp.StatusCode == http.StatusOK {
		// Check if we got redirected to dashboard (authenticated)
		respBody, _ := io.ReadAll(resp.Body)
		component := extractInertiaComponent(string(respBody))
		if component == "Auth/Login" {
			return errors.New("artistly: login returned login page - credentials may be wrong or MFA required")
		}
	}

	a.loggedIn = true
	return nil
}

// GenerateImage submits an image generation request to Artistly's AI Image
// Designer v6 store endpoint. The category maps to the tool name (e.g.
// "AI Image Designer v6"). Quality is "fast" or "high". Aspect ratio uses
// the format "1:1", "16:9", etc. Count is the number of images (1-4).
//
// After a successful submission, Artistly redirects (302) to /personal-designs.
// The generated image is NOT inline in that page — it is loaded asynchronously
// via the internal designs API. We follow the redirect, then poll the designs
// API until the newest design appears with a CDN image URL.
func (a *ArtistlyClient) GenerateImage(ctx context.Context, prompt, category, quality, aspectRatio string, count int, folder, visibility string) (ArtistlyMedia, error) {
	// Retry once on session expiry (419/409/401)
	for attempt := 0; attempt < 2; attempt++ {
		media, err := a.generateImageOnce(ctx, prompt, category, quality, aspectRatio, count, folder, visibility)
		if err == nil {
			return media, nil
		}
		// Only retry on session-related errors
		errMsg := err.Error()
		if strings.Contains(errMsg, "session expired") || strings.Contains(errMsg, "status 419") || strings.Contains(errMsg, "status 409") || strings.Contains(errMsg, "status 401") {
			a.mu.Lock()
			a.loggedIn = false
			a.xsrf = ""
			a.version = ""
			a.cookies = nil
			a.mu.Unlock()
			continue
		}
		return ArtistlyMedia{}, err
	}
	return ArtistlyMedia{}, errors.New("artistly: session expired after retry")
}

// generateImageOnce performs one attempt at image generation + polling.
func (a *ArtistlyClient) generateImageOnce(ctx context.Context, prompt, category, quality, aspectRatio string, count int, folder, visibility string) (ArtistlyMedia, error) {
	if err := a.ensureSession(ctx); err != nil {
		return ArtistlyMedia{}, err
	}

	// Parse aspect ratio into width/height
	width, height := artistlyAspectRatioToSize(aspectRatio)

	if category == "" {
		category = "AI Image Designer v6"
	}
	if quality == "" {
		quality = "fast"
	}
	if count < 1 {
		count = 1
	} else if count > 4 {
		count = 4
	}

	payload := map[string]any{
		"image_option":          "upload",
		"image_selected":        "no",
		"prompt":                prompt,
		"negative_prompt":       "",
		"tool_used":             "AI Image Designer v6",
		"seed":                  0,
		"aspect_ratio":          aspectRatio,
		"width":                 width,
		"height":                height,
		"quality":               quality,
		"quantity":              count,
		"category":              category,
		"theme":                 "",
		"image_url":             "",
		"image_base64":          "",
		"personal_design":       "",
		"denoise_strength":      "0.75",
		"folder_id":             nil,
		"prefix":                "",
		"suffix":                "",
		"lora":                  nil,
		"checkpoint":            category,
		"should_enhance_prompt": false,
		"seasonal_style":        "",
	}
	if folder != "" {
		payload["folder_id"] = folder
	}

	body, _ := json.Marshal(payload)

	a.mu.Lock()
	headers := map[string]string{
		"X-Inertia":        "true",
		"X-XSRF-TOKEN":     a.xsrf,
		"Accept":           "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"Content-Type":     "application/json",
	}
	if a.version != "" {
		headers["X-Inertia-Version"] = a.version
	}
	a.mu.Unlock()

	resp, err := a.doRequestWithCookies(ctx, http.MethodPost, artistlyBaseURL+artistlyStorePath, bytes.NewReader(body), headers)
	if err != nil {
		return ArtistlyMedia{}, fmt.Errorf("artistly: generation request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ArtistlyMedia{}, fmt.Errorf("artistly: response read failed: %w", err)
	}

	// Handle session expiry (409 = Inertia version mismatch, 401 = unauthenticated, 419 = CSRF)
	if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == 419 {
		return ArtistlyMedia{}, fmt.Errorf("artistly: session expired (status %d)", resp.StatusCode)
	}

	if resp.StatusCode >= 400 {
		return ArtistlyMedia{}, fmt.Errorf("artistly: generation failed (status %d): %s", resp.StatusCode, truncateString(string(respBody), 500))
	}

	// The store POST typically returns 302 → /personal-designs. Go's HTTP client
	// follows the redirect automatically, so respBody is the personal-designs HTML.
	// The generated image is NOT inline — we must fetch it from the designs API.

	// First try: check if the response body already contains a CDN URL (unlikely but possible)
	if imageURL := extractImageURL(string(respBody)); imageURL != "" {
		return ArtistlyMedia{URL: imageURL, Status: "completed"}, nil
	}

	// Poll the internal designs API for the newest design with our prompt
	return a.pollForDesign(ctx, prompt)
}

// pollForDesign connects to Artistly's Reverb WebSocket server and listens
// for the DesignProcessed event on the design.{userId} channel. This is how
// the frontend receives generated images — there is no REST API for it.
func (a *ArtistlyClient) pollForDesign(ctx context.Context, prompt string) (ArtistlyMedia, error) {
	// Get user ID from the session by fetching dashboard props
	userID, err := a.getUserID(ctx)
	if err != nil {
		return ArtistlyMedia{}, fmt.Errorf("artistly: failed to get user ID for WebSocket: %w", err)
	}

	wsURL := "wss://ws.artistly.ai/app/kb747pbueimelgd4?protocol=7&client=js&version=8.4.0"
	channel := fmt.Sprintf("design.%s", userID)

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return ArtistlyMedia{}, fmt.Errorf("artistly: WebSocket dial failed: %w", err)
	}
	defer conn.Close()

	// Wait for connection_established
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return ArtistlyMedia{}, fmt.Errorf("artistly: WebSocket read failed: %w", err)
	}
	var connEvent map[string]json.RawMessage
	if err := json.Unmarshal(msg, &connEvent); err != nil {
		return ArtistlyMedia{}, fmt.Errorf("artistly: WebSocket parse failed: %w", err)
	}

	// Subscribe to the design channel
	subscribeMsg, _ := json.Marshal(map[string]any{
		"event": "pusher:subscribe",
		"data":  map[string]string{"channel": channel},
	})
	if err := conn.WriteMessage(websocket.TextMessage, subscribeMsg); err != nil {
		return ArtistlyMedia{}, fmt.Errorf("artistly: WebSocket subscribe failed: %w", err)
	}

	// Listen for DesignProcessed event with timeout
	deadline := time.After(ArtistlyPollTimeout)
	for {
		select {
		case <-ctx.Done():
			return ArtistlyMedia{}, ctx.Err()
		case <-deadline:
			return ArtistlyMedia{}, errors.New("artistly: timed out waiting for generated image")
		default:
		}

		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			// Timeout or transient error — keep listening
			continue
		}

		var event map[string]json.RawMessage
		if err := json.Unmarshal(msg, &event); err != nil {
			continue
		}

		eventName, _ := event["event"]
		var name string
		json.Unmarshal(eventName, &name)

		if name == "DesignProcessed" {
			rawData, ok := event["data"]
			if !ok {
				continue
			}
			// Data may be a JSON-encoded string or an object
			var dataBytes []byte
			var dataStr string
			if err := json.Unmarshal(rawData, &dataStr); err == nil {
				dataBytes = []byte(dataStr)
			} else {
				dataBytes = rawData
			}

			var design map[string]any
			if err := json.Unmarshal(dataBytes, &design); err != nil {
				continue
			}

			imageURL := findDesignImageURL(design)
			if imageURL != "" {
				return ArtistlyMedia{URL: imageURL, Status: "completed"}, nil
			}
		}
	}
}

// getUserID fetches the authenticated user's ID from the dashboard page props.
func (a *ArtistlyClient) getUserID(ctx context.Context) (string, error) {
	resp, err := a.doRequestWithCookies(ctx, http.MethodGet, artistlyBaseURL+"/dashboard", nil, map[string]string{
		"Accept": "text/html",
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`data-page="([^"]+)"`)
	matches := re.FindStringSubmatch(string(body))
	if len(matches) < 2 {
		return "", errors.New("no data-page attribute found")
	}
	decoded := strings.ReplaceAll(matches[1], "&quot;", `"`)
	decoded = strings.ReplaceAll(decoded, "&#34;", `"`)
	var page map[string]any
	if err := json.Unmarshal([]byte(decoded), &page); err != nil {
		return "", err
	}
	props, _ := page["props"].(map[string]any)
	if props == nil {
		return "", errors.New("no props in data-page")
	}
	auth, _ := props["auth"].(map[string]any)
	if auth == nil {
		return "", errors.New("no auth in props")
	}
	user, _ := auth["user"].(map[string]any)
	if user == nil {
		return "", errors.New("no user in auth")
	}
	id := user["id"]
	switch v := id.(type) {
	case float64:
		return fmt.Sprintf("%.0f", v), nil
	case string:
		return v, nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// findDesignImageURL extracts the first image URL from a DesignProcessed payload.
func findDesignImageURL(design map[string]any) string {
	// Check direct image_url field
	if url, ok := design["image_url"].(string); ok && url != "" {
		return url
	}
	// Check images array
	if images, ok := design["images"].([]any); ok && len(images) > 0 {
		if firstImage, ok := images[0].(map[string]any); ok {
			if url, ok := firstImage["url"].(string); ok && url != "" {
				return url
			}
			if url, ok := firstImage["image_url"].(string); ok && url != "" {
				return url
			}
		}
		if url, ok := images[0].(string); ok && url != "" {
			return url
		}
	}
	// Check thumbnail / preview fields
	for _, key := range []string{"thumbnail", "preview", "preview_url", "cdn_url"} {
		if url, ok := design[key].(string); ok && url != "" && strings.Contains(url, "cdn.artistly.ai") {
			return url
		}
	}
	// Last resort: search entire JSON for CDN URL
	raw, _ := json.Marshal(design)
	return extractImageURL(string(raw))
}

// ConnectionStatus verifies the session is valid by attempting login.
func (a *ArtistlyClient) ConnectionStatus(ctx context.Context) (map[string]any, error) {
	err := a.ensureSession(ctx)
	if err != nil {
		return map[string]any{"connected": false, "error": err.Error()}, nil
	}
	return map[string]any{"connected": true, "email": a.email}, nil
}

// doRequest performs an HTTP request without session cookies.
func (a *ArtistlyClient) doRequest(ctx context.Context, method, url string, body io.Reader, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", artistlyUserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return a.client.Do(req)
}

// doRequestWithCookies performs an HTTP request with session cookies attached.
// Cookies are attached manually only if the client jar has none (the jar set
// in NewArtistlyClient normally handles this automatically).
func (a *ArtistlyClient) doRequestWithCookies(ctx context.Context, method, rawURL string, body io.Reader, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", artistlyUserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if a.client.Jar == nil {
		for _, c := range a.cookies {
			req.AddCookie(c)
		}
	}
	return a.client.Do(req)
}

// setCookie updates or adds a cookie in the session.
func (a *ArtistlyClient) setCookie(c *http.Cookie) {
	for i, existing := range a.cookies {
		if existing.Name == c.Name {
			a.cookies[i] = c
			return
		}
	}
	a.cookies = append(a.cookies, c)
}

// extractInertiaVersion pulls the version string from a data-page attribute.
func extractInertiaVersion(html string) string {
	re := regexp.MustCompile(`data-page="([^"]+)"`)
	matches := re.FindStringSubmatch(html)
	if len(matches) < 2 {
		return ""
	}
	decoded := strings.ReplaceAll(matches[1], "&quot;", `"`)
	decoded = strings.ReplaceAll(decoded, "&#34;", `"`)
	var page map[string]any
	if err := json.Unmarshal([]byte(decoded), &page); err != nil {
		return ""
	}
	version, _ := page["version"].(string)
	return version
}

// extractInertiaComponent pulls the component name from a data-page attribute.
func extractInertiaComponent(html string) string {
	re := regexp.MustCompile(`data-page="([^"]+)"`)
	matches := re.FindStringSubmatch(html)
	if len(matches) < 2 {
		return ""
	}
	decoded := strings.ReplaceAll(matches[1], "&quot;", `"`)
	decoded = strings.ReplaceAll(decoded, "&#34;", `"`)
	var page map[string]any
	if err := json.Unmarshal([]byte(decoded), &page); err != nil {
		return ""
	}
	component, _ := page["component"].(string)
	return component
}

// extractImageURL searches HTML/JSON content for CDN image URLs.
func extractImageURL(content string) string {
	re := regexp.MustCompile(`https://cdn\.artistly\.ai/[^\s"'\\]+\.(?:png|jpg|jpeg|webp)`)
	match := re.FindString(content)
	return match
}

// findImageURL searches Inertia props for image URLs in common locations.
func findImageURL(props map[string]any) string {
	// Check common prop keys that might contain generated image URLs
	for _, key := range []string{"image_url", "imageUrl", "url", "download_url", "generated_image", "design"} {
		if val, ok := props[key].(string); ok && val != "" {
			return val
		}
	}
	// Check nested structures
	if flash, ok := props["flash"].(map[string]any); ok {
		if msg, ok := flash["message"].(string); ok {
			if url := extractImageURL(msg); url != "" {
				return url
			}
		}
	}
	return ""
}

// artistlyAspectRatioToSize converts an aspect ratio string to pixel dimensions.
func artistlyAspectRatioToSize(ratio string) (int, int) {
	switch ratio {
	case "1:1":
		return 1024, 1024
	case "16:9":
		return 1344, 768
	case "9:16":
		return 768, 1344
	case "21:9":
		return 1536, 640
	case "9:21":
		return 640, 1536
	case "3:2":
		return 1216, 832
	case "2:3":
		return 832, 1216
	case "4:5":
		return 896, 1088
	case "5:4":
		return 1088, 896
	case "3:4":
		return 896, 1152
	case "4:3":
		return 1152, 896
	default:
		return 1024, 1024
	}
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ArtistlyImageModels returns the model identifiers this client advertises.
func ArtistlyImageModels() []string {
	return []string{
		"artistly_v6",
		"artistly_photorealism",
		"artistly_artistic",
		"artistly_logo",
		"artistly_pattern",
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
const ArtistlyPollTimeout = 10 * time.Minute
