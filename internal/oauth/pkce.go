package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// PKCEParams 包含 PKCE 流程所需的参数。
type PKCEParams struct {
	CodeVerifier  string
	CodeChallenge string
	State         string
}

// GenerateCodeVerifier 生成 cryptographically random 的 code verifier。
// verifier 长度为 43 字符，使用 unreserved 字符集（base64url 无填充）。
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成 code verifier 失败: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateCodeChallenge 使用 S256 方法生成 code challenge。
// 对 verifier 进行 SHA256 哈希，然后 base64url 编码（无填充）。
func GenerateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// GenerateState 生成随机的 state 参数，用于防止 CSRF 攻击。
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成 state 失败: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewPKCEParams 创建一组新的 PKCE 参数，包含 code verifier、challenge 和 state。
func NewPKCEParams() (*PKCEParams, error) {
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		return nil, err
	}

	state, err := GenerateState()
	if err != nil {
		return nil, err
	}

	return &PKCEParams{
		CodeVerifier:  verifier,
		CodeChallenge: GenerateCodeChallenge(verifier),
		State:         state,
	}, nil
}