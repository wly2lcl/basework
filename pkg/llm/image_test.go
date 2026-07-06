package llm

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// createTestPNG 创建测试用 PNG 文件
func createTestPNG(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	// 填充纯色
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.NRGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("创建测试图片失败: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("写入测试图片失败: %v", err)
	}
	return path
}

func TestLoadImage(t *testing.T) {
	path := createTestPNG(t, 100, 100)
	img, err := LoadImage(path)
	if err != nil {
		t.Fatalf("LoadImage 失败: %v", err)
	}
	if img.MediaType != "image/png" {
		t.Errorf("期望 MediaType image/png, 得到 %s", img.MediaType)
	}
	if img.Width != 100 || img.Height != 100 {
		t.Errorf("期望 100x100, 得到 %dx%d", img.Width, img.Height)
	}
	if len(img.Data) == 0 {
		t.Error("Data 不应为空")
	}
}

func TestLoadImageInvalid(t *testing.T) {
	// 无效文件路径
	_, err := LoadImage("/nonexistent/path/to/image.png")
	if err == nil {
		t.Fatal("期望无效路径返回错误，但没有")
	}
	// 无效数据
	_, err = LoadImageFromReader(bytes.NewReader([]byte("not-an-image")))
	if err == nil {
		t.Fatal("期望无效数据返回错误，但没有")
	}
	// 空数据
	_, err = LoadImageFromReader(bytes.NewReader(nil))
	if err == nil {
		t.Fatal("期望空数据返回错误，但没有")
	}
}

func TestResize(t *testing.T) {
	// 创建超过限制的大图
	path := createTestPNG(t, 3000, 2000)
	img, err := LoadImage(path)
	if err != nil {
		t.Fatalf("LoadImage 失败: %v", err)
	}
	if err := img.Resize(); err != nil {
		t.Fatalf("Resize 失败: %v", err)
	}
	if img.Width > MaxImageWidth || img.Height > MaxImageHeight {
		t.Errorf("缩放后尺寸 %dx%d 超过限制 %dx%d", img.Width, img.Height, MaxImageWidth, MaxImageHeight)
	}
	// 验证等比缩放 — 宽度应缩放到 MaxImageWidth
	if img.Width != MaxImageWidth {
		t.Errorf("宽度应缩放到 %d, 得到 %d", MaxImageWidth, img.Width)
	}
}

func TestResizeSmall(t *testing.T) {
	// 小于限制的图片不应被缩放
	path := createTestPNG(t, 100, 100)
	img, err := LoadImage(path)
	if err != nil {
		t.Fatalf("LoadImage 失败: %v", err)
	}
	if err := img.Resize(); err != nil {
		t.Fatalf("Resize 失败: %v", err)
	}
	if img.Width != 100 || img.Height != 100 {
		t.Errorf("小图不应被缩放，期望 100x100, 得到 %dx%d", img.Width, img.Height)
	}
}

func TestResizeWidthDominant(t *testing.T) {
	// 宽度比例更大的图片
	path := createTestPNG(t, 4000, 1000)
	img, err := LoadImage(path)
	if err != nil {
		t.Fatalf("LoadImage 失败: %v", err)
	}
	if err := img.Resize(); err != nil {
		t.Fatalf("Resize 失败: %v", err)
	}
	if img.Width > MaxImageWidth || img.Height > MaxImageHeight {
		t.Errorf("缩放后尺寸 %dx%d 超过限制 %dx%d", img.Width, img.Height, MaxImageWidth, MaxImageHeight)
	}
}

func TestResizeHeightDominant(t *testing.T) {
	// 高度比例更大的图片
	path := createTestPNG(t, 1000, 4000)
	img, err := LoadImage(path)
	if err != nil {
		t.Fatalf("LoadImage 失败: %v", err)
	}
	if err := img.Resize(); err != nil {
		t.Fatalf("Resize 失败: %v", err)
	}
	if img.Width > MaxImageWidth || img.Height > MaxImageHeight {
		t.Errorf("缩放后尺寸 %dx%d 超过限制 %dx%d", img.Width, img.Height, MaxImageWidth, MaxImageHeight)
	}
	// 高度应缩放到 MaxImageHeight
	if img.Height != MaxImageHeight {
		t.Errorf("高度应缩放到 %d, 得到 %d", MaxImageHeight, img.Height)
	}
}

func TestResizeMinimumSize(t *testing.T) {
	// 极小图片缩放后至少为 1x1
	path := createTestPNG(t, 2, 2)
	img, err := LoadImage(path)
	if err != nil {
		t.Fatalf("LoadImage 失败: %v", err)
	}
	if err := img.Resize(); err != nil {
		t.Fatalf("Resize 失败: %v", err)
	}
	if img.Width < 1 || img.Height < 1 {
		t.Errorf("缩放后尺寸至少为 1x1, 得到 %dx%d", img.Width, img.Height)
	}
}

func TestToBase64(t *testing.T) {
	path := createTestPNG(t, 10, 10)
	img, err := LoadImage(path)
	if err != nil {
		t.Fatalf("LoadImage 失败: %v", err)
	}
	b64 := img.ToBase64()
	if b64 == "" {
		t.Fatal("base64 不应为空")
	}
	// 验证可以解码回原数据
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("base64 解码失败: %v", err)
	}
	if len(decoded) != len(img.Data) {
		t.Errorf("解码后数据长度 %d 不等于原数据长度 %d", len(decoded), len(img.Data))
	}
}