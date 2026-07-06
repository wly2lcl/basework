//go:build sqlite

package session

import (
	"encoding/json"
	"testing"
)

func TestCompressor_Snappy(t *testing.T) {
	compressor := NewCompressor("snappy")

	// 测试数据
	data := []byte("Hello, World! This is a test message for compression.")

	// 压缩
	compressed, err := compressor.Compress(data)
	if err != nil {
		t.Fatalf("Compress 失败: %v", err)
	}

	if len(compressed) == 0 {
		t.Fatal("压缩后数据为空")
	}

	// 解压
	decompressed, err := compressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress 失败: %v", err)
	}

	if string(decompressed) != string(data) {
		t.Errorf("解压后数据不匹配: 期望 %s，得到 %s", data, decompressed)
	}
}

func TestCompressor_Gzip(t *testing.T) {
	compressor := NewCompressor("gzip")

	data := []byte("Hello, World! This is a test message for compression.")

	compressed, err := compressor.Compress(data)
	if err != nil {
		t.Fatalf("Compress 失败: %v", err)
	}

	decompressed, err := compressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress 失败: %v", err)
	}

	if string(decompressed) != string(data) {
		t.Errorf("解压后数据不匹配")
	}
}

func TestMessageCompressor_CompressDecompress(t *testing.T) {
	config := DefaultCompressionConfig()
	config.Threshold = 2 // 降低阈值以便测试
	mc := NewMessageCompressor(config)

	// 创建测试消息
	messages := []json.RawMessage{
		json.RawMessage(`{"role":"user","content":"Hello"}`),
		json.RawMessage(`{"role":"assistant","content":"Hi"}`),
		json.RawMessage(`{"role":"user","content":"How are you?"}`),
	}

	// 压缩
	compressed, err := mc.CompressMessages(messages)
	if err != nil {
		t.Fatalf("CompressMessages 失败: %v", err)
	}

	if len(compressed) == 0 {
		t.Fatal("压缩后数据为空")
	}

	// 解压
	decompressed, err := mc.DecompressMessages(compressed)
	if err != nil {
		t.Fatalf("DecompressMessages 失败: %v", err)
	}

	if len(decompressed) != len(messages) {
		t.Errorf("消息数量不匹配: 期望 %d，得到 %d", len(messages), len(decompressed))
	}
}

func TestMessageCompressor_Stats(t *testing.T) {
	config := DefaultCompressionConfig()
	config.Threshold = 2
	mc := NewMessageCompressor(config)

	messages := []json.RawMessage{
		json.RawMessage(`{"role":"user","content":"Hello"}`),
		json.RawMessage(`{"role":"assistant","content":"Hi"}`),
		json.RawMessage(`{"role":"user","content":"Test"}`),
	}

	mc.CompressMessages(messages)
	stats := mc.GetStats()

	if stats.TotalMessages != 3 {
		t.Errorf("期望 TotalMessages=3，得到 %d", stats.TotalMessages)
	}

	if stats.CompressedCount != 3 {
		t.Errorf("期望 CompressedCount=3，得到 %d", stats.CompressedCount)
	}

	if stats.OriginalSize == 0 {
		t.Error("期望 OriginalSize > 0")
	}

	if stats.CompressedSize == 0 {
		t.Error("期望 CompressedSize > 0")
	}
}

func TestMessageCompressor_NoCompressionBelowThreshold(t *testing.T) {
	config := DefaultCompressionConfig()
	config.Threshold = 10 // 高阈值
	mc := NewMessageCompressor(config)

	messages := []json.RawMessage{
		json.RawMessage(`{"role":"user","content":"Hello"}`),
		json.RawMessage(`{"role":"assistant","content":"Hi"}`),
	}

	compressed, err := mc.CompressMessages(messages)
	if err != nil {
		t.Fatalf("CompressMessages 失败: %v", err)
	}

	decompressed, err := mc.DecompressMessages(compressed)
	if err != nil {
		t.Fatalf("DecompressMessages 失败: %v", err)
	}

	if len(decompressed) != len(messages) {
		t.Errorf("消息数量不匹配")
	}

	stats := mc.GetStats()
	// 低于阈值，不应该压缩
	if stats.CompressedCount != 0 {
		t.Errorf("期望 CompressedCount=0，得到 %d", stats.CompressedCount)
	}
}
