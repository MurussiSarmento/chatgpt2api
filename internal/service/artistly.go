package service

import (
	"context"
	"fmt"

	chatgpt "github.com/basketikun/chatgpt2api/internal/upstream"
)

// ArtistlyService generates images via Artistly's web session (Laravel Inertia.js)
// and persists them into our own storage so the returned URL points at this
// server, not Artistly's CDN. Mirrors VetaService's interface.
type ArtistlyService struct {
	client *chatgpt.ArtistlyClient
	images *ImageService
}

func NewArtistlyService(email, password string, images *ImageService) *ArtistlyService {
	client := chatgpt.NewArtistlyClient("")
	client.SetCredentials(email, password)
	return &ArtistlyService{client: client, images: images}
}

// Image generates an image via Artistly and stores it locally. The model name
// maps to the category (e.g. artistly_v6 → "AI Image Designer v6"). Quality
// is "fast" or "high". Aspect ratio uses "1:1", "16:9", etc.
func (s *ArtistlyService) Image(ctx context.Context, prompt, model, quality, aspectRatio string, count int, baseURL string) (ImageResult, error) {
	category := artistlyModelToCategory(model)
	media, err := s.client.GenerateImage(ctx, prompt, category, quality, aspectRatio, count, "", "private")
	if err != nil {
		return ImageResult{}, err
	}
	url := media.DownloadURL
	if url == "" {
		url = media.URL
	}
	if url == "" {
		return ImageResult{}, fmt.Errorf("artistly: generation succeeded but no image URL was returned")
	}
	stored, err := s.store(ctx, url, baseURL)
	if err != nil {
		return ImageResult{}, err
	}
	return ImageResult{URL: stored.URL, RevisedPrompt: prompt}, nil
}

func (s *ArtistlyService) store(ctx context.Context, mediaURL, baseURL string) (StoredImage, error) {
	data, contentType, err := fetchMedia(ctx, mediaURL)
	if err != nil {
		return StoredImage{}, err
	}
	ext, ct, kind := mediaMeta(mediaURL, contentType)
	return s.images.SaveMedia(ctx, data, baseURL, ext, ct, kind)
}

// ConnectionStatus proxies to the upstream client for health checks.
func (s *ArtistlyService) ConnectionStatus(ctx context.Context) (map[string]any, error) {
	return s.client.ConnectionStatus(ctx)
}

// artistlyModelToCategory maps our advertised model names to Artistly's
// internal category names used in the POST body.
func artistlyModelToCategory(model string) string {
	switch model {
	case "artistly_v6":
		return "AI Image Designer v6"
	case "artistly_photorealism":
		return "Photorealism"
	case "artistly_artistic":
		return "Artistic Designer"
	case "artistly_logo":
		return "Logo"
	case "artistly_pattern":
		return "Seamless Patterns V2"
	default:
		return "AI Image Designer v6"
	}
}