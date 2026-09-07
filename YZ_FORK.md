# YZ-Xray-core 版本说明

当前源码目标版本为 `v26.7.11-yz.4`，尚未创建对应 Tag 或 Release。本次修改起点为 `601226e180d3684a5eabb8bc901c99f499398db1`，该提交包含上一批 `yz.3` 兼容补丁；既有 `v26.7.11-yz.2` Tag 对应 `26b01717dd8d1fd604de5e23e2868fdef59eba2f`。Go 模块消费者应固定到本次修复的完整提交，不能仅使用这里的源码目标版本。

## 上游基线

- 官方仓库：`XTLS/Xray-core`
- 官方预发布 Tag：`v26.7.11`
- Tag 对应 commit：`50231eaff98ccc31b5cbd247a721c16e97fe5ec1`
- 同步方式：将固定 Tag 合并到既有 YZ 分支，不跟随持续变化的 `main`

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

- Xray 二进制 Release 使用 `v<上游版本>-yz.N` Tag，本次源码目标版本为 `v26.7.11-yz.4`；
- 不创建或覆盖官方 `v26.7.11` Tag；
- Go 模块消费者固定到明确的 YZ fork commit，并记录生成的 pseudo-version；
- 同一上游版本继续修订时递增 `yz.N`；同步到新上游版本后从 `yz.1` 重新开始。

## 三仓库兼容矩阵

| 项目 | 固定标识 |
| --- | --- |
| Xray 上游预发布 Tag / commit | `v26.7.11` / `50231eaff98ccc31b5cbd247a721c16e97fe5ec1` |
| YZ-Xray-core 源码目标版本 | `v26.7.11-yz.4`（尚未发布；实际提交以 Git 和消费者的固定依赖为准） |
| YZ-Xray-core 本次修改起点 | `601226e180d3684a5eabb8bc901c99f499398db1`（上一批 `yz.3` 兼容补丁） |
| YZ-Xray-core 既有 Tag / commit | `v26.7.11-yz.2` / `26b01717dd8d1fd604de5e23e2868fdef59eba2f` |
| YZboard-Node 上游基线 | `v1.13` / `0a29338e1f102a462363ce3527417029f89bab28` |
| YZboard-Node 本次 HY2 开发起点 | `94e2a76e42c1f059126b588b2d02f49e54fd8246`；消费者版本及集成状态由 Node 兼容矩阵记录 |
| YZboard 本次 HY2 开发起点 | `eff2fa22531f2e15168d3e7e96d8ab45639b1969`；面板版本及发布状态由面板兼容矩阵记录 |
| HY2 验证使用的 sing-box 请求 / replacement | `v1.14.0` / `github.com/P0me1oo/YZ-sing-box v1.14.0-yz.1` |

YZboard 没有独立的 Xray Tag；面板使用自身版本和代码 commit 回滚。Node 的 Xray `replace` 必须固定到本次修复提交对应的 fork pseudo-version，不得改为 `main` 或其他移动引用。

发布前必须确认 `xray version` 的上游版本为 `26.7.11`，构建信息能够区分 YZ Tag 或 commit，并运行相关单元测试和目标平台构建。

## `yz.4` 本地验证（2026-09-07）

Go `1.26.4`、Windows 环境下，以下核心测试通过，直接使用修复后的源码，没有使用临时源码覆盖：

```text
go test -mod=readonly -count=1 ./common/buf ./core ./app/dispatcher ./app/stats/... ./proxy/hysteria/... ./proxy/vless/... ./transport/internet/stat -timeout 120s
```

新增计数回归覆盖显式刷新、自动刷新、普通字节写入、部分写入及失败、重复刷新、单个和多个缓冲区写入，以及未启用计数器的路径。

Node 的单独验证副本基于上表开发起点，带入 HY2 Xray 适配、运行测试和公共中转校验文件，通过临时 `validation.mod` 指向本地核心。
以下 Xray 包测试通过，包含原生 HY2 客户端访问 Shadowsocks/VLESS 落地、TCP/UDP、Salamander、用户变更、重载和恢复，以及各类流量累计断言：

```text
go test -modfile validation.mod -mod=readonly -p 2 -count=1 -tags "with_quic with_utls with_wireguard with_acme with_clash_api" ./internal/kernel/xray -timeout 120s
```

`linux/amd64`、`linux/arm64` 和 `windows/amd64` 编译通过，Windows 版本命令确认上游版本 `26.7.11` 与 fork 构建标识 `v26.7.11-yz.4-dev`。
开发构建已检查实际架构、`CGO_ENABLED=0` 和 VCS 来源；消费者固定远程提交后仍应核对模块解析和运行测试。
本次未执行 Linux 运行测试；当前环境未启用 CGO，未执行 race 检查。所有 Node 转发测试监听均使用回环地址。
