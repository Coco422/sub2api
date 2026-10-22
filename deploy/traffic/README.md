# 多主机脱敏清单离线预检

`preflight.py` 只读取手写的最小JSON清单，不读取应用配置、环境变量、密钥、数据库或Redis，不联网、解析DNS、创建文件或调用部署命令。示例地址使用`.invalid`，不是可部署环境。Python仅需标准库；回归使用标准库unittest。

```sh
python3 deploy/traffic/preflight.py deploy/traffic/multi-host.example.json
```

输出固定结构JSON：`ok`表示声明静态一致，`static_only: true`始终保留；退出0为静态检查通过、1为错误。错误只含索引路径和固定code，不回显节点名、地址、未知字段名称或输入值。不要传入真实config.yaml、环境文件或含凭据的JSON：未知字段拒绝只是防误用，不能替代提交前脱敏。清单最大256KiB，拒绝重复JSON字段。

## v1最小契约

- 2–16个节点、唯一安全id，一个full auto/writer，其余gateway auto/reader；不提供选主或writer自动接管。工具不修改应用默认行为；它检查的是本次有限的共享Redis部署约定。
- 每节点声明1–16个`scheduler_buckets`（如虚构`17:openai:single`）；reader须显式开启scheduler_readiness、覆盖所声明承接桶，writer声明覆盖reader的桶。年龄范围沿用应用0–86400秒。清单不证明发布者实际已发布或账号可调度。
- `capabilities`仅允许`http-responses`、`native-ws`、`bps-relay`，表示运营者希望承接的路径。v1不含WS bridge/passthrough、区域快照或其他协议；不支持的字段直接拒绝，不能借工具通过声称这些模式已支持。
- 声明native-ws须开启node-owner门禁并提供唯一node_id；这提供安全拒绝，不迁移socket、不自动重放。仍需要实际WS模式配置和运行路径验收。HTTP tenant-owner与node owner不是同一种授权。
- 声明bps-relay须显式提供本节点HTTPS origin，所有非空owner origin必须不同；默认443/大小写/末尾斜杠作同一origin处理。域名不同仍可能指向相同LB，工具无法识别，不作网络健康保证。原生附件file ID、异步S3输出不在此能力里。

示例`runtime`字段与应用对应，但这是独立部署清单的有限投影，不自动生成或更新应用配置；尤其不会把清单中的字段传入Compose/systemd。更换模板、节点规模或业务承接范围后，应再次维护清单，并单独核对运行配置一致。

## 验收与不覆盖范围

CI `release-helpers`执行`test_multihost_preflight.py`，覆盖合法双节点及HTTP-only、writer冲突、快照覆盖不足、WS身份重复、owner origin冲突/不安全、错误类型/未知凭据字段脱敏、重复JSON字段及CLI输出边界。实现本轮只做AST/JSON及git静态检查，测试结果待精确提交CI，不能把源码回归当执行通过。

通过不会使节点ready，不访问URL、不检查共享加密身份、DB/Redis连通、真实用户撤销/并发/入账幂等、DNS/TLS、模型出口或WS连接存活，不保证origin直达所属节点、透明续接、文件重启恢复或任何流量比例。下一步必须针对所选模式单独做获授权的隔离/真实验收。工具没有apply、DNS或入口供应商选项，不负责切流。

透明迁移仍需单独选择“客户端显式重建”或“服务器保存可重放状态”的产品语义；前者会终止旧链，后者涉及正文保留、有效期、重放幂等和费用归属。本工具不替用户作该选择。媒体长期持久化同样需明确capability有效期、用户撤销及对象删除规则，不能把临时relay变为无限生命周期。
