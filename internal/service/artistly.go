package service

import (
	"context"

	chatgpt "github.com/basketikun/chatgpt2api/internal/upstream"
)

// ArtistlyService generates images via Artistly MCP and persists them into our
// own storage so the returned URL points at this server, not Artistly's domain.
// Mirrors VetaService exactly; swap the upstream client when real endpoints
// land.
type ArtistlyService struct {
	client *chatgpt.ArtistlyClient
	images *ImageService
}

func NewArtistlyService(token string, images *ImageService) *ArtistlyService {
	return &ArtistlyService{client: chatgpt.NewArtistlyClient(token), images: images}
}

// Image generates an image via Artistly and stores it locally. The category
// maps to the model name (e.g. artistly_v6 → AI Image Designer v6 prompt).
// Quality, aspectRatio, count, folder and visibility are forwarded as-is once
// the upstream client is implemented; today they are accepted but unused by
// the stub.
func (s *ArtistlyService) Image(ctx context.Context, prompt, model, quality, aspectRatio string, count int, baseURL string) (ImageResult, error) {
	media, err := s.client.GenerateImage(ctx, prompt, model, quality, aspectRatio, count, "", "private")
	if err != nil {
		return ImageResult{}, err
	}
	url := media.DownloadURL
	if url == "" {
		url = media.URL
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