# 用户流量记录

用户在「使用记录 → 流量记录」查看自己的 API 正文流量，管理员在「使用记录 → 流量记录」查看全站或指定用户。首版只统计，不改变 Token 计费、余额、套餐或收费规则。

## 计量范围

- 上传为网关从原始请求体读取的实际字节数；鉴权预读后重放给业务处理的正文不重复累计。未知 Content-Length 的请求仍按读取量计算。
- 下载为响应写入操作返回的字节数，包括 SSE 数据、心跳和错误响应。部分写入只累计实际返回的 `n`，不把整个计划响应作为已发送流量。
- 成功鉴权后，失败、取消、写入异常与 panic 也记录已观察到的流量。`completed` 仅表示 HTTP 处理结束，不证明 SSE 业务成功或远端完整接收。
- 未成功鉴权的请求不归属用户。HTTP 头、TLS/TCP 开销、重传、WebSocket、Live/WebRTC、外部媒体下载以及无 API Key 的能力链接下载不在首版范围内。返回媒体 URL 时只计算本站返回的正文。
- Serverless 由外部入口计量；经签名和重放保护验证的内部接收节点不重复计量。客户端伪造转发头或 Upgrade 头不能关闭普通 HTTP 计量。

只保存字节数、用户与 Key ID、路由模板、请求关联 ID、时间和结果；不保存正文、认证头、原始查询参数或能力链接令牌。单次请求按开始时间归属统计周期，在 HTTP 处理结束后生成记录，因此长请求和补偿写入会延迟反映在统计中。

## 日周月年查询

使用站点配置时区，日按小时、周和月按日、年按月展示；周一为一周开始。周期为左闭右开区间，支持闰年、夏令时及非整小时偏移。夏令时重复小时在趋势中保留不同 UTC 偏移。用户可切换历史周期、按 API Key ID 筛选并分页查看请求；管理员还可按用户 ID 筛选。

日期字段独立于时区解析，再映射到该日期的最早有效边界：午夜被跳过时取当天第一个有效时刻，午夜重复时取第一次。整日不存在的日期（例如 `Pacific/Apia` 的 2011 年 12 月 30 日）在日查询中返回 400，周/月趋势跳过空日并保持实际时间区间连续，避免日期推进停滞。

API 为 `GET /api/v1/usage/traffic` 和 `GET /api/v1/admin/usage/traffic`，参数如下：

| 参数 | 含义 |
| --- | --- |
| `period` | `day`、`week`、`month`、`year`，默认 `month` |
| `date` | 周期内的 `YYYY-MM-DD`，默认站点时区当天 |
| `api_key_id` | 可选正整数；用户接口始终叠加当前登录用户限制 |
| `user_id` | 仅管理员接口生效 |
| `page`、`page_size` | 默认 1、20；上限 10000、100 |

返回上传、下载、总字节数、请求数、完整时间桶、分页明细、是否有下一页、统计时区及当前筛选下第一条记录时间 `recorded_since`。未采集的历史流量无法推算，`recorded_since` 不是无损覆盖承诺。原始字节数为整数，前端只在显示时换算 KiB/MiB/GiB。

## 数据存储和补偿

新增迁移 `307_user_traffic.sql` 创建 `user_traffic_events` 与 `user_traffic_hourly`。事件使用独立服务端 UUID；相同 request ID 的客户端重试仍产生独立事件。数据库以一条原子 SQL 写入事件并累计小时桶，重放相同事件不会重复累计。查询使用一致性快照，小时桶覆盖完整 UTC 小时，原始事件补齐时区边界片段。

应用先把已完成请求的元数据写入数据目录下的 `user-traffic-pending/`，文件同步并原子改名后再异步落库。目录随项目数据目录解析：优先 `DATA_DIR`，其次可写的 `/app/data`，最后工作目录。目录创建权限为 0700，事件文件为 0600。初始化目录失败会中止服务启动。

