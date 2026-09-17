package index

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ScanState 保存 Talea 自己的来源快照和适配器游标。
type ScanState struct {
	AgentInstanceID string
	Fingerprint     string
	Cursor          string
	UpdatedAt       time.Time
}

// LoadScanState 读取指定 Agent 实例的来源状态。
func (db *DB) LoadScanState(ctx context.Context, instanceID string) (ScanState, bool, error) {
	var s ScanState
	var updated int64
	err := db.sql.QueryRowContext(ctx, `
		SELECT agent_instance_id, source_fingerprint, coalesce(cursor,''), updated_at
		FROM source_scan_state WHERE agent_instance_id = ?`, instanceID).
		Scan(&s.AgentInstanceID, &s.Fingerprint, &s.Cursor, &updated)
	if err == sql.ErrNoRows {
		return ScanState{}, false, nil
	}
	if err != nil {
		return ScanState{}, false, err
	}
	s.UpdatedAt = time.Unix(updated, 0)
	return s, true, nil
}

// SaveScanState 在本地索引中保存一次成功的来源同步状态。
func (db *DB) SaveScanState(ctx context.Context, s ScanState) error {
	if s.AgentInstanceID == "" || s.Fingerprint == "" {
		return fmt.Errorf("来源扫描状态不完整")
	}
	updated := s.UpdatedAt
	if updated.IsZero() {
		updated = time.Now()
	}
	_, err := db.sql.ExecContext(ctx, `
		INSERT INTO source_scan_state (agent_instance_id, source_fingerprint, cursor, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(agent_instance_id) DO UPDATE SET
			source_fingerprint=excluded.source_fingerprint,
			cursor=excluded.cursor,
			updated_at=excluded.updated_at`,
		s.AgentInstanceID, s.Fingerprint, s.Cursor, updated.Unix())
	return err
}

// FingerprintSources 对来源文件及其父目录做只读快照。
// 返回 valid=false 表示无法可靠判断，调用方应回退完整发现。
func FingerprintSources(root string, paths []string) (fingerprint string, valid bool, err error) {
	items := sourceFingerprintPaths(root, paths)
	h := sha256.New()
	for _, path := range items {
		if err := appendFileState(h, path); err != nil {
			return "", false, err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), true, nil
}

func sourceFingerprintPaths(root string, paths []string) []string {
	seen := map[string]bool{}
	add := func(path string) {
		if path != "" {
			seen[filepath.Clean(path)] = true
		}
	}
	root = filepath.Clean(root)
	add(root)
	for _, path := range paths {
		path = filepath.Clean(path)
		add(path)
		if strings.HasSuffix(path, ".db") {
			add(path + "-wal")
			add(path + "-shm")
		}
		for dir := filepath.Dir(path); dir != "." && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			add(dir)
			if root != "." && dir == root {
				break
			}
		}
	}
	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func appendFileState(h interface{ Write([]byte) (int, error) }, path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		_, _ = h.Write([]byte("missing\x00" + path + "\n"))
		return nil
	}
	if err != nil {
		return err
	}
	line := fmt.Sprintf("%s\x00%d\x00%d\x00%d\n", path, info.Mode(), info.Size(), info.ModTime().UnixNano())
	_, err = h.Write([]byte(line))
	return err
}
