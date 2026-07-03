package oauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store 是 OAuth token 存储接口，支持不同的后端实现。
type Store interface {
	// Save 保存指定 provider 的 token。
	Save(provider string, token *Token) error

	// Load 加载指定 provider 的 token。
	Load(provider string) (*Token, error)

	// Delete 删除指定 provider 的 token。
	Delete(provider string) error

	// List 返回所有已存储的 provider 名称列表。
	List() ([]string, error)
}

// FileStore 使用 AES-GCM 加密文件存储 token。
// 加密密钥自动生成并保存在 token 目录下的 .encryption_key 文件中。
type FileStore struct {
	tokenDir string
}

// NewFileStore 创建新的文件存储。
// tokenDir 是 token 加密文件存放的目录，如果不存在会自动创建。
func NewFileStore(tokenDir string) (*FileStore, error) {
	if err := os.MkdirAll(tokenDir, 0700); err != nil {
		return nil, fmt.Errorf("创建 token 目录失败: %w", err)
	}
	return &FileStore{tokenDir: tokenDir}, nil
}

// getKeyPath 返回加密密钥文件路径。
func (fs *FileStore) getKeyPath() string {
	return filepath.Join(fs.tokenDir, ".encryption_key")
}

// getTokenPath 返回指定 provider 的加密 token 文件路径。
func (fs *FileStore) getTokenPath(provider string) string {
	return filepath.Join(fs.tokenDir, fmt.Sprintf("%s_token.json.enc", provider))
}

// ensureKey 确保加密密钥存在，不存在则生成新的 256 位密钥。
func (fs *FileStore) ensureKey() ([]byte, error) {
	keyPath := fs.getKeyPath()
	key, err := os.ReadFile(keyPath)
	if err == nil && len(key) == 32 {
		return key, nil
	}

	// 生成新的 256 位随机密钥
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("生成加密密钥失败: %w", err)
	}

	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return nil, fmt.Errorf("保存加密密钥失败: %w", err)
	}

	return key, nil
}

// encrypt 使用 AES-GCM 加密明文数据。
// 返回格式：nonce（12 字节）+ ciphertext + auth tag。
func (fs *FileStore) encrypt(plaintext []byte) ([]byte, error) {
	key, err := fs.ensureKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 AES 密码失败: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("生成 nonce 失败: %w", err)
	}

	// Seal 返回 nonce + ciphertext + auth tag
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt 使用 AES-GCM 解密密文数据。
// 输入格式：nonce（12 字节）+ ciphertext + auth tag。
func (fs *FileStore) decrypt(ciphertext []byte) ([]byte, error) {
	key, err := fs.ensureKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 AES 密码失败: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("密文太短")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("解密失败: %w", err)
	}

	return plaintext, nil
}

// Save 加密保存指定 provider 的 token。
func (fs *FileStore) Save(provider string, token *Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("序列化 token 失败: %w", err)
	}

	encrypted, err := fs.encrypt(data)
	if err != nil {
		return err
	}

	if err := os.WriteFile(fs.getTokenPath(provider), encrypted, 0600); err != nil {
		return fmt.Errorf("写入 token 文件失败: %w", err)
	}

	return nil
}

// Load 加载并解密指定 provider 的 token。
func (fs *FileStore) Load(provider string) (*Token, error) {
	encrypted, err := os.ReadFile(fs.getTokenPath(provider))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("provider %q 未授权: token 文件不存在", provider)
		}
		return nil, fmt.Errorf("读取 token 文件失败: %w", err)
	}

	data, err := fs.decrypt(encrypted)
	if err != nil {
		return nil, err
	}

	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("解析 token 失败: %w", err)
	}

	return &token, nil
}

// Delete 删除指定 provider 的 token 文件。
func (fs *FileStore) Delete(provider string) error {
	if err := os.Remove(fs.getTokenPath(provider)); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("删除 token 文件失败: %w", err)
	}
	return nil
}

// List 返回所有已存储的 provider 名称列表。
func (fs *FileStore) List() ([]string, error) {
	entries, err := os.ReadDir(fs.tokenDir)
	if err != nil {
		return nil, fmt.Errorf("读取 token 目录失败: %w", err)
	}

	suffix := "_token.json.enc"
	var providers []string
	for _, entry := range entries {
		name := entry.Name()
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			providers = append(providers, name[:len(name)-len(suffix)])
		}
	}

	return providers, nil
}

// KeychainStore 是系统密钥链存储的占位实现。
// TODO: 对接 macOS Keychain、Windows Credential Manager 或 Linux Secret Service。
type KeychainStore struct {
	serviceName string
}

// NewKeychainStore 创建新的密钥链存储。
// serviceName 是密钥链中的服务标识名称。
func NewKeychainStore(serviceName string) *KeychainStore {
	return &KeychainStore{serviceName: serviceName}
}

// Save 保存 token 到系统密钥链。
func (ks *KeychainStore) Save(provider string, token *Token) error {
	return fmt.Errorf("密钥链存储尚未实现: provider=%s", provider)
}

// Load 从系统密钥链加载 token。
func (ks *KeychainStore) Load(provider string) (*Token, error) {
	return nil, fmt.Errorf("密钥链存储尚未实现: provider=%s", provider)
}

// Delete 从系统密钥链删除 token。
func (ks *KeychainStore) Delete(provider string) error {
	return fmt.Errorf("密钥链存储尚未实现: provider=%s", provider)
}

// List 返回密钥链中所有已存储的 provider 名称列表。
func (ks *KeychainStore) List() ([]string, error) {
	return nil, fmt.Errorf("密钥链存储尚未实现")
}