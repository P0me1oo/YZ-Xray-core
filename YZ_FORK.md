# YZ-Xray-core 版本说明

当前产品版本为 `v26.9.1`（可信转发机名单），起点为已被 YZboard-Node `v1.17.2` 至 `v1.20.0` 固定消费的 `v26.8.1` / `f242ad6931521c1d56245e3566dcf7bc11b9571c`。产品版本独立于 Xray 上游版本；历史版本保持原名。Go 模块消费者应固定到验证后的完整提交。

## `v26.9.1` 空名单不恢复全来源信任

- 起点：本地 `67e631504e068fccd5345798be5d40abcb0ef124`，上游仍为 `v26.9.9`。配套 Node `v1.21.0`。
- 修正前一开发提交：清空名单后，即使旧的接收开关仍开启，也不解析任何来源的 PROXY 头。普通直连正常工作；名单外带头的数据按原始协议处理，不能伪装来源。
- 保留名单内可选带头、已有监听热更新和 IPv4 映射地址兼容。不改 UDP、出站或认证。普通链式代理不发送 PROXY 头时，继续按前置地址处理。
- 多实例嵌入时，Node 通过实例上下文给各监听传入独立名单；监听创建时绑定所属实例的名单对象，之后更新只影响该实例。旧的进程级设置接口仅供未传实例上下文的调用方兼容使用。TCP 和 Unix 工作线程不再丢弃创建时的实例上下文，否则监听会错误退回进程级名单。
- 本地相关验证：`transport/internet`、`websocket`、`httpupgrade` 与 `core` 包通过；新增空名单旧开关直连、清空名单后不能重新冒充来源的回归断言。
- 本版取代上一开发提交中空名单继承旧开关的行为；补丁删除条件沿用下节。

## `v26.9.0` 可信转发机名单

- 修改起点：`f242ad6931521c1d56245e3566dcf7bc11b9571c`（`v26.8.1`）。官方上游基线仍为 `v26.9.9`。
- Node 需求：节点前面有转发机时，转发机可以在连接开头附带 PROXY 头说明真实用户地址，节点据此计算设备数；同时不能让任何人伪造这个头冒充来源地址，也不能影响直连用户。
- 根因：上游 `acceptProxyProtocol` 开启后要求所有连接都带头，并且信任任何来源发来的头。直连用户因此全部连不上，绕开转发机直连的人还可以伪造来源地址。这个判断发生在内核接收连接的那一步，Node 无法在外层补上。
- 实现：新增 `transport/internet/yz_proxy_protocol.go`，提供进程级名单 `SetProxyProtocolTrustedPrefixes`，由嵌入方运行时整体替换；所有流式监听统一经过按来源判断的包装：
  - 名单为空：与上游完全相同，开启 `acceptProxyProtocol` 时要求带头，未开启时不解析；
  - 名单非空：名单内来源可选带头，带头时采用头里的地址，不带头照常处理；名单外来源不解析，连接原样交给协议层，与是否开启 `acceptProxyProtocol` 无关。
  - 不解析时直接返回原始连接，直连用户不经过额外包装，零拷贝等依赖原始连接的路径不受影响。名单变化对已建立的监听在下一次接收连接时生效，无需重载。
- 影响范围：只涉及 TCP 与 Unix 流式监听的接收阶段，UDP、QUIC、出站和协议认证不变。`testing/servers/tcp` 测试服务器不再假定监听一定是原始 TCP 类型；生产代码中没有其他此类假定。
- 删除条件：上游提供按来源限定信任 PROXY 头、且头可选的配置，并允许嵌入方在运行时更新名单后，可删除本补丁改用上游配置。
- 本地验证（Windows/amd64，Go 1.27.0）：新增 4 项用例覆盖空名单保持上游行为、名单内来源带头与不带头、名单外来源伪造无效、已有监听即时使用新名单及 IPv4 映射写法；`transport/internet`、`core`、`websocket`、`httpupgrade` 包通过。`go test ./...` 除 `app/dns` 的 `TestQUICNameServer` 外全部通过；该用例访问外部 DNS-over-QUIC 服务，本机网络经本地代理，恢复到修改前代码同样失败，与本次改动无关。本机未运行 `-race`。

## `v26.8.1` REALITY 双模式兼容（YZboard-Node `v1.17.2` 起固定消费）

