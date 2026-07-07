//go:build sqlite

package main

import (
	"fmt"

	"github.com/wly2lcl/basework/internal/permission"
	"github.com/wly2lcl/basework/pkg/config"
)

func openRuntimePermissionPersistence(cfg *config.Config) (*runtimePermissionPersistence, error) {
	storeType := cfg.Security.PermissionStore
	if storeType == "" {
		storeType = "sqlite"
	}
	if storeType == "memory" {
		return &runtimePermissionPersistence{}, nil
	}
	if storeType != "sqlite" {
		return nil, fmt.Errorf("不支持的权限存储: %q", storeType)
	}

	store, err := permission.NewSQLiteStore(getPermissionDBPath())
	if err != nil {
		return nil, fmt.Errorf("打开权限存储失败: %w", err)
	}
	auditLogger := permission.NewAuditLoggerWithDB(store.GetDB())
	return &runtimePermissionPersistence{
		Store:       store,
		AuditLogger: auditLogger,
		Cleanups: []func() error{
			store.Close,
			auditLogger.Close,
		},
	}, nil
}
