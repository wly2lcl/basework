//go:build sqlite

package session

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/golang/snappy"
)

// CompressionConfig 是压缩配置
type CompressionConfig struct {
	Enabled       bool  `json:"enabled"`        // 是否启用压缩
	Threshold     int   `json:"threshold"`      // 触发压缩的消息数阈值（默认 1000）
	Algorithm     string `json:"algorithm"`     // 压缩算法：snappy/gzip（默认 snappy）
}

// DefaultCompressionConfig 返回默认压缩配置
func DefaultCompressionConfig() CompressionConfig {
	return CompressionConfig{
		Enabled:   true,
		Threshold: 1000,
		Algorithm: "snappy",
	}
}

// Compressor 是压缩器接口
type Compressor interface {
	Compress(data []byte) ([]byte, error)
	Decompress(data []byte) ([]byte, error)
}

// SnappyCompressor 使用 Snappy 算法压缩
type SnappyCompressor struct{}

// Compress 使用 Snappy 压缩数据
func (c *SnappyCompressor) Compress(data []byte) ([]byte, error) {
	return snappy.Encode(nil, data), nil
}

// Decompress 使用 Snappy 解压数据
func (c *SnappyCompressor) Decompress(data []byte) ([]byte, error) {
	return snappy.Decode(nil, data)
}

// GzipCompressor 使用 Gzip 算法压缩
type GzipCompressor struct{}

// Compress 使用 Gzip 压缩数据
func (c *GzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(data); err != nil {
		return nil, fmt.Errorf("gzip compress failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("gzip close failed: %w", err)
	}
	return buf.Bytes(), nil
}

// Decompress 使用 Gzip 解压数据
func (c *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader failed: %w", err)
	}
	defer reader.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return nil, fmt.Errorf("gzip decompress failed: %w", err)
	}
	return buf.Bytes(), nil
}

// NewCompressor 根据算法名称创建压缩器
func NewCompressor(algorithm string) Compressor {
	switch algorithm {
	case "gzip":
		return &GzipCompressor{}
	default:
		return &SnappyCompressor{}
	}
}

// CompressedMessage 是压缩后的消息
type CompressedMessage struct {
	Compressed bool   `json:"compressed"`
	Algorithm  string `json:"algorithm"`
	Data       []byte `json:"data"`
	OrigSize   int    `json:"orig_size"`
}

// CompressionStats 是压缩统计信息
type CompressionStats struct {
	TotalMessages    int     `json:"total_messages"`
	CompressedCount  int     `json:"compressed_count"`
	OriginalSize     int64   `json:"original_size"`
	CompressedSize   int64   `json:"compressed_size"`
	CompressionRatio float64 `json:"compression_ratio"`
}

// MessageCompressor 管理消息压缩
type MessageCompressor struct {
	config     CompressionConfig
	compressor Compressor
	mu         sync.RWMutex
	stats      CompressionStats
}

// NewMessageCompressor 创建消息压缩器
func NewMessageCompressor(config CompressionConfig) *MessageCompressor {
	return &MessageCompressor{
		config:     config,
		compressor: NewCompressor(config.Algorithm),
		stats:      CompressionStats{},
	}
}

// CompressMessages 压缩消息列表
func (mc *MessageCompressor) CompressMessages(messages []json.RawMessage) ([]byte, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if !mc.config.Enabled || len(messages) < mc.config.Threshold {
		// 不压缩，直接序列化
		data, err := json.Marshal(messages)
		if err != nil {
			return nil, err
		}
		mc.stats.TotalMessages += len(messages)
		mc.stats.OriginalSize += int64(len(data))
		mc.stats.CompressedSize += int64(len(data))
		return data, nil
	}

	// 压缩
	data, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}

	originalSize := len(data)
	compressed, err := mc.compressor.Compress(data)
	if err != nil {
		return nil, fmt.Errorf("compress failed: %w", err)
	}

	// 包装压缩数据
	wrapper := CompressedMessage{
		Compressed: true,
		Algorithm:  mc.config.Algorithm,
		Data:       compressed,
		OrigSize:   originalSize,
	}

	result, err := json.Marshal(wrapper)
	if err != nil {
		return nil, err
	}

	// 更新统计
	mc.stats.TotalMessages += len(messages)
	mc.stats.CompressedCount += len(messages)
	mc.stats.OriginalSize += int64(originalSize)
	mc.stats.CompressedSize += int64(len(result))
	if originalSize > 0 {
		mc.stats.CompressionRatio = float64(mc.stats.CompressedSize) / float64(mc.stats.OriginalSize)
	}

	return result, nil
}

// DecompressMessages 解压消息列表
func (mc *MessageCompressor) DecompressMessages(data []byte) ([]json.RawMessage, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// 尝试解析为压缩格式
	var wrapper CompressedMessage
	if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Compressed {
		// 解压
		decompressed, err := mc.compressor.Decompress(wrapper.Data)
		if err != nil {
			return nil, fmt.Errorf("decompress failed: %w", err)
		}

		var messages []json.RawMessage
		if err := json.Unmarshal(decompressed, &messages); err != nil {
			return nil, err
		}
		return messages, nil
	}

	// 未压缩格式，直接解析
	var messages []json.RawMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// GetStats 获取压缩统计
func (mc *MessageCompressor) GetStats() CompressionStats {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.stats
}
