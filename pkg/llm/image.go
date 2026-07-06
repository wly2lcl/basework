package llm

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // 注册 WebP 解码器
)

// ImageData 封装图片数据
type ImageData struct {
	Data      []byte // 原始图片数据
	MediaType string // "image/jpeg", "image/png", "image/webp"
	Width     int
	Height    int
}

const (
	MaxImageWidth  = 2000
	MaxImageHeight = 2000
)

// LoadImage 从文件加载图片
func LoadImage(path string) (*ImageData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开图片文件失败: %w", err)
	}
	defer f.Close()
	return LoadImageFromReader(f)
}

// LoadImageFromReader 从 reader 加载
func LoadImageFromReader(r io.Reader) (*ImageData, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("读取图片数据失败: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("图片数据为空")
	}
	mediaType, err := detectMediaType(data)
	if err != nil {
		return nil, err
	}
	// 解码获取尺寸
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("解码图片配置失败: %w", err)
	}
	return &ImageData{
		Data:      data,
		MediaType: mediaType,
		Width:     cfg.Width,
		Height:    cfg.Height,
	}, nil
}

// Resize 等比缩放到最大 MaxImageWidth x MaxImageHeight
func (img *ImageData) Resize() error {
	if img.Width <= MaxImageWidth && img.Height <= MaxImageHeight {
		return nil // 无需缩放
	}
	// 计算缩放比例
	ratio := 1.0
	if img.Width > MaxImageWidth {
		ratio = float64(MaxImageWidth) / float64(img.Width)
	}
	if img.Height > MaxImageHeight {
		hRatio := float64(MaxImageHeight) / float64(img.Height)
		if hRatio < ratio {
			ratio = hRatio
		}
	}
	newWidth := int(float64(img.Width) * ratio)
	newHeight := int(float64(img.Height) * ratio)
	if newWidth < 1 {
		newWidth = 1
	}
	if newHeight < 1 {
		newHeight = 1
	}
	// 解码原图
	src, _, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil {
		return fmt.Errorf("解码图片失败: %w", err)
	}
	// 创建目标画布并等比缩放
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	// 重新编码为原格式（WebP 转为 JPEG）
	var buf bytes.Buffer
	switch img.MediaType {
	case "image/png":
		err = png.Encode(&buf, dst)
	case "image/webp":
		// WebP 只能解码不能编码，转为 JPEG
		err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85})
		img.MediaType = "image/jpeg"
	default:
		// 默认 JPEG
		err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85})
		img.MediaType = "image/jpeg"
	}
	if err != nil {
		return fmt.Errorf("编码缩放后图片失败: %w", err)
	}
	img.Data = buf.Bytes()
	img.Width = newWidth
	img.Height = newHeight
	return nil
}

// ToBase64 编码为 base64 字符串
func (img *ImageData) ToBase64() string {
	return base64.StdEncoding.EncodeToString(img.Data)
}

// detectMediaType 根据 magic bytes 检测图片格式
func detectMediaType(data []byte) (string, error) {
	if len(data) < 4 {
		return "", fmt.Errorf("图片数据过短，无法检测格式")
	}
	// JPEG: FF D8 FF
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg", nil
	}
	// PNG: 89 50 4E 47 0D 0A 1A 0A
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
		data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A {
		return "image/png", nil
	}
	// WebP: RIFF .... WEBP
	if len(data) >= 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "image/webp", nil
	}
	return "", fmt.Errorf("不支持的图片格式: %x... (前4字节)", data[:4])
}