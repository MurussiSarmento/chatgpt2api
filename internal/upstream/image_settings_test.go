package upstream_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"
)

// TestOpenAIBackendImageSettingsIntegration verifies that web image generation
// can still specify an aspect ratio via message.metadata.image_generation_settings.
func TestOpenAIBackendImageSettingsIntegration(t *testing.T) {
	client := openAIBackendClient(t)
	message := map[string]any{
		"role":    "user",
		"content": "Draw an orange cat, 16:9 landscape composition, clean illustration",
		"image_generation_settings": map[string]any{
			"aspect_ratio": "16:9",
			"image_count":  1,
		},
	}
	conversationID, err := client.StartConversation(context.Background(), []map[string]any{message}, "auto", "", []string{"picture_v2"}, false, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	ids, err := client.PollImagesFor(context.Background(), conversationID, 3*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("missing image")
	}
	image, err := client.Download(context.Background(), conversationID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(image) == 0 {
		t.Fatal("empty image")
	}
}

func TestOpenAIBackendImageEditIntegration(t *testing.T) {
	client := openAIBackendClient(t)
	source := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			source.Set(x, y, color.RGBA{R: uint8(x / 2), G: uint8(y / 2), B: 180, A: 255})
		}
	}
	buffer := bytes.NewBuffer(nil)
	if err := png.Encode(buffer, source); err != nil {
		t.Fatal(err)
	}
	message := map[string]any{"role": "user", "content": []any{
		map[string]any{"type": "text", "text": "Draw a white dot in the center of the image"},
		map[string]any{"type": "image", "data": buffer.Bytes(), "name": "source.png", "mime": "image/png"},
	}}
	conversationID, err := client.StartConversation(context.Background(), []map[string]any{message}, "auto", "", []string{"picture_v2"}, false, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	ids, err := client.PollImagesFor(context.Background(), conversationID, 3*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	imageData, err := client.Download(context.Background(), conversationID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(imageData) == 0 {
		t.Fatal("empty image")
	}
}
