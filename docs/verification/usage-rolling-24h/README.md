# Admin usage「近24小时」验证记录

基线：`2af964fd589886ab9ba2d7e93bf6ac07c17aa8e1`（工作开始时的 `origin/production`）。分支：`fix/usage-rolling-24h`。本地 Node `v24.21.0`、pnpm `9.15.9`；CI 版本以仓库 workflow 为准。

## 链路与约定

已阅读根目录 `CONTRIBUTING.md`、PR 模板、Makefile 和 `.github/workflows/backend-ci.yml`；checkout 内及父目录未找到适用的 `AGENTS.md`。

原链路：`DateRangePicker.last24Hours` 和 admin `UsageView` 默认值把时间截成 `YYYY-MM-DD` → `/admin/usage`、`/admin/usage/stats` 将结束日期推进到次日零点 → repository 的 `created_at >= start AND created_at < end` 查询两个自然日。模型、趋势、分组、排行与拆分使用独立的 dashboard `parseTimeRange`，也需要支持精确时间。GET API 拦截器自动附带浏览器 IANA 时区。

修复后：admin usage 为 picker 开启 `rolling24Hours`，生成含毫秒的 ISO UTC 时间；在应用/筛选/刷新时统一捕获 `[now-24h, now)`，列表、卡片、模型、snapshot、错误及连接诊断共用边界。分页、排序、导出沿用已应用的窗口。其它 picker 使用方维持原有接口；其它预设和手工日期依旧是本地自然日。清理对话框继续接收本地日期，其清理 API 没有修改。

后端共同解析 YYYY-MM-DD / 带时区 RFC3339：仅日期型结束值使用 `AddDate(0,0,1)`，时间戳结束值原样作为排除边界。显式无效/倒置 dashboard 范围返回 400，不再静默切换默认区间。缓存键使用 RFC3339Nano。趋势仅在边界完全对齐 UTC 小时/日桶时使用预聚合，其余范围直接查原始日志，防止部分桶漏算或多算。

统计卡口径不变：总消费是 `total_actual_cost = SUM(actual_cost)`，标准费用是 `total_cost = SUM(total_cost)`，账户成本是 `SUM(COALESCE(account_stats_cost,total_cost) * COALESCE(account_rate_multiplier,1))`。没有修改账本、结算、支付或迁移。

## 实际 RED → GREEN

保存的日志仅移除了行尾空白以通过 diff 检查，测试输出、计数和退出码没有修改。

先写回归测试并实际运行以下 RED 命令，确认断言失败后才修改实现。完整原始输出见 [red.log](red.log)，退出码 **1**：**12 failed / 22 passed**（2 个文件）。关键差异：跨午夜期望 `2026-09-29T15:01:02.123Z` 至 `2026-09-30T15:01:02.123Z`，实际发送 `2026-09-30` 至 `2026-10-01`；picker 同样丢失时间，并将手工两日范围误标为 last24Hours。

```bash
TZ=Asia/Seoul pnpm --dir frontend exec vitest run \
  src/components/common/__tests__/DateRangePicker.rolling24h.spec.ts \
  src/views/admin/__tests__/UsageView.spec.ts --maxWorkers=1 --minWorkers=1
```

实现后先跑同一范围，剩余 2 个失败；修正首次渲染预设识别，并让初始页面测试推进其既有的 100ms 图表延迟。旧图表测试的时钟期望也改为断言推进计时器前捕获的已应用窗口。随后以下定向 GREEN 实际退出 **0**，**50 passed**（7 个文件），输出见 [green.log](green.log)：

```bash
TZ=Asia/Seoul pnpm --dir frontend exec vitest run \
  src/components/common/__tests__/DateRangePicker.rolling24h.spec.ts \
  src/components/common/__tests__/DateRangePicker.spec.ts \
  src/components/common/__tests__/DateRangePicker.dismiss.spec.ts \
  src/components/common/__tests__/DateRangePicker.midnight.spec.ts \
  src/views/admin/__tests__/UsageView.spec.ts \
  src/components/admin/usage/__tests__/UsageStatsCards.spec.ts \
  src/components/admin/usage/__tests__/UsageCleanupDialog.spec.ts \
  --maxWorkers=1 --minWorkers=1
```

