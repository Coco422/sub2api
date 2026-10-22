# 多主机会话归属：HTTP共享owner基础

本次仅完善Responses HTTP的**下游用户授权归属**，不实现原生WS的节点owner路由。现有`ValidateOpenAIHTTPResponseOwner`在转发带`previous_response_id`的HTTP请求前校验归属；响应成功后`bindHTTPResponseAccount`分别记录上游账号粘性及下游用户owner。账号相同不等于用户相同，不能用账号粘性替代授权。

## 共享记录与兼容

生产Redis适配层新增可选`OpenAIHTTPResponseOwnerCache`能力，不扩大既有`GatewayCache`接口，不新增部署开关。新writer用同一Lua原子写入一条`v1:<user_id>:<api_key_id>`记录及旧reader需要的两个整数兼容键，使用相同毫秒TTL。键仍按group隔离，response ID只以SHA-256摘要进入键，不保存请求正文、API Key字符串、上游凭据、节点地址或连接ID。

新reader在同一Lua内读取v1及兼容键：有v1时须与完整兼容键一致，仅当v1完全不存在时才按旧键对判断。旧记录缺一键则不授予续接；v1未知版本、损坏、与旧writer改写的兼容键冲突或Redis不可用时拒绝，不用本机热缓存或旧键绕过。新writer发布失败也不在本机留下授权。shared adapter路径每次授权读Redis，不把权威TTL延长到本地一分钟；本机缓存命中不能绕过到期、全格式删除或Redis故障。

同一用户的其他API Key继续沿用既有可续接语义，跨用户或跨group仍拒绝；不修改账本、并发、模型调用、HTTP错误格式或重试策略。没有共享cache的旧适配层和单进程用法保留旧行为，因此不承诺这些自定义adapter具有新原子契约。

## 升级与失效

新writer兼写旧格式，支持旧reader；新reader兼读旧writer留下的完整键对。滚动升级中旧writer本身仍分两次写，无法仅靠新reader保证其发布原子性；应完成全部writer升级后再宣称全路径原子。response ID作为上游生成的不可变归属标识，不能用相同ID跨用户重分配来迁移会话。

显式删除owner时必须删除v1和两条旧格式键；只删除v1会按兼容规则重新读取旧键，不表示撤销。若需要用户撤销/账号禁用，仍须沿用现有鉴权和撤销流程，本次不新增撤销API。TTL到期正常清理全部键，无数据库迁移；回滚旧二进制仍可读旧键，额外v1键按TTL自行过期。老二进制的本地TTL延长和双写局限也随回滚恢复。

## 支持范围与剩余缺口

| 状态 | 当前位置 | 本次承诺 |
| --- | --- | --- |
| HTTP tenant-owner | 共享Redis，可跨reader查询 | 原子发布、版本识别、权威TTL；上游账号及模型是否仍可续接另行判断 |
| response→account | 现有Redis及本地缓存 | 保留原逻辑，不把它当tenant授权 |
| 原生WS response→conn、turn state、session→conn | 进程内存和socket | 不共享、不转发；重启及换节点不能保证无损续接 |
| 图片/附件owner | 另项工作 | 本次无变更 |

因此本次不能称“全部会话可跨节点续接”，也不将DNS多A当成会话粘性。下一步需要稳定node ID/boot ID、明确WS状态保留期限、owner查找及安全拒绝/重建契约，再单独实现owner路由或受控恢复；图片归属随后独立推进。

## 回归与证据边界

新增仓储回归覆盖两个独立store共享、旧reader格式、新reader读旧键、缺键/未知版本/错误类型、组隔离、共享到期、发布失败、本地热节点共享故障和全格式删除。真实Redis检查接入现有`GatewayCacheSuite.TestHTTPResponseOwnerSharedRecord`；现有service用例继续检查同用户不同Key可续接、跨用户拒绝。

本机是生产宿主，本轮不运行Go编译、测试、构建。只做源码及非编译静态检查，全部回归结果等待精确提交的远端CI；新增测试源码不是已验证结果。没有访问生产DB/Redis或真实模型，不代表真实两机HTTP续接已验收。