- 修改起点：`6bf92ef6ac5f5c458e995510679cc499ab4e2d2a`。官方上游基线仍为 `v26.9.9`，不回退整个核心或覆盖既有 YZ 补丁。
- Node 需求：同一个 Reality 节点同时接收 sing-box 的传统握手和 mihomo 的混合握手，不增加节点开关、面板控制字段或 Node 业务逻辑。
- 根因：固定 REALITY 依赖要求客户端必须携带混合密钥项，并检查它在普通 X25519 之前；sing-box 当前客户端会移除混合项。现有后续 TLS 处理本身仍支持两种协商算法。
- 实现：将固定依赖的源码与许可证纳入 `third_party/reality`，五个传输入口统一使用该副本。认证入口允许传统、混合及两者共存，保持独立 X25519 认证优先级，并拒绝重复或长度错误的已知密钥项。保留时间、版本、短标识及认证检查，不强制改写目标站点协商结果。
- 使用源码副本是因为依赖模块自己的 `replace` 不会自动传递给 Node；内部导入保证消费者获得同一补丁，不依赖修改模块缓存。补丁来源和删除条件见 [底层兼容说明](third_party/README.zh.md)。
- 实际算法必须通过握手确认：允许混合握手不代表每次连接都使用抗量子算法，也不意味着错误认证可以回退成有效代理连接。
- 已验证：本地 Xray 客户端在两种目标站点下完成握手、32 KiB 双向传输及重连，并断言实际协商算法；固定 mihomo 和 YZ-sing-box 客户端完成两种目标站点下的传输、重连、混合并发和错误短标识认证测试。恢复上游检查的对照测试中，sing-box 再次认证失败，证明补丁覆盖了原问题。
- 互通测试位于独立的 `testing/realityinterop` 模块，不向核心引入 mihomo 或 sing-box 的运行时依赖。CI 增加 Linux `-race` 互通检查；新增 CI 尚未运行，不能将配置检查表述为 CI 已通过。
- Windows 工作区全仓库 `vformat` 检查报告大量未改文件。抽查 `app/app.go` 的工作区文本含 CRLF，而 Git 记录为 LF；本次改动的 40 个 Go 文件在隔离格式检查目录中通过相同工具的检查。未为制造绿色结果而重排全仓库文件。
- 本地核心 `third_party/reality`、`transport/internet/reality`、`core` 测试通过。扩大到 `transport/internet/...` 时，未修改的 Sudoku 端到端测试超过 5 分钟；单独复测时在内部 `go build ./main` 阶段超过 90 秒，尚未进入该测试的握手步骤，不能把这项测试记为通过。
- Node 使用临时本地源码替换验证：`internal/kernel/xray` 测试通过；`internal/kernel/singbox` 的运行时回显测试在 120 秒限制处超时，Node 整套回归未通过。临时替换文件已清理，Node 正式依赖仍固定在旧核心，不得把本地验证当作已发布 Node。
- 发布前须提交并固定新核心、更新 Node 正式依赖与构建来源、重新构建发布。开发验证使用临时相对路径替换，不能将该开发产物作为已发布版本使用。

## 上游基线

- 官方仓库：`XTLS/Xray-core`
- 官方预发布 Tag：`v26.9.9`
- Tag 对应 commit：`52a412d9e2f5c2a5142b1b4e2ab3771dacb8b120`
- 同步方式：将固定 Tag 合并到既有 YZ 分支，不跟随持续变化的 `main`

## 本次同步（2026-09-22）

- 修改起点：`b4caa82d6414196565599c19ebc1b53e331349b6`（历史版本 `v26.7.11-yz.6`），原上游基线为 `v26.7.11`。
- 合入范围：上游 `v26.7.28`、`v26.9.8`、`v26.9.9`，包括路由并发修复、HY2 更新、XHTTP 修复、REALITY 与相关依赖更新；不合入 Tag 之后的开发分支变更。
- 保留 YZ 用户统计、用户与落地流量归属、动态限速、SS2022 时间服务、缓冲写入统计、XUDP 关闭兼容、UDP 缓冲所有权和 HY2 会话关闭同步补丁。上游仍未包含这些补丁的完整等价实现。
- 构建要求提升为 Go 1.27，本地验证使用 Go 1.27.1；Node 必须同步工具链及固定依赖后才能使用新核心。
- 核心验证（2026-09-22，Windows/amd64，Go 1.27.1）：合并后的 `go.mod` 与上游 `v26.9.9` 完全一致；与上游的源码差异仅限 YZ 补丁文件。补齐 `resources/geoip.dat`、`resources/geosite.dat` 后 `go test ./...` 全部包通过，此前中断的 `app/router`、`infra/conf` 亦通过。`testing/scenarios` 的 `TestDomainSniffing` 在全量并行运行时因嗅探超时窗口偶发失败，单独运行该包两次均通过，官方 `v26.9.9` 在同一环境同样通过，判定为负载时序抖动而非合并问题。按仓库存储内容执行 `vformat` 格式检查和 proto 头检查通过；`go vet` 对 `proxy/shadowsocks_2022/outbound.go` 报告的上下文取消函数未调用属上游既有代码，本次不改动。Windows 本地未执行 `-race`。Node 联调、并发检测与目标平台构建结果由 YZboard-Node 的 `YZ_COMPATIBILITY.md` 记录。
- CI 核验（2026-09-22）：合并提交 `9fcf874e21147978c5117e4832c78b4adadd4320` 的「Tests and Checkings」在 ubuntu、macOS、windows 三个运行器全部成功，格式与 proto 头检查通过，「Build and Release」和 Windows 7 构建同样成功，补齐了本机未覆盖的 Linux 全量测试。
- 消费者固定：YZboard-Node `v1.17.0`（来源 `806a3db3911fc22967f37ce86c85d866c02ab31d`）已发布，`go.mod` 固定为 `github.com/P0me1oo/YZ-Xray-core v0.0.0-20260922055117-9fcf874e2114`，其 CI 的 `-race` 全量测试与双架构构建通过，发布产物核验见 Node 的 `YZ_COMPATIBILITY.md`。

## 当前三仓库兼容矩阵（`v26.8.0`）

| 项目 | 固定标识 |
| --- | --- |
| Xray 上游预发布 Tag / commit | `v26.9.9` / `52a412d9e2f5c2a5142b1b4e2ab3771dacb8b120` |
| YZ-Xray-core 源码版本 / commit | `v26.8.0` / `9fcf874e21147978c5117e4832c78b4adadd4320` |
| YZ-Xray-core 本次修改起点 | `b4caa82d6414196565599c19ebc1b53e331349b6`（历史版本 `v26.7.11-yz.6`） |
| YZboard-Node 消费版本 / 来源 | `v1.17.0` / `806a3db3911fc22967f37ce86c85d866c02ab31d` |
| YZboard-Node 固定的 Xray 模块 | `github.com/P0me1oo/YZ-Xray-core v0.0.0-20260922055117-9fcf874e2114` |
| 配套 sing-box 请求 / replacement | `v1.14.0` / `github.com/P0me1oo/YZ-sing-box v1.14.0-yz.2` |
| 构建工具链 | `Go 1.27.1`（`go.mod` 要求 `go 1.27`） |

## YZ 补丁

该补丁版本保留以下面板节点依赖能力：

- Hysteria2 在统计连接包装后仍能恢复 `MemoryUser`，维持邮箱、用户级别和 VLESS Route 信息；
- Dispatcher 按用户邮箱注册上传、下载和在线状态统计；
- 每用户共享限速器及动态更新能力；
- XUDP 关闭路径兼容统计和超时 Reader 包装；
- Hysteria2 用户热更新、统计和协议能力回归测试；
- SS2022 单用户入站、多人入站和出站从 Xray 创建上下文读取可选的 `ntp.TimeService`，供 YZboard-Node 在不修改系统时间的前提下校准协议时间。
- Dispatcher 为带 VLESS 路由的认证用户同时注册普通用户流量和“用户-落地路由”流量计数器，供 YZboard-Node 生成落地归属明细。

SS2022 时间补丁只改变 Shadowsocks 2022 构造器使用的时间函数。上下文没有注册时间服务时仍回退到库原有的系统时间行为；传统 Shadowsocks、VLESS、VMess、Trojan 和中转原始转发服务不受影响。

该补丁用于满足 YZboard-Node `v1.13-yz.16` 的前置 SS2022 出站、落地 SS2022 入站和用户-落地流量归属需求。若后续 Xray 上游原生支持这些能力，可以删除对应补丁并恢复上游实现。

## 历史补丁：`yz.6` HY2 UDP 会话关闭同步