另外实际运行纽约时区（覆盖 DST 本地语义），退出 **0**，**34 passed**（2 个文件），见 [green-new-york.log](green-new-york.log)：

```bash
TZ=America/New_York pnpm --dir frontend exec vitest run \
  src/components/common/__tests__/DateRangePicker.rolling24h.spec.ts \
  src/views/admin/__tests__/UsageView.spec.ts --maxWorkers=1 --minWorkers=1
```

覆盖午夜前后、毫秒、春/秋 DST 的严格 86,400,000ms、选择到应用期间跨午夜、初始/刷新/重置请求一致性、分页/导出窗口稳定、错误/连接精确边界、全部其它预设和手工范围的自然日语义、清理日期兼容及消费/标准费用/账户成本区分。新回归文件已加入 `FRONTEND_CRITICAL_VITEST`，供远端 CI 后续执行。

## 其它实际验证和限制

- `pnpm --dir frontend install --frozen-lockfile`：退出 0；没有改锁文件。
- `pnpm --dir frontend exec eslint src/components/common/DateRangePicker.vue src/views/admin/UsageView.vue src/components/common/__tests__/DateRangePicker.rolling24h.spec.ts src/views/admin/__tests__/UsageView.spec.ts src/components/admin/usage/__tests__/UsageStatsCards.spec.ts`：退出 0，无输出。
- `pnpm --dir frontend run typecheck`：**失败，退出 134**；Node 默认约 2 GB 堆内存耗尽，未完成类型验证。原始输出见 [typecheck.log](typecheck.log)，交远端 CI 继续验证，不计为通过。
- 对本次全部 Go 改动执行 `gofmt -w`，随后 `gofmt -l` 无输出（退出 0）；这是格式/语法检查，**不是编译或 Go 测试通过**。
- `git diff --check`：退出 0；提交前人工核对 diff 及范围。
- 本机**没有运行** Go 构建、`go test`、`go vet` 或 golangci-lint；没有连接生产、读取账务数据或凭据、部署或发布。全部测试输入为虚构数据。

Go handler 测试检查实际传给 repository 的上下界、显式时区不被用户时区覆写、日期型范围在 DST 的 23/25h 自然日兼容、错误边界、统计和 dashboard 的毫秒缓存隔离。SQL mock 测试检查部分桶直接查日志，且不把被忽略的预聚合错误误当成通过。集成测试使用现有隔离数据库事务，插入 start−1µs、start、end−1µs、end、end+1µs，断言仅中间两个记录进入列表、统计和趋势，同时区分三种费用。

以下为交给远端 CI 的命令，**不是本地已运行结果**；现有 CI 的 `make test-unit` / `make test-integration` 会包含这些测试：

```bash
cd backend
go test -tags=unit ./internal/handler/admin -run 'Test(AdminUsageTimeRange|UsageStatsCacheKeyPreservesTimePrecision|AdminDashboardRolling24HoursQueriesAndCachePrecision|AdminDashboardRejectsInvalidTimeRange)' -count=1
go test -tags=unit ./internal/repository -run '^TestUsageTrendRolling24HoursPreservesQueryBounds$' -count=1
go test -tags=integration ./internal/repository -run '^TestUsageLogRolling24HoursHalfOpenBoundaries$' -count=1
make test-unit
make test-integration
```

Agent：`codex-cli`，参与调查、测试、实现、验证和 PR。完整实际模型 ID 与推理强度均为**未知**：本次会话未暴露可核实的实际值，模型/推理强度专用环境变量也未提供；没有通过读取凭据或会话日志推测它们。
