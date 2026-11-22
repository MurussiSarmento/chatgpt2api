package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"path"
	"strings"
	"syscall"
	"time"

	chatgpt "github.com/basketikun/chatgpt2api/internal/upstream"
)

const vetaMaxMediaBytes = 200 << 20 // 200MB cap for downloaded media (video)

// VetaService generates media with Veta IA and persists it into our own storage
// so the returned URL points at this server, not Veta's domain.
type VetaService struct {
	client *chatgpt.VetaClient
	images *ImageService
}

func NewVetaService(token string, images *ImageService) *VetaService {
	return &VetaService{client: chatgpt.NewVetaClient(token), images: images}
}

// Image generates an image via Veta (e.g. nano_banana_pro) and stores it.
func (s *VetaService) Image(ctx context.Context, prompt, model, aspect, resolution, baseURL string) (ImageResult, error) {
	media, err := s.client.GenerateImage(ctx, prompt, model, aspect, resolution)
	if err != nil {
		return ImageResult{}, err
	}
	stored, err := s.store(ctx, pickURL(media), baseURL)
	if err != nil {
		return ImageResult{}, err
	}
	return ImageResult{URL: stored.URL, RevisedPrompt: prompt}, nil
}

// Video generates a video via Veta (Veo 3.1) and stores it, returning our URL.
func (s *VetaService) Video(ctx context.Context, prompt, model, aspect, quality, duration, startImage, baseURL string) (string, error) {
	media, err := s.client.GenerateVideo(ctx, prompt, model, aspect, quality, duration, startImage)
	if err != nil {
		return "", err
	}
	stored, err := s.store(ctx, pickURL(media), baseURL)
	if err != nil {
		return "", err
	}
	return stored.URL, nil
}

func (s *VetaService) store(ctx context.Context, mediaURL, baseURL string) (StoredImage, error) {
	if mediaURL == "" {
		return StoredImage{}, errors.New("veta returned no media url")
	}
	data, contentType, err := fetchMedia(ctx, mediaURL)
	if err != nil {
		return StoredImage{}, err
	}
	ext, ct, kind := mediaMeta(mediaURL, contentType)
	return s.images.SaveMedia(ctx, data, baseURL, ext, ct, kind)
}

func pickURL(media chatgpt.VetaMedia) string {
	if media.DownloadURL != "" {
		return media.DownloadURL
	}
	return media.URL
}

// mediaFetchClient blocks private/internal addresses (SSRF guard) while
// accepting any content type (unlike the image-only fetcher in the handler).
var mediaFetchClient = &http.Client{Timeout: 5 * time.Minute, Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 10 * time.Second, Control: func(network string, address string, conn syscall.RawConn) error {
	host, _, _ := net.SplitHostPort(address)
	ip, _ := netip.ParseAddr(host)
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return errors.New("media url must point to a public address")
	}
	return nil
}}).DialContext, TLSHandshakeTimeout: 10 * time.Second}}

func fetchMedia(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "chatgpt2api media fetcher")
	resp, err := mediaFetchClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", errors.New("media fetch failed")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, vetaMaxMediaBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > vetaMaxMediaBytes {
		return nil, "", errors.New("media exceeds size limit")
	}
	if len(data) == 0 {
		return nil, "", errors.New("media is empty")
	}
	contentType := resp.Header.Get("Content-Type")
	if index := strings.IndexByte(contentType, ';'); index >= 0 {
		contentType = contentType[:index]
	}
	return data, strings.TrimSpace(contentType), nil
}

// mediaMeta resolves the stored extension, content-type and kind from the media
// URL and the response content-type.
func mediaMeta(url string, contentType string) (ext string, ct string, kind string) {
	contentType = strings.ToLower(contentType)
	switch filepath := strings.ToLower(path.Ext(strings.Split(url, "?")[0])); {
	case strings.Contains(contentType, "mp4") || filepath == ".mp4":
		return ".mp4", "video/mp4", "video"
	case strings.Contains(contentType, "webm") || filepath == ".webm":
		return ".webm", "video/webm", "video"
	case strings.Contains(contentType, "quicktime") || filepath == ".mov":
		return ".mov", "video/quicktime", "video"
	case strings.Contains(contentType, "jpeg") || filepath == ".jpg" || filepath == ".jpeg":
		return ".jpg", "image/jpeg", "image"
	case strings.Contains(contentType, "webp") || filepath == ".webp":
		return ".webp", "image/webp", "image"
	case strings.Contains(contentType, "png") || filepath == ".png":
		return ".png", "image/png", "image"
	case strings.HasPrefix(contentType, "video"):
		return ".mp4", "video/mp4", "video"
	default:
		return ".png", "image/png", "image"
	}
}
