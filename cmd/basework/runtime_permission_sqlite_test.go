//go:build sqlite

package main

import (
	"context"
	"testing"
	"time"

	"github.com/wly2lcl/basework/internal/permission"
	"github.com/wly2lcl/basework/pkg/config"
)

func TestRuntimePermissionCheckerUsesSQLiteStoreAndAudit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := config.NewStore("").Get()
	cfg.Permission.Enabled = true
	cfg.Permission.Mode = "interactive"
	cfg.Security.PermissionStore = "sqlite"

	checker, err := newPermissionChecker(cfg, nil)
	if err != nil {
		t.Fatalf("newPermissionChecker: %v", err)
	}
	defer closeRuntimeTestCleanups(t, checker.Cleanups)

	if checker.Core.Store == nil {
		t.Fatal("expected SQLite permission store")
	}
	if checker.Core.AuditLogger == nil {
		t.Fatal("expected SQLite audit logger")
	}

	err = checker.Core.Store.Create(&permission.StoredRule{
		RuleType: "allow",
		Pattern:  "read",
		Scope:    "global",
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	allowed, err := checker.Core.Check(context.Background(), "read", nil)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !allowed {
		t.Fatal("stored allow rule should allow the tool")
	}

	checker.Core.AuditLogger.Flush()
	records, err := checker.Core.AuditLogger.Query("", "read", time.Time{}, time.Time{}, 10)
	if err != nil {
		t.Fatalf("Query audit: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	if records[0].Decision != "allowed" {
		t.Fatalf("expected allowed audit decision, got %q", records[0].Decision)
	}
}

func closeRuntimeTestCleanups(t *testing.T, cleanups []func() error) {
	t.Helper()
	for i := len(cleanups) - 1; i >= 0; i-- {
		if err := cleanups[i](); err != nil {
			t.Fatalf("cleanup failed: %v", err)
		}
	}
}
