package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

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
	log.Printf("[artistly] GenerateImage called: prompt=%q category=%q quality=%q aspect=%s count=%d", truncateString(prompt, 60), category, quality, aspectRatio, count)
	// Retry once on session expiry (419/409/401)
	for attempt := 0; attempt < 2; attempt++ {
		log.Printf("[artistly] GenerateImage attempt %d/2", attempt+1)
		media, err := a.generateImageOnce(ctx, prompt, category, quality, aspectRatio, count, folder, visibility)
		if err == nil {
			log.Printf("[artistly] GenerateImage succeeded: url=%s status=%s", truncateString(media.URL, 80), media.Status)
			return media, nil
		}
		log.Printf("[artistly] GenerateImage attempt %d failed: %v", attempt+1, err)
		// Only retry on session-related errors
		errMsg := err.Error()
		if strings.Contains(errMsg, "session expired") || strings.Contains(errMsg, "status 419") || strings.Contains(errMsg, "status 409") || strings.Contains(errMsg, "status 401") {
			log.Printf("[artistly] Session error detected, clearing state for retry")
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
	log.Printf("[artistly] ensureSession starting")
	if err := a.ensureSession(ctx); err != nil {
		log.Printf("[artistly] ensureSession failed: %v", err)
		return ArtistlyMedia{}, err
	}
	log.Printf("[artistly] ensureSession OK, xsrf=%s... version=%s", truncateString(a.xsrf, 20), a.version)

	// Parse aspect ratio into width/height
	width, height := artistlyAspectRatioToSize(aspectRatio)

	if category == "" {
		category = "AI Image Designer v6"
	}
	// Artistly only accepts "fast" or "high"; anything else (e.g. the Studio's
	// "auto") is accepted with HTTP 200 but the job is silently never created.
	if quality != "high" {
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

	// Capture the submission timestamp BEFORE the POST: the design is created
	// server-side during the request, so a timestamp taken after the response
	// can exclude the very design we just created (clock/latency race).
	submittedAt := time.Now().UTC().Add(-5 * time.Second) // clock-skew allowance

	log.Printf("[artistly] POSTing to store endpoint: %s", artistlyBaseURL+artistlyStorePath)
	resp, err := a.doRequestWithCookies(ctx, http.MethodPost, artistlyBaseURL+artistlyStorePath, bytes.NewReader(body), headers)
	if err != nil {
		log.Printf("[artistly] store POST failed: %v", err)
		return ArtistlyMedia{}, fmt.Errorf("artistly: generation request failed: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("[artistly] store POST response: status=%d content_length=%d", resp.StatusCode, resp.ContentLength)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[artistly] response read failed: %v", err)
		return ArtistlyMedia{}, fmt.Errorf("artistly: response read failed: %w", err)
	}
	log.Printf("[artistly] response body length: %d bytes", len(respBody))

	// Handle session expiry (409 = Inertia version mismatch, 401 = unauthenticated, 419 = CSRF)
	if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == 419 {
		log.Printf("[artistly] session expired: status=%d body=%s", resp.StatusCode, truncateString(string(respBody), 200))
		return ArtistlyMedia{}, fmt.Errorf("artistly: session expired (status %d)", resp.StatusCode)
	}

	if resp.StatusCode >= 400 {
		log.Printf("[artistly] generation failed: status=%d body=%s", resp.StatusCode, truncateString(string(respBody), 200))
		return ArtistlyMedia{}, fmt.Errorf("artistly: generation failed (status %d): %s", resp.StatusCode, truncateString(string(respBody), 500))
	}

	// The store POST typically returns 302 → /personal-designs. Go's HTTP client
	// follows the redirect automatically, so respBody is the personal-designs HTML.
	// The generated image is NOT inline — we must fetch it from the designs API.

	// First try: check if the response body already contains a CDN URL (unlikely but possible)
	if imageURL := extractImageURL(string(respBody)); imageURL != "" {
		log.Printf("[artistly] found CDN URL directly in store response: %s", truncateString(imageURL, 80))
		return ArtistlyMedia{URL: imageURL, Status: "completed"}, nil
	}

	log.Printf("[artistly] no CDN URL in store response, starting designs API poll")
	return a.pollForDesign(ctx, submittedAt)
}

// pollForDesign polls the REST designs API (/api/internal/designs) for a new
// design created after submission. Verified working: it returns the user's
// designs newest-first, including the CDN image URL. We prefer this over the
// Reverb WebSocket (private-design.{userId}) because the WebSocket event only
// fires for jobs Artistly actually created — and any transient disconnect
// could miss it entirely.
func (a *ArtistlyClient) pollForDesign(ctx context.Context, submittedAt time.Time) (ArtistlyMedia, error) {
	log.Printf("[artistly] pollForDesign starting (submittedAt=%s)", submittedAt.Format(time.RFC3339))
	overallDeadline := time.Now().Add(ArtistlyPollTimeout)
	interval := 5 * time.Second

	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return ArtistlyMedia{}, ctx.Err()
		}
		if time.Now().After(overallDeadline) {
			log.Printf("[artistly] overall deadline (%v) reached", ArtistlyPollTimeout)
			return ArtistlyMedia{}, errors.New("artistly: timed out waiting for generated image")
		}

		imageURL, found, err := a.fetchLatestDesignImage(ctx, submittedAt)
		if err != nil {
			log.Printf("[artistly] designs poll attempt %d error: %v", attempt, err)
		} else if found {
			log.Printf("[artistly] found image URL after %d polls: %s", attempt, truncateString(imageURL, 100))
			return ArtistlyMedia{URL: imageURL, Status: "completed"}, nil
		} else {
			log.Printf("[artistly] designs poll attempt %d: no new design yet", attempt)
		}

		select {
		case <-ctx.Done():
			return ArtistlyMedia{}, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// fetchLatestDesignImage calls GET /api/internal/designs and returns the image
// URL of the newest design created after `since`. found=false when no design
// qualifies yet (job still processing).
func (a *ArtistlyClient) fetchLatestDesignImage(ctx context.Context, since time.Time) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artistlyBaseURL+"/api/internal/designs", nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("User-Agent", artistlyUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if a.client.Jar == nil {
		for _, c := range a.cookies {
			req.AddCookie(c)
		}
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", false, fmt.Errorf("designs API status %d: %s", resp.StatusCode, truncateString(string(body), 200))
	}

	var out struct {
		Data []struct {
			UUID      string `json:"uuid"`
			Images    string `json:"images"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", false, err
	}

	for _, d := range out.Data {
		created, err := time.Parse(time.RFC3339, d.CreatedAt)
		if err != nil {
			continue
		}
		if created.Before(since) {
			continue // older than this submission
		}
		if d.Images == "" {
			continue // design exists but image not ready
		}
		var urls []string
		if err := json.Unmarshal([]byte(d.Images), &urls); err != nil || len(urls) == 0 || urls[0] == "" {
			continue
		}
		return urls[0], true, nil
	}
	return "", false, nil
}
// getUserID fetches the authenticated user's ID from the dashboard page props.
func (a *ArtistlyClient) getUserID(ctx context.Context) (string, error) {
	log.Printf("[artistly] getUserID: fetching dashboard page")
	resp, err := a.doRequestWithCookies(ctx, http.MethodGet, artistlyBaseURL+"/dashboard", nil, map[string]string{
		"Accept": "text/html",
	})
	if err != nil {
		log.Printf("[artistly] getUserID: dashboard fetch failed: %v", err)
		return "", err
	}
	defer resp.Body.Close()
	log.Printf("[artistly] getUserID: dashboard status=%d", resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[artistly] getUserID: body read failed: %v", err)
		return "", err
	}
	log.Printf("[artistly] getUserID: body length=%d", len(body))

	re := regexp.MustCompile(`data-page="([^"]+)"`)
	matches := re.FindStringSubmatch(string(body))
	if len(matches) < 2 {
		log.Printf("[artistly] getUserID: no data-page attribute found")
		return "", errors.New("no data-page attribute found")
	}
	decoded := strings.ReplaceAll(matches[1], "&quot;", `"`)
	decoded = strings.ReplaceAll(decoded, "&#34;", `"`)
	var page map[string]any
	if err := json.Unmarshal([]byte(decoded), &page); err != nil {
		log.Printf("[artistly] getUserID: JSON unmarshal failed: %v", err)
		return "", err
	}
	props, _ := page["props"].(map[string]any)
	if props == nil {
		log.Printf("[artistly] getUserID: no props in data-page")
		return "", errors.New("no props in data-page")
	}
	auth, _ := props["auth"].(map[string]any)
	if auth == nil {
		log.Printf("[artistly] getUserID: no auth in props")
		return "", errors.New("no auth in props")
	}
	user, _ := auth["user"].(map[string]any)
	if user == nil {
		log.Printf("[artistly] getUserID: no user in auth")
		return "", errors.New("no user in auth")
	}
	id := user["id"]
	switch v := id.(type) {
	case float64:
		result := fmt.Sprintf("%.0f", v)
		log.Printf("[artistly] getUserID: found userID=%s", result)
		return result, nil
	case string:
		log.Printf("[artistly] getUserID: found userID=%s", v)
		return v, nil
	default:
		result := fmt.Sprintf("%v", v)
		log.Printf("[artistly] getUserID: found userID=%s (type=%T)", result, v)
		return result, nil
	}
}

// findDesignImageURL extracts the first image URL from a DesignProcessed payload.
func findDesignImageURL(design map[string]any) string {
	log.Printf("[artistly] findDesignImageURL: checking design keys=%v", func() []string {
		keys := make([]string, 0, len(design))
		for k := range design {
			keys = append(keys, k)
		}
		return keys
	}())
	// Check direct image_url field
	if url, ok := design["image_url"].(string); ok && url != "" {
		log.Printf("[artistly] found image_url: %s", truncateString(url, 100))
		return url
	}
	// Check images array — the DesignProcessed payload carries images as a
	// JSON-encoded string: "images":"[\"https://cdn.artistly.ai/...\"]"
	if imagesRaw, ok := design["images"]; ok {
		switch v := imagesRaw.(type) {
		case string:
			var urls []string
			if err := json.Unmarshal([]byte(v), &urls); err == nil && len(urls) > 0 && urls[0] != "" {
				log.Printf("[artistly] found images[0] from JSON string: %s", truncateString(urls[0], 100))
				return urls[0]
			}
			if v != "" && strings.HasPrefix(v, "https://") {
				log.Printf("[artistly] found images as plain URL: %s", truncateString(v, 100))
				return v
			}
		case []any:
			if len(v) > 0 {
				if s, ok := v[0].(string); ok && s != "" {
					log.Printf("[artistly] found images[0] from array: %s", truncateString(s, 100))
					return s
				}
				if m, ok := v[0].(map[string]any); ok {
					if u, ok := m["url"].(string); ok && u != "" {
						return u
					}
					if u, ok := m["image_url"].(string); ok && u != "" {
						return u
					}
				}
			}
		}
	}
	// Check thumbnail / preview fields
	for _, key := range []string{"thumbnail", "preview", "preview_url", "cdn_url"} {
		if url, ok := design[key].(string); ok && url != "" && strings.Contains(url, "cdn.artistly.ai") {
			log.Printf("[artistly] found %s: %s", key, truncateString(url, 100))
			return url
		}
	}
	// Last resort: search entire JSON for CDN URL
	log.Printf("[artistly] no known fields matched, searching raw JSON for CDN URL")
	raw, _ := json.Marshal(design)
	url := extractImageURL(string(raw))
	if url != "" {
		log.Printf("[artistly] found CDN URL via regex fallback: %s", truncateString(url, 100))
	} else {
		log.Printf("[artistly] no CDN URL found in design data at all")
	}
	return url
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
