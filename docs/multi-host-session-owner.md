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

## 可选的原生WS node/boot安全门禁

新增默认关闭的配置（环境变量`RUNTIME_WS_NODE_OWNER_ENABLED`、`RUNTIME_WS_NODE_OWNER_NODE_ID`）：

```yaml
runtime:
  ws_node_owner:
    enabled: false
    node_id: "" # 开启时填写唯一且稳定的node-a等标识，1–64个小写字母/数字/-/_
```

开启的节点仅支持原生池化WS ingress，不接受WS HTTP bridge或passthrough，包括首帧为store=true的这些模式，以免后续切到store=false绕过门禁。HTTP Responses接口和其tenant-owner不受此开关影响。此功能约束原生ingress中实际store-disabled的`previous_response_id`续接；不承诺所有隐式session、工具状态或HTTP转WS传输都获得相同保护。

| 开关 | 原生池化 | WS HTTP bridge / passthrough |
| --- | --- | --- |
| 关闭（默认） | 保留旧续接/恢复策略，不读写node-owner marker | 保留原模式，可正常完成mock回归；不执行node-owner检查 |
| 开启 | 对store-disabled续接执行node/boot/连接归属检查 | store=true和store=false首帧均在上游发送前拒绝，不降级到其他模式 |

因此“桥接/透传保持原行为”只描述关闭状态，开启后是明确的能力拒绝，不是这些模式已获得owner保护。

服务实例首次使用时生成随机boot代次，与稳定node ID及原连接ID的SHA-256摘要组成内部v1 owner token；进程重启/重新构造服务会产生新代次。原生store-disabled响应成功后，以group、API Key、账号和response摘要为作用域发布Redis marker，TTL沿用现有response sticky TTL。声明使用原子compare-and-set：同owner可续期，另一node/boot或同boot的不同连接不能覆盖未过期owner。记录不包含客户正文、API Key字符串、明文连接ID、上游密钥或节点地址，不向客户端返回owner token。

续接必须同时满足共享marker匹配当前node/boot、API Key/账号/group作用域匹配、本地response→conn绑定存在，并实际获取/保留同一个池连接；Redis可达或账号粘性命中本身不构成准入。缺记录、旧boot、不同node/Key/账号/group、版本不匹配、Redis故障、本地绑定过期或原连接丢失均保守关闭。此门禁不替代已有鉴权、调度资格、计费和并发检查，不能把owner token当用户授权。

受保护路径在解析前核对显式previous ID；不允许handler或parser剥离ID后静默重建，连接探活失败及relay失败不自动重试/切账号。owner错误以固定WS1008返回，不输出owner/response信息，也不处罚上游账号。发布marker发生在现有AfterTurn完成之后；若模型已完成而marker发布失败，仅关闭连接，不重放、不重复调用或重记账。不要让客户端把该错误当成自动重放已完成请求的指令。

开启前先保证全节点都具备新适配层并为节点设置不同稳定ID；存量未带marker的链会被拒绝，建议单独原生WS灰度范围、先排空并由客户端显式开启新会话。缺新cache能力时首个store-disabled请求也拒绝，不用本机memory降级。关闭开关恢复旧行为；marker按TTL自动清理，不改DB schema、不需要删除生产数据。

新增config、仓储、service及原生WS假上游回归源码，覆盖相同boot继续两turn、未知owner在上游dial前拒绝、不同node/boot/Key/账号/group、marker过期及local conn丢失、失败脱敏、owner拒绝不进入failover/账号惩罚。**本轮均未运行，等待远端CI。** 仍未提供原节点发现、签名owner URL、跨节点转发/迁移、自动重建、生产网络故障或真实两机验收；此项是安全拒绝契约，不是“无损跨节点续接完成”。

补充的真实ingress回归仍只连接本机httptest WS和假上游：默认关闭的原生两turn；同node/boot存活连接成功；foreign boot、本地绑定丢失、绑定尚在但池连接失效、cache错误均零上游发送且不产生成功AfterTurn；模型成功后marker发布失败只有一次生成和一次成功AfterTurn。额外的模式矩阵覆盖两种非目标模式×开关开/关×store=true/false：关闭时实际完成假上游响应，开启时零dial/零HTTP发送/零回调。这里验证的是调用与回调计数，不是实际账本/支付集成验收；无生产DB或真实模型。连接获取前已进入turn准入的失败允许一次用于释放槽位的失败AfterTurn，不能当作成功计费。

## BPS临时图片/附件：本节点owner origin

原生BPS附件上传返回上游file ID，不由本节点提供文件下载。异步生图任务已有共享Redis的UserID/APIKeyID授权记录及S3兼容存储；本轮保持这些路径。另一个可选relay模式把输入图片临时写入本机私有目录，并由本进程索引/HMAC生成不可猜测的下载capability。两个节点共用全局图片BaseURL时，链接可能落到没有该文件的节点；共享Redis本身不能解决这个问题。

新增可信进程配置，默认空沿用既有全局图片BaseURL：

```yaml
runtime:
  image_relay_owner_origin: "" # 可填 https://node-a.example.invalid，必须直达生成节点
```

环境变量`RUNTIME_IMAGE_RELAY_OWNER_ORIGIN`（Compose须自行显式传入service）只接受HTTPS origin，可带端口或末尾单个斜杠，不接受userinfo、路径、query或fragment。它不采用请求Host/Forwarded头，不做DNS探测/修改，不自动配置TLS或开防火墙。接入方需保证该origin的DNS/TLS和现有BPS图片GET/HEAD路由直达当前节点；配置同一个轮询域名无法保证owner。

只有relay已启用且为relay模式时覆盖生成URL的origin；不改变全局设置或开启功能，不绕过现有全局settings有效性校验。全局relay BaseURL仍需是合法HTTPS origin，以兼容没有本节点覆盖的旧reader。新进程重复刷新共享配置时保留本节点origin。已有serverless的签名owner query装饰继续保留，不新建经旧节点转发正文的路径。

图片字节、HMAC、capability作用域、下载上限及过期规则不改，API Key字符串/客户正文/节点凭据不加入URL。只有已鉴权的BPS请求可以创建临时图片；下载URL本身是bearer capability，外部上游不携带用户Key即可取图，不能把它当普通公开静态目录，也不应记录或传播完整链接。别的节点和进程重启后没有对应索引/密钥，旧capability返回404，不自动回流旧机、不迁移文件。修改进程配置涉及既有重启/排空流程，本轮未执行。

新增离线源码回归覆盖两套独立relay分别生成各自origin、正确owner GET/HEAD及字节、外节点/篡改链接404、不同API Key作用域capability不同、共享设置刷新不覆盖本节点origin、重启旧链接404、默认空配置/禁用/native兼容。保留原有relay到期/存储上限回归。**新增测试未运行，待精确提交远端CI；本机只做非编译静态检查。** 此功能只提供临时relay直达owner URL，仍未统一所有图片/附件的存储、鉴权、生命周期或跨机恢复。
