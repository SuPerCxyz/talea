package opencode

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

// 存储表名与 StorageShape 返回的存储形态标识（实测 OpenCode 2.0.12）。
const (
	tableSession   = "session"
	tableSessionV2 = "session_v2"

	shapeSession   = "session"
	shapeSessionV2 = "session_v2"
)

// 发现查询拼装片段。无 session_v2 时输出与既有实现语义一致的传统查询。
const (
	discoverySelect      = `SELECT id, time_updated, coalesce(length(title),0) FROM `
	discoveryCutoffWhere = ` WHERE time_updated >= ? ORDER BY time_updated ASC, id ASC`

	v2RowsSQL = `SELECT id, time_updated, title FROM session_v2`
	// legacyOnlyRowsSQL 补回仅存在于传统 session 的遗留行；NOT EXISTS 保证同一会话
	// id 即使两表时间戳不同也不会被发现两次。
	legacyOnlyRowsSQL = `SELECT s.id, s.time_updated, s.title FROM session s ` +
		`WHERE NOT EXISTS (SELECT 1 FROM session_v2 v WHERE v.id = s.id)`
)

// sessionMetaColumns 是 session 与 session_v2 共有的元数据列。
const sessionMetaColumns = `id, directory, path, title, parent_id, model, agent, ` +
	`tokens_input, tokens_output, tokens_reasoning, ` +
	`tokens_cache_read, tokens_cache_write, time_created, time_updated`

// tableFlags 记录探测到的表存在性；零值表示探测失败或纯传统库。
type tableFlags struct {
	session   bool
	sessionV2 bool
}

// shape 返回存储形态标识。
func (t tableFlags) shape() string {
	if t.sessionV2 {
		return shapeSessionV2
	}
	return shapeSession
}

// probeTables 只读查询 sqlite_master，返回会话主表存在性。
func probeTables(ctx context.Context, db *sql.DB) (tableFlags, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name IN (?, ?)`,
		tableSession, tableSessionV2)
	if err != nil {
		return tableFlags{}, err
	}
	defer rows.Close()
	var flags tableFlags
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return tableFlags{}, err
		}
		switch name {
		case tableSessionV2:
			flags.sessionV2 = true
		case tableSession:
			flags.session = true
		}
	}
	return flags, rows.Err()
}

// probeTablesAt 以只读方式探测表存在性；任何失败都回退零值（传统表路径），
// 保证探测失败不中断发现、不导致应用退出。
func probeTablesAt(ctx context.Context, dbPath string) tableFlags {
	db, err := openRO(dbPath)
	if err != nil {
		return tableFlags{}
	}
	defer db.Close()
	flags, err := probeTables(ctx, db)
	if err != nil {
		return tableFlags{}
	}
	return flags
}

// StorageShape 返回 OpenCode 数据库存储形态：存在 session_v2 主表时为 "session_v2"，否则为 "session"。
// 无法只读打开数据库时返回错误。
func (a *Adapter) StorageShape(ctx context.Context, inst model.AgentInstance) (string, error) {
	return storageShapeAt(ctx, opencodeDatabasePath(ctx, inst))
}

// storageShapeAt 只读探测存储形态；打开或探测失败时返回错误。
func storageShapeAt(ctx context.Context, dbPath string) (string, error) {
	db, err := openRO(dbPath)
	if err != nil {
		return "", fmt.Errorf("只读打开 OpenCode 数据库失败: %w", err)
	}
	defer db.Close()
	flags, err := probeTables(ctx, db)
	if err != nil {
		return "", fmt.Errorf("探测 OpenCode 存储形态失败: %w", err)
	}
	return flags.shape(), nil
}

// discoverySQL 按表形态构造发现查询；cutoff 非 nil 时在外层施加高水位过滤，
// 保持既有游标与 5 分钟重叠窗口语义。无 session_v2 时与既有 SQL 一致。
func discoverySQL(flags tableFlags, cutoff *int64) (string, []any) {
	query := discoverySelect + discoverySource(flags)
	if cutoff == nil {
		return query, nil
	}
	return query + discoveryCutoffWhere, []any{*cutoff}
}

// discoverySource 返回发现查询的来源子句。
func discoverySource(flags tableFlags) string {
	switch {
	case flags.sessionV2 && flags.session:
		return `(` + v2RowsSQL + ` UNION ` + legacyOnlyRowsSQL + `)`
	case flags.sessionV2:
		return tableSessionV2
	default:
		return tableSession
	}
}

// discoverAt 按探测到的表形态执行发现查询；cutoff 非 nil 时施加高水位过滤。
// 探测失败回退传统 session 查询，由 discoverQuery 统一报告打开错误并保留旧索引。
func (a *Adapter) discoverAt(
	ctx context.Context,
	dbPath string,
	cutoff *int64,
) ([]adapters.SessionSource, error) {
	query, args := discoverySQL(probeTablesAt(ctx, dbPath), cutoff)
	return a.discoverQuery(ctx, dbPath, query, args...)
}

// scanSessionRow 优先从 session_v2 读取会话元数据；表不存在或行缺失时回退传统 session。
func scanSessionRow(ctx context.Context, db *sql.DB, sessionID string) (sessionRow, error) {
	flags, err := probeTables(ctx, db)
	if err != nil {
		flags = tableFlags{} // 探测失败按传统表处理
	}
	if flags.sessionV2 {
		row, qerr := querySessionRow(ctx, db, tableSessionV2, sessionID)
		if qerr == nil {
			return row, nil
		}
		if !errors.Is(qerr, sql.ErrNoRows) {
			return sessionRow{}, qerr
		}
	}
	return querySessionRow(ctx, db, tableSession, sessionID)
}

// querySessionRow 从指定会话表读取一行元数据；表名仅来自本包常量。
func querySessionRow(ctx context.Context, db *sql.DB, table, sessionID string) (sessionRow, error) {
	var s sessionRow
	err := db.QueryRowContext(ctx,
		`SELECT `+sessionMetaColumns+` FROM `+table+` WHERE id = ?`, sessionID).
		Scan(&s.ID, &s.Directory, &s.Path, &s.Title, &s.ParentID,
			&s.Model, &s.Agent, &s.TokensInput, &s.TokensOutput, &s.TokensReasoning,
			&s.TokensCacheRead, &s.TokensCacheWrite, &s.TimeCreated, &s.TimeUpdated)
	if err != nil {
		return sessionRow{}, err
	}
	return s, nil
}
