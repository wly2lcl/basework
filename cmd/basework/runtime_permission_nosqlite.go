//go:build !sqlite

package main

import (
	"fmt"

	"github.com/wly2lcl/basework/pkg/config"
)

func openRuntimePermissionPersistence(cfg *config.Config) (*runtimePermissionPersistence, error) {
	storeType := cfg.Security.PermissionStore
	if storeType == "" || storeType == "sqlite" || storeType == "memory" {
		return &runtimePermissionPersistence{}, nil
	}
	return nil, fmt.Errorf("不支持的权限存储: %q", storeType)
}
