# YZ-Xray-core 版本说明

当前 fork 版本为 `v26.7.11-yz.1`，对应 fork commit 为 `620bee93867095f73880056cdfb08bc54a15f69e`。

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
- Hysteria2 用户热更新、统计和协议能力回归测试。

## 发布与依赖约定

- Xray 二进制 Release 使用 `v<上游版本>-yz.N` Tag，本版本为 `v26.7.11-yz.1`；
- 不创建或覆盖官方 `v26.7.11` Tag；
- Go 模块消费者固定到明确的 YZ fork commit，并记录生成的 pseudo-version；
- 同一上游版本继续修订时递增 `yz.N`；同步到新上游版本后从 `yz.1` 重新开始。

## 三仓库兼容矩阵

| 项目 | 固定标识 |
| --- | --- |
| Xray 上游预发布 Tag / commit | `v26.7.11` / `50231eaff98ccc31b5cbd247a721c16e97fe5ec1` |
| YZ-Xray-core fork Tag / commit | `v26.7.11-yz.1` / `620bee93867095f73880056cdfb08bc54a15f69e` |
| YZboard-Node 上游基线 | `v1.13` / `0a29338e1f102a462363ce3527417029f89bab28` |
| YZboard-Node Release Tag / commit | `v1.13-yz.1` / `6fb176456c305f7aaad47c19f6acd7d1bca66d0b` |
| YZboard 兼容标识 / 代码 commit | `xray-v26.7.11-yz.1` / `342ceb5305af5df557fd85264a3157de84d233c5` |
| sing-box 请求 / 实际 replacement | `v1.13.2` / `github.com/cedar2025/sing-box v1.14.0-alpha.2.0.20260316103356-2e665cb7e295` |

YZboard 没有独立的 Xray Tag；面板使用自身版本和代码 commit 回滚。Node 的 Xray `replace` 必须固定到上表 fork pseudo-version，不得改为 `main` 或其他移动引用。

发布前必须确认 `xray version` 的上游版本为 `26.7.11`，构建信息能够区分 YZ Tag 或 commit，并运行相关单元测试和目标平台构建。
