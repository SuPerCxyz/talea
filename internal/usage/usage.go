// Package usage 提供 Token 汇总与查询。
package usage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

// Load 读取指定会话的 Token 汇总。
func Load(ctx context.Context, db *index.DB, instanceID, sessionID string) (*model.TokenUsage, error) {
	row := db.SQL().QueryRowContext(ctx, `
		SELECT input_tokens, output_tokens, total_tokens,
		       cache_read_tokens, cache_write_tokens, reasoning_tokens, tool_tokens,
		       request_count, peak_context_tokens, max_input_tokens, max_output_tokens,
		       max_total_tokens, self_tokens, direct_child_tokens, descendant_tokens,
		       estimated_cost_micros, currency, pricing_model, pricing_snapshot_at,
		       usage_source, completeness, is_estimated, raw_fields_json
		FROM session_usage WHERE agent_instance_id=? AND session_id=?`, instanceID, sessionID)
	return scanUsage(row)
}

func scanUsage(row *sql.Row) (*model.TokenUsage, error) {
	var (
		u                                            model.TokenUsage
		input, output, total                         sql.NullInt64
		cacheR, cacheW, reason, tool                 sql.NullInt64
		reqCount, peak, maxIn, maxOut, maxTot        sql.NullInt64
		self, directChild, desc                      sql.NullInt64
		cost                                         sql.NullInt64
		snapshot                                     sql.NullInt64
		source, completeness, currency, pricing, raw sql.NullString
		estimated                                    int
	)
	err := row.Scan(&input, &output, &total,
		&cacheR, &cacheW, &reason, &tool,
		&reqCount, &peak, &maxIn, &maxOut,
		&maxTot, &self, &directChild, &desc,
		&cost, &currency, &pricing, &snapshot,
		&source, &completeness, &estimated, &raw)
	if err != nil {
		return nil, err
	}
	u.InputTokens = nn(input)
	u.OutputTokens = nn(output)
	u.TotalTokens = nn(total)
	u.CacheReadTokens = nn(cacheR)
	u.CacheWriteTokens = nn(cacheW)
	u.ReasoningTokens = nn(reason)
	u.ToolTokens = nn(tool)
	u.RequestCount = nn(reqCount)
	u.PeakContextTokens = nn(peak)
	u.MaxInputTokens = nn(maxIn)
	u.MaxOutputTokens = nn(maxOut)
	u.MaxTotalTokens = nn(maxTot)
	u.SelfTokens = nn(self)
	u.DirectChildTokens = nn(directChild)
	u.DescendantTokens = nn(desc)
	u.EstimatedCostMicros = nn(cost)
	u.Currency = currency.String
	u.PricingModel = pricing.String
	if snapshot.Valid {
		t := sqliteTime(snapshot.Int64)
		u.PricingSnapshotAt = &t
	}
	u.Source = model.UsageSource(source.String)
	u.Completeness = model.UsageCompleteness(completeness.String)
	u.IsEstimated = estimated != 0
	if raw.Valid && raw.String != "" {
		_ = json.Unmarshal([]byte(raw.String), &u.RawFields)
	}
	return &u, nil
}

func nn(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

func sqliteTime(sec int64) time.Time {
	return time.Unix(sec, 0)
}

// AddChild 将子会话 Token 聚合到父会话的 direct_child/descendant 字段。
func AddChild(ctx context.Context, db *index.DB, parentInstance, parentID, childInstance, childID string) error {
	child, err := Load(ctx, db, childInstance, childID)
	if err != nil {
		return err
	}
	if child == nil || child.TotalTokens == nil {
		return nil
	}
	_, err = db.SQL().ExecContext(ctx, `
		UPDATE session_usage SET
			direct_child_tokens = COALESCE(direct_child_tokens, 0) + ?,
			descendant_tokens = COALESCE(descendant_tokens, 0) + ?
		WHERE agent_instance_id=? AND session_id=?`,
		*child.TotalTokens, *child.TotalTokens, parentInstance, parentID)
	if err != nil {
		return fmt.Errorf("聚合子会话 Token: %w", err)
	}
	return nil
}
