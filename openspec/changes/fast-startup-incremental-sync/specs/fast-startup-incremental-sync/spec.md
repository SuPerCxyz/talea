## Purpose

让 Talea 在重复启动时优先使用自己的本地索引快速可用，并在后台以保守、可恢复的方式同步变化，减少无变化场景下的扫描、解析和数据库写入。

## ADDED Requirements

### Requirement: TUI SHALL show cached sessions before background synchronization completes

当 Talea 本地索引包含可读的会话记录时，TUI SHALL 先显示该缓存列表，不得要求用户等待完整同步完成后才能浏览列表。首次运行或索引不可读且没有可用缓存时，TUI SHALL 保留加载态并执行完整同步。

#### Scenario: Cached startup

- **WHEN** Talea 启动且本地索引包含会话记录
- **THEN** TUI 立即显示缓存列表，同时在后台执行同步，并明确表示列表可能正在更新

#### Scenario: First startup without cache

- **WHEN** Talea 启动且本地索引没有可用会话记录
- **THEN** TUI 显示加载状态并完成首次同步后显示列表

#### Scenario: Background synchronization fails

- **WHEN** 缓存列表已经显示但后台同步失败
- **THEN** TUI 保留缓存列表，显示同步失败或数据可能过期的反馈，并允许用户退出或继续查看缓存内容

#### Scenario: Refresh preserves selection

- **WHEN** 后台同步完成并刷新列表，且原选中会话仍然存在
- **THEN** TUI 保留该会话的选中状态；若会话不存在则选择合法的列表项

### Requirement: Talea SHALL detect source changes using only local read-only observations

Talea SHALL 将来源扫描状态保存在自己的索引库中，并可使用来源文件、目录和 OpenCode 数据库相关文件的本地元数据判断是否需要重新发现。Talea MUST NOT 修改、重命名或删除 Agent 原始会话文件或数据库。

#### Scenario: No source change

- **WHEN** 自上次成功同步后来源指纹和已知来源文件状态均未变化
- **THEN** Talea 跳过不必要的来源重新发现和会话解析，并继续使用已有索引

#### Scenario: Known source changes

- **WHEN** 已知会话文件或 OpenCode 数据库相关文件发生变化
- **THEN** Talea 重新检查受影响的来源并更新对应会话，不重新解析明确未变化的会话

#### Scenario: New or removed source cannot be ruled out

- **WHEN** 本地指纹缺失、损坏、不一致或无法可靠判断新增/删除来源
- **THEN** Talea 保守执行完整来源发现，不因快速路径而遗漏会话

### Requirement: OpenCode synchronization SHALL use a recoverable local high-water mark

Talea SHALL 在自己的索引库中保存 OpenCode 来源的 `(time_updated, session_id)` 高水位标记，并使用包含边界重叠的查询发现变化候选。高水位缺失、数据库路径变化、数据库状态不一致或查询结果不可靠时，Talea SHALL 回退到完整发现。

#### Scenario: Valid OpenCode cursor

- **WHEN** OpenCode 来源指纹发生变化且 Talea 存在有效高水位标记
- **THEN** Talea 只处理高水位之后及安全重叠窗口内的会话候选，并通过已有来源状态去重

#### Scenario: Same-timestamp updates

- **WHEN** 多个 OpenCode 会话具有相同的 `time_updated` 值
- **THEN** Talea 使用 `session_id` 辅助边界判断，不能因时间戳相同而漏掉候选会话

#### Scenario: Missing or invalid cursor

- **WHEN** OpenCode 高水位不存在或无法验证
- **THEN** Talea 执行完整发现并在成功后重新建立高水位

### Requirement: Repeated synchronization SHALL be idempotent and avoid unnecessary local writes

当来源没有变化时，Talea SHALL 不重复解析会话时间线，不重复写入相同的活动状态，并 SHALL 跳过不必要的全文索引同步。同步结果和搜索数据必须保持完整。

#### Scenario: Unchanged activity state

- **WHEN** 会话活动状态与本次检测结果相同
- **THEN** Talea 不为该会话执行无意义的状态更新

#### Scenario: Changed session remains searchable

- **WHEN** 某个会话的元数据发生变化并完成后台同步
- **THEN** 列表和全文搜索均能看到最新的索引字段

#### Scenario: FTS state is missing or incomplete

- **WHEN** 全文索引尚未建立、同步中断或被检测为不完整
- **THEN** Talea 执行必要的补齐或重建，不因快速路径返回不完整搜索结果

### Requirement: Cached list loading SHALL use persisted session metadata

加载已有缓存列表时，Talea SHALL 优先使用索引中已保存的工作目录存在性和 Git 字段，不得为每个未变化会话重复执行文件或 Git 检查。同步受影响会话时仍 SHALL 更新这些字段。

#### Scenario: Loading unchanged cached sessions

- **WHEN** TUI 读取未变化的缓存会话列表
- **THEN** 列表使用已索引字段完成展示，不因列表加载重新执行每个会话的 Git 探测

#### Scenario: Refreshing changed sessions

- **WHEN** 某个会话来源被重新解析
- **THEN** Talea 可以重新检查其工作目录和 Git 信息，并将结果写入本地索引

### Requirement: Fast synchronization SHALL preserve existing safety and failure isolation

快速路径和回退路径 SHALL 继续遵守本地优先、无默认联网、Agent 数据只读、单 Agent/会话/文件错误隔离和索引文件权限约束。

#### Scenario: Agent source is read-only

- **WHEN** Talea 执行快速检测或完整同步
- **THEN** Agent 原始文件和 OpenCode 数据库内容保持不变

#### Scenario: One source fails

- **WHEN** 单个 Agent、会话文件或来源查询失败
- **THEN** Talea 保留已有索引并继续处理其他可用来源，不退出整个应用
