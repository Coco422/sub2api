# 完整双应用模型 SSE：合法隔离 fixture 与正常结算前置

本切片基于已经双事件 CI 通过的 `e74c98dcf`。原 bootstrap、HTTP 七项/TLS 十项、全部硬内存/隔离/401/故障撤流/恢复/SIGTERM 断言保留。本轮实际新增代码与远端入口，不把已有 bootstrap 当模型验收。

## 实际链路

仍由已验证的 bootstrap 启动两个**未修改的完整 `backend/cmd/server`**：实际配置、Wire、PostgreSQL migrations、HTTP鉴权、gateway reader调度、真实Redis及真实Caddy。额外的 Go 工具 `backend/cmd/fullapp-fixture` 只有显式 `fullappfixture` build tag 才能构建，不进生产server/Release。它不是应用替身，只做隔离种子、有限状态读取和合成模型上游。

种子工具仅接受 stdin JSON，拒绝不以本轮生成 `s2-bootstrap-` 命名的 PG/Redis、非固定fixture数据库/端口，以及非internal fixture模型源；不从生产配置或宿主环境加载连接，不将密码/APIKey/DSN放argv/日志。失败只返回固定阶段。实际调用Ent/User/Group、APIKeyRepository和AccountRepository.CreateWithAccountGroups建立active用户、OpenAI标准公共组、APIKey、OpenAI api_key账号和组关系，root `parent_account_id=NULL/quota_dimension=global`；不删约束、迁移或修改账本迎合fixture。

账号只指向同一internal网络内合成模型容器，凭据虚构，模型仅 `gpt-6.1-sol`。从真实AccountRepository重新hydrate并检查资格后，经真实SchedulerCache的CaptureBucketWriteToken/SetSnapshot发布该group/openai/single快照。两个真实应用仍是gateway reader，不能临时切成双full或DB fallback绕过缺快照。

模型夹具为独立受限Go进程，接受实际应用发来的HTTP `/v1/responses`，校验合成上游认证和模型，输出合法Responses `response.created`、文本delta、`response.completed`、固定usage7in/2out及结束标记；只统计数字accept/complete/active，无请求日志。模型没有任何真实生成、互联网出口或付费服务。测试为此封闭网络显式允许HTTP/private fixture；应用生产默认、安全配置和TLS验证未改，不使用skip证书验证。

## 新回归实际要求

`test_fullapp_models.py` 复用原bootstrap仅新增无默认效果的价格/应用配置hook，原独立bootstrap CI步骤照常运行。新模型步骤从全新独占fixture开始，生成单独受限APIKey、合法种子及共享快照；本地定价改为合成**非零**成本，避免空账本路径也被当通过。

仅精确发送四次真实鉴权POST，**不重试模型请求来预热**。每次必须：

1. 经真实Caddy、两个完整应用中的一个，HTTP200/event-stream且含实际 `response.completed` 和合成输出；四次实际目标集合须覆盖两个应用。
2. 合成上游累计accepted/completed各恰为请求序号、active=0，不能把重放后最终200算通过。
3. 真实数据库该fixture的usage_logs和usage_billing_dedup累计各恰为请求序号，input/output token为7n/2n，真实用户balance出现非零合成扣减且范围合理。不是只调用账本repository的fake结果，也不改生产入账代码。
4. 通过真实ConcurrencyCache读取user/account/APIKey槽位均归零；下一次请求仍能走实际路径，不能只靠停止容器或TTL消失掩盖泄漏。12秒等待只读收敛，不重放POST/修改业务TTL或时钟。

报告只固定生成/用量/去重行计数及槽位数字，不包含Key、账户凭据、SQL正文、业务请求正文或内部网络地址。DB观察工具只查询本fixture实体范围，连接在stdin中，容器/匿名卷/network/临时文件沿原严格清理。模型容器额外256MiB、CPU0.5/pids128、readonly/capdrop/no-newpriv，不加入外部网络；原两应用及其他容器资源限制不变。

## 远端门禁与本机边界

`multihost-fullapp.yml` 保留原bootstrap步骤，随后在远端单编译槽构建显式tag工具：

```sh
cd backend
CGO_ENABLED=0 GOMAXPROCS=1 GOFLAGS=-p=1 go build -tags=fullappfixture -trimpath \
  -o <runner-temp>/sub2api-fullapp/fullapp-fixture ./cmd/fullapp-fixture
```

再独立执行 `python3 deploy/traffic/test_fullapp_models.py`。新入口仅加载新增正常SSE用例，不因继承重复执行原bootstrap suite；原suite仍由原步骤完整跑。普通production server没有测试tag和新增CLI能力。原Caddy binary固定版本/哈希不变，job35分钟与新模型步骤15分钟有界，所有动态编译/Go工具/PGRedis/Docker/模型fixture只在CI runner。

**本机仅gofmt、Python AST/YAML解析、bash语法和git静态检查，未执行/编译新增工具或测试。** 新精确head的双push/PR实际工具编译、合法种子阶段、`test_two_real_applications_complete_sse_once_and_settle_shared_state`真实执行/PASS/数字收尾及全部旧CI才是门禁。e74旧绿不证明新增模型链通过；新工具API签名/运行仍可能暴露缺口，失败应据固定阶段和原日志最小修，不skip/删断言/盲重跑。

## 明确后续而非已完成项

这个最小依赖前置先证明两真实应用的**正常SSE/真实结算/槽位收尾**。在途旧SSE drain、新请求切peer、accept后EOF/partial流不跨节点重放、客户端取消和失败/取消后的账本/并发收尾仍须沿此合法夹具新增受控故障模式及逐请求计数，不能拿正常SSE或旧Python上游七/十项当这些完整应用场景已经通过。当前没有自动恢复/重放、长期媒体/供应商新语义，也未修改生产账本或业务策略。

真实两主机、DNS、用户流量、出口字节及真实付费模型仍未验，不因隔离模型fixture就视为授权生产操作。
