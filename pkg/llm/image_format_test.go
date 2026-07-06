package llm

import (
	"strings"
	"testing"
)

func TestImageContentAnthropic(t *testing.T) {
	img := &ImageData{
		Data:      []byte("test-image-data"),
		MediaType: "image/jpeg",
		Width:     100,
		Height:    100,
	}
	content, err := ImageContentForProvider(img, "anthropic")
	if err != nil {
		t.Fatalf("ImageContentForProvider 失败: %v", err)
	}

	// 验证结构
	source, ok := content["source"].(map[string]any)
	if !ok {
		t.Fatal("source 应为 map[string]any")
	}
	if source["type"] != "base64" {
		t.Errorf("期望 type=base64, 得到 %v", source["type"])
	}
	if source["media_type"] != "image/jpeg" {
		t.Errorf("期望 media_type=image/jpeg, 得到 %v", source["media_type"])
	}
	if content["type"] != "image" {
		t.Errorf("期望 type=image, 得到 %v", content["type"])
	}
}

func TestImageContentOpenAI(t *testing.T) {
	img := &ImageData{
		Data:      []byte("test-image-data"),
		MediaType: "image/png",
		Width:     100,
		Height:    100,
	}
	content, err := ImageContentForProvider(img, "openai")
	if err != nil {
		t.Fatalf("ImageContentForProvider 失败: %v", err)
	}

	if content["type"] != "image_url" {
		t.Errorf("期望 type=image_url, 得到 %v", content["type"])
	}
	imageURL, ok := content["image_url"].(map[string]any)
	if !ok {
		t.Fatal("image_url 应为 map[string]any")
	}
	url, ok := imageURL["url"].(string)
	if !ok {
		t.Fatal("url 应为 string")
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Errorf("url 应以 data:image/png;base64, 开头, 得到 %s", url)
	}
}

func TestImageContentGemini(t *testing.T) {
	img := &ImageData{
		Data:      []byte("test-image-data"),
		MediaType: "image/webp",
		Width:     100,
		Height:    100,
	}
	content, err := ImageContentForProvider(img, "gemini")
	if err != nil {
		t.Fatalf("ImageContentForProvider 失败: %v", err)
	}

	inlineData, ok := content["inline_data"].(map[string]any)
	if !ok {
		t.Fatal("inline_data 应为 map[string]any")
	}
	if inlineData["mime_type"] != "image/webp" {
		t.Errorf("期望 mime_type=image/webp, 得到 %v", inlineData["mime_type"])
	}
}

func TestImageContentUnsupported(t *testing.T) {
	img := &ImageData{
		Data:      []byte("test-image-data"),
		MediaType: "image/jpeg",
		Width:     100,
		Height:    100,
	}
	_, err := ImageContentForProvider(img, "bedrock")
	if err == nil {
		t.Fatal("期望不支持的 provider 返回错误，但没有")
	}
}

func TestImageContentOpenAIEncode(t *testing.T) {
	// 验证 base64 编码正确性
	img := &ImageData{
		Data:      []byte("hello"),
		MediaType: "image/jpeg",
		Width:     100,
		Height:    100,
	}
	content, err := ImageContentForProvider(img, "openai")
	if err != nil {
		t.Fatalf("ImageContentForProvider 失败: %v", err)
	}
	imageURL := content["image_url"].(map[string]any)
	url := imageURL["url"].(string)

	expectedPrefix := "data:image/jpeg;base64," + img.ToBase64()
	if url != expectedPrefix {
		t.Errorf("期望 URL %s, 得到 %s", expectedPrefix, url)
	}
}