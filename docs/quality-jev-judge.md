# 质量运维复用 Jev 判题（Draft）

质量规则的 `quality.judge.kind` 省略或为 `llm` 时保持原来的普通分组模型判题。新增 `jev` 明确选择复用安全审计保存的 TypeSafe 配置，不接入普通账号选号，也不允许失败时回退普通模型。现有内置确定性判题路径保留。

Jev 使用已有原生 `/v1/systemone`，不是 Chat Completions 模型。公开协议来源：[TypeSafe OpenAPI](https://api.typesafe.ai/openapi.json)（2026-10-27 读取）；仓库实现见 `backend/internal/pkg/typesafe/client.go` 与 `systemone.go`。

- `noul` 是某个问题为是/真的概率，范围 `[0,1]`，不是通用质量分。
- `choice` 从 criteria 中选择概率最高的一项，附置信度及各项概率。
- `score` 是有序 criteria 等级下标的概率加权期望，可为小数，不能任意解读为百分制。

计划采用独立的质量 `choice`（correct / incorrect / unknown），不读取安全违规概率或审计阈值。题目、参考答案、候选回答作为有界数据；固定判题约束与站长 rubric 独立。模型判断仍可能受注入影响，结构隔离不能保证语义准确率。

复用语义：安全审计关闭、模式 off 或全局风控关闭时不发送 Jev 请求，返回 unknown 与明确原因；不会为了判题打开审计、修改其配置或触发审计副作用。缺少配置、凭据失败、限流、超时、取消和非法响应也只是不确定，不能作为质量不合格触发停调或作为恢复依据。

不增加账本、支付、结算或虚构的 Jev token/成本指标；未测量速度与成本，不承诺收益。当前实现与远端 CI 仍在进行，保持 Draft。本任务不调用真实 Jev，不部署、不合并。

回滚前先将 Jev 规则改回普通 LLM 或禁用；旧二进制不理解 `kind`，不能直接用旧版本执行 Jev 规则。持久化复用既有计划与判题 JSON 字段，无新增迁移。
