package search

// defaultSearchLimit 是 Limit <= 0 时的默认返回上限。
// 注意：Limit 为 0 或负数不表示“不限制”，全量查询请显式设置 Unlimited。
const defaultSearchLimit = 200

// Query 描述搜索条件。
type Query struct {
	Term      string
	Agent     string
	Cwd       string
	Project   string
	Branch    string
	SinceDays int
	Limit     int
	// ExcludeSubagents 为 true 时在 SQL 层排除子 Agent 会话（is_subagent=0）。
	// 默认 false：不追加任何过滤条件，现有调用方（talea list 等在结果层
	// 自行按 IsSubagent 过滤）行为保持不变。
	ExcludeSubagents bool
	// Unlimited 为 true 时忽略 Limit 做全量查询（TUI 全量浏览列表）。
	// 默认 false：保持 Limit <= 0 时取 defaultSearchLimit 的既有语义。
	Unlimited bool
}

// limitClause 返回 LIMIT 子句及其绑定参数；Unlimited 时不追加截断。
func (q Query) limitClause() (string, []any) {
	if q.Unlimited {
		return "", nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	return " LIMIT ?", []any{limit}
}
