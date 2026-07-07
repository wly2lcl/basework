//go:build sqlite

package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RecoveryStatus 表示恢复操作的状态
type RecoveryStatus struct {
	SessionID  string
	Corrupted  bool
	Repaired   bool
	BackupPath string
	Error      error
	CheckedAt  time.Time
}

// CheckIntegrity 检查会话数据库的完整性
func (s *SQLiteStore) CheckIntegrity() (*RecoveryStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := &RecoveryStatus{
		SessionID: filepath.Base(s.path),
		CheckedAt: time.Now(),
	}

	// 运行 PRAGMA integrity_check
	var result string
	err := s.db.QueryRow("PRAGMA integrity_check").Scan(&result)
	if err != nil {
		status.Corrupted = true
		status.Error = fmt.Errorf("integrity check failed: %w", err)
		return status, nil
	}

	if result != "ok" {
		status.Corrupted = true
		status.Error = fmt.Errorf("database corrupted: %s", result)
		return status, nil
	}

	return status, nil
}

// AttemptRecovery 尝试从 WAL 文件恢复数据
func (s *SQLiteStore) AttemptRecovery() (*RecoveryStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := &RecoveryStatus{
		SessionID: filepath.Base(s.path),
		CheckedAt: time.Now(),
	}

	// 1. 检查当前完整性
	var integrityResult string
	err := s.db.QueryRow("PRAGMA integrity_check").Scan(&integrityResult)
	if err != nil || integrityResult != "ok" {
		status.Corrupted = true
		status.Error = fmt.Errorf("database corrupted: %v", err)
	} else {
		// 数据库完整，无需恢复
		return status, nil
	}

	// 2. 创建备份
	backupPath := s.path + ".recovery-backup-" + fmt.Sprintf("%d", time.Now().Unix())
	status.BackupPath = backupPath

	data, err := os.ReadFile(s.path)
	if err != nil {
		status.Error = fmt.Errorf("backup failed: %w", err)
		return status, err
	}

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		status.Error = fmt.Errorf("write backup failed: %w", err)
		return status, err
	}

	// 3. 尝试从 WAL 恢复
	// SQLite 的 WAL 恢复是自动的，只需确保 WAL 文件存在
	walPath := s.path + "-wal"
	if _, err := os.Stat(walPath); os.IsNotExist(err) {
		status.Error = fmt.Errorf("WAL file not found, cannot recover")
		return status, status.Error
	}

	// 4. 重新打开数据库（触发 WAL 恢复）
	s.db.Close()
	db, err := sql.Open("sqlite", s.path)
	if err != nil {
		status.Error = fmt.Errorf("reopen database failed: %w", err)
		return status, err
	}
	s.db = db

	// 5. 验证恢复结果
	err = db.QueryRow("PRAGMA integrity_check").Scan(&integrityResult)
	if err != nil || integrityResult != "ok" {
		status.Error = fmt.Errorf("recovery failed: database still corrupted")
		return status, status.Error
	}

	status.Repaired = true
	return status, nil
}

// ListCorruptedSessions 列举所有损坏的会话
func ListCorruptedSessions(sessionDir string) ([]*RecoveryStatus, error) {
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("read session directory failed: %w", err)
	}

	var corrupted []*RecoveryStatus
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".db" {
			continue
		}

		dbPath := filepath.Join(sessionDir, entry.Name())
		store, err := NewSQLiteStore(dbPath)
		if err != nil {
			continue
		}

		status, err := store.CheckIntegrity()
		store.Close()
		if err != nil {
			continue
		}

		if status.Corrupted {
			corrupted = append(corrupted, status)
		}
	}

	return corrupted, nil
}