Node 发布前完整并发测试在 `TestHysteria2RelayRuntime/salamander=false` 发现另一处关闭状态竞争：`udpSessionManager.run` 持有管理器锁写入 `closed`，定时清理任务却在加锁前读取它，见 [失败记录](https://github.com/P0me1oo/YZboard-Node/actions/runs/34155848942)。同一文件中，单个 `InterConn` 的写入与关闭也读写未同步的关闭标志。

本补丁让清理任务在读取管理器状态前持有读锁，并将单会话关闭标志改为原子状态。关闭不会等正在进行的数据报写入；重复关闭只释放一次接收通道，已经进入通道的数据可以读完，随后返回 EOF。关闭后新写入返回原有关闭错误，底层写入错误保持不变。

这两处都位于核心的 HY2 会话内部，Node 外层锁无法覆盖。补丁对应 Node `v1.13-yz.21` 的 HY2 中转和重载需求，不改变协议格式、认证、选路或流量口径；上游完成等价修复并通过相同回归后可删除。

新增回归覆盖清理任务退出、关闭后拒绝新会话、并发写入与关闭、阻塞写入时关闭、读取唤醒、错误传递、缓存排空和重复关闭。正式依赖需重新通过 Node 完整并发检测，不能沿用 `yz.5` 的局部验收结果。

2026-09-08，新增用例在 `yz.5` 上分别复现管理器和单会话的状态竞争；修复后 5 项测试在 Linux/amd64、Go 1.26.4 下连续运行 10 轮 `-race`，共 50 次通过，耗时 31.045 秒，无竞争报告。Windows/amd64 的 `transport/internet/hysteria`、`common/singbridge` 和 `core` 普通测试通过。Windows 本地并发检测器受工具链和运行时地址分配限制未能有效运行，并发结论来自 Linux 实测。

## `yz.5` UDP 缓冲与关闭同步

YZboard-Node 的 sing-box/Xray 混合中转并发测试发现：`PacketConnWrapper.ReadPacket` 拆分或替换 UDP 缓存时，另一协程的 `Close` 同时释放同一缓存。只在 Node 外层加锁无法覆盖核心内部的收发协程。

本补丁为读取建立顺序，并单独同步缓存所有权与关闭状态。阻塞读取不持有缓存锁，关闭可以立即释放已有缓存；关闭后才返回的数据会被释放，不再留下无人回收的缓存；重复关闭不会重复释放。多个 UDP 数据包仍保持顺序和各自目标地址，随最后一批数据返回的底层错误会在缓存排空后传递，不再被吞掉。

影响范围为使用该包装的 UDP 桥接路径，不改变协议格式、认证、路由或流量计数，也不接管外层连接的停止职责。对应 Node `v1.13-yz.21` 混合中转需求；后续上游提供等价缓存所有权与关闭同步、通过这些回归测试时，可删除本兼容补丁。

2026-09-08 使用 Go 1.26.4 在 Linux/amd64 执行新增包测试，修复前能复现缓存竞争和关闭后数据未释放；修复后连续 10 轮 `-race` 全部通过，共 30 项测试执行，无数据竞争。测试覆盖包顺序、目标地址、底层错误、读取阻塞时关闭、关闭后迟到数据、并发读取及重复关闭。

## `yz.4` 缓冲写入统计修复

YZboard-Node 的 HY2 前置入口测试使用原生客户端分别访问 Shadowsocks 和 VLESS 落地。
实际转发、用户套餐流量和用户-落地明细均正常，但 VLESS 首批缓冲写入没有计入 `relay-<ID>` 出站上传量。

根因位于 `common/buf.NewWriter`：它将 `stat.CounterConnection` 拆成底层连接与计数器，交给 `BufferToBytesWriter`。
原有 `WriteMultiBuffer` 会更新计数器，普通 `Write` 却直接继承底层连接的方法；`BufferedWriter` 刷新时也会走这个普通写入路径。

本补丁只为 `BufferToBytesWriter` 补上 `Write`，按底层返回的实际写入字节数更新已有计数器，并保留原返回值和错误。
部分成功后失败只累计已经写出的字节，零字节失败不增加流量；原有多缓冲区写入仍使用自身计数路径，不重复累计。
该改动修复所有采用这一包装方式的字节写入统计，不改变认证、路由、协议数据或面板报告结构。

对应需求是 Node 的 HY2/VLESS 中转入口能够准确上报各落地出站的上下行总量。
补丁中不包含面板或 Node 业务逻辑。后续上游修复相同路径并通过这些回归用例后，可以删除此兼容补丁。

`core.YZForkVersion` 同步标为 `v26.7.11-yz.4`，修正此前源码常量仍停留在 `yz.2` 的不一致；上游协议版本仍为 `26.7.11`。

## 发布与依赖约定

- 后续 Xray 二进制 Release 使用独立的三段语义版本 Tag，本次源码版本为 `v26.8.0`；
- 不创建或覆盖官方 Tag，也不改名或覆盖历史 YZ Tag；
- Go 模块消费者固定到明确的 YZ fork commit，并记录生成的 pseudo-version；
- 产品版本按修改影响递增，上游基线单独记录。

## 历史三仓库兼容矩阵（`v26.7.11-yz.6`）

| 项目 | 固定标识 |
| --- | --- |
| Xray 上游预发布 Tag / commit | `v26.7.11` / `50231eaff98ccc31b5cbd247a721c16e97fe5ec1` |
| YZ-Xray-core 源码版本 | `v26.7.11-yz.6`（实际提交以同名 Tag 和消费者的固定依赖为准） |
| YZ-Xray-core 本次修改起点 | `dcb690846b525851f0ee8dc47388e110d4600042`（包含 `yz.5` UDP 缓冲同步与 `yz.4` 缓冲写入统计修复） |
| YZ-Xray-core 既有 Tag / commit | `v26.7.11-yz.2` / `26b01717dd8d1fd604de5e23e2868fdef59eba2f` |
| YZboard-Node 上游基线 | `v1.13` / `0a29338e1f102a462363ce3527417029f89bab28` |
| YZboard-Node 本次 HY2 开发起点 | `94e2a76e42c1f059126b588b2d02f49e54fd8246`；消费者版本及集成状态由 Node 兼容矩阵记录 |
| YZboard 本次 HY2 开发起点 | `eff2fa22531f2e15168d3e7e96d8ab45639b1969`；面板版本及发布状态由面板兼容矩阵记录 |
| 本次 Node 配套 sing-box 请求 / replacement | `v1.14.0` / `github.com/P0me1oo/YZ-sing-box v1.14.0-yz.2` |

YZboard 没有独立的 Xray Tag；面板使用自身版本和代码 commit 回滚。Node 的 Xray `replace` 必须固定到本次修复提交对应的 fork pseudo-version，不得改为 `main` 或其他移动引用。

本次发布前必须确认 `xray version` 的上游版本为 `26.9.9`、YZ 产品版本为 `v26.8.0`，构建信息能够区分 YZ Tag 或 commit，并运行相关单元测试和目标平台构建。

## `yz.4` 本地验证（2026-09-07）

Go `1.26.4`、Windows 环境下，以下核心测试通过，直接使用修复后的源码，没有使用临时源码覆盖：

```text
go test -mod=readonly -count=1 ./common/buf ./core ./app/dispatcher ./app/stats/... ./proxy/hysteria/... ./proxy/vless/... ./transport/internet/stat -timeout 120s
```

新增计数回归覆盖显式刷新、自动刷新、普通字节写入、部分写入及失败、重复刷新、单个和多个缓冲区写入，以及未启用计数器的路径。

Node 的单独验证副本基于上表开发起点，带入 HY2 Xray 适配、运行测试和公共中转校验文件，通过临时 `validation.mod` 指向本地核心。
修复提交 `0de309feb5f4b949c4954914fa61ecd4935bbb8e` 推送后，再将验证依赖固定为远程 `github.com/P0me1oo/YZ-Xray-core v0.0.0-20260907143825-0de309feb5f4`，核对模块来源与校验值，并通过同一组测试。
以下 Xray 包测试通过，包含原生 HY2 客户端访问 Shadowsocks/VLESS 落地、TCP/UDP、Salamander、用户变更、重载和恢复，以及各类流量累计断言：

```text
go test -modfile validation.mod -mod=readonly -p 2 -count=1 -tags "with_quic with_utls with_wireguard with_acme with_clash_api" ./internal/kernel/xray -timeout 120s
```

`linux/amd64`、`linux/arm64` 和 `windows/amd64` 编译通过，Windows 版本命令确认上游版本 `26.7.11` 与 fork 构建标识 `v26.7.11-yz.4-0de309fe`。
该提交的构建已核对实际架构、`CGO_ENABLED=0`、`vcs.revision=0de309feb5f4b949c4954914fa61ecd4935bbb8e` 和 `vcs.modified=false`；构建校验值随独立验证记录保存。
本次未执行 Linux 运行测试；当前环境未启用 CGO，未执行 race 检查。所有 Node 转发测试监听均使用回环地址。

## `yz.4` CI 修正

首次推送后的 Build and Release 在资源缓存缺失时调用资源更新工作流，被 GitHub 以缺少 `actions: write` 权限拒绝。
同批全量测试的 `app/router`、`common/geodata`、`infra/conf` 和 Windows 7 打包均因缺少 GeoIP/GeoSite 资源失败。
本次为 `release.yml` 的 `check-assets` 作业单独授予触发工作流所需的权限，使原有资源准备流程能够执行；后续 CI 仍按原规则校验资源和运行完整测试。

`app/dispatcher/relay_user_test.go` 补齐已有方法之间的空行，满足 CI 使用的 gofumpt `v0.11.0` 格式要求。
该修正与资源权限修正不改变核心统计实现；最终 CI 结果以消费者固定提交对应的工作流记录为准。
