package llm

import "fmt"

// ImageContentForProvider 将图片转换为特定 Provider 的格式
func ImageContentForProvider(img *ImageData, provider string) (map[string]any, error) {
	switch provider {
	case "anthropic":
		return map[string]any{
			"type": "image",
			"source": map[string]any{
				"type":       "base64",
				"media_type": img.MediaType,
				"data":       img.ToBase64(),
			},
		}, nil
	case "openai":
		return map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": "data:" + img.MediaType + ";base64," + img.ToBase64(),
			},
		}, nil
	case "gemini":
		return map[string]any{
			"inline_data": map[string]any{
				"mime_type": img.MediaType,
				"data":      img.ToBase64(),
			},
		}, nil
	default:
		return nil, fmt.Errorf("provider %q does not support image input", provider)
	}
}
