package oauth

// Config 是 OAuth 认证模块的配置。
type Config struct {
	Enabled        bool                      `json:"enabled"`
	StorageBackend string                    `json:"storage_backend"` // "file" 或 "keychain"
	CallbackPort   int                       `json:"callback_port"`   // 默认 8080
	Providers      map[string]ProviderConfig `json:"providers"`
}

// DefaultConfig 返回默认 OAuth 配置。
func DefaultConfig() Config {
	return Config{
		Enabled:        false,
		StorageBackend: "file",
		CallbackPort:   8080,
		Providers:      make(map[string]ProviderConfig),
	}
}