部署时为此目录挂载持久卷并监控磁盘容量。队列通知满不会丢弃已落盘文件；数据库暂时不可用时保留文件并重试，进程重启后继续处理。停止后台补偿后，新完成请求仍可落盘，留待下次启动处理。不需要重放用户请求或再次调用上游。

- `delivery pending; will retry` 表示数据库投递尚未完成，检查数据库连接和待补偿文件数量。
- 本地落盘失败会尝试直接写数据库。两者都失败时输出 `METERING LOSS`，对应请求可能缺失，需处理存储故障。
- 损坏的文件保留为 `.json.invalid` 并记录日志，避免阻塞其他事件；人工修复后恢复 `.json` 后缀才能重试。
- 请求仍在进行时遇到进程被强制杀死，内存中的当前读写量可能丢失。首版不作为可直接扣款的账本；正式收费前需补齐在途恢复、完整性核验和对账。

首版不自动删除流量明细和汇总，诊断日志与 Token 用量清理不会联动删除这些表。不要直接清理事件表：事件 UUID 去重、非整小时边界查询和历史首条时间都依赖原始记录。后续保留策略需要同时设计归档、去重和汇总边界方案。

## 验证和页面复核

离线 mock 测试覆盖普通 HTTP、SSE、预读重放、未知长度、取消、部分写入、Flush 失败、panic、伪造头和内部转发。补偿测试覆盖数据库故障、重启和通知队列满。PostgreSQL 测试使用单连接 `pg_temp` 表，验证迁移重复执行、幂等汇总、事务回滚、用户隔离、分页、非整小时偏移和夏令时重复小时。

panic 请求在生产 Recovery 完成响应后结算，包含恢复时写入的错误 JSON；已经写出正文或连接断开时仍遵循原恢复行为，不追加错误正文。回归用例覆盖 500 正文计量、部分正文后 panic、broken pipe、连续请求不重复结算，以及跳日整月查询、午夜缺失与午夜重复。

后端定向检查：

```sh
cd backend
go test ./internal/service ./internal/repository ./internal/handler ./internal/server/middleware -run '^TestTraffic' -count=1
go test -race -p 2 ./internal/service ./internal/repository ./internal/handler ./internal/server/middleware -run '^TestTraffic' -count=1
```

PostgreSQL 用例需要显式设置指向隔离测试实例的 `TRAFFIC_TEST_POSTGRES_DSN`；未设置时该用例跳过。前端定向用例位于 `TrafficPanel.spec.ts`、`traffic.spec.ts` 和两个既有 `UsageView.spec.ts`，并运行中英文 locale key 检查。

页面交互复核：打开流量页签，切换日/周/月/年，选择历史日期，点击上一/下一周期，按 Key 查询；管理员增加用户筛选。核对上传、下载之和等于总流量，展开趋势明细，翻页后改变筛选应返回第一页。请求失败后旧结果不继续显示。

布局复核使用真实组件、项目样式及虚构本地数据，覆盖桌面和移动端深色布局；不代表生产数据或完整站点端到端验收。本地截图位于 `docs/screenshots/user-traffic/`，按仓库约定不入 Git，提交 PR 时作为附件上传。

本次本地验证：流量专项后端测试与 `-race` 均通过，已显式启用隔离 PostgreSQL 用例；受影响的路由、middleware、handler 单元测试通过，repository 与 IP 风控测试在 `TMPDIR=/tmp` 下复测通过。前端关键测试集 108 个文件、1751 项测试通过，完整 ESLint、`vue-tsc --noEmit` 和 Vite 生产构建通过。新增前端用例已加入根 Makefile 关键测试集。

在目标仓库 `production` 基线 `f348dba4a` 上执行 `TMPDIR=/tmp make -C backend test-unit` 已通过；流量专项竞态与隔离 PostgreSQL 用例也在该基线上复测通过。当前环境无 Docker 和 golangci-lint，未执行完整容器集成测试与该 lint。默认 pnpm 入口触发依赖和锁文件校验错误，前端验证直接调用现有 `node_modules/.bin` 工具，未改动依赖或锁文件。
