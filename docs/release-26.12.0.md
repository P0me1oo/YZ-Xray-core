# YZ-Xray-core v26.12.0 发布

维护者：P0me1oo。日期：2026-11-16。

[v26.12.0](https://github.com/P0me1oo/YZ-Xray-core/releases/tag/v26.12.0) 已发布并设为 Latest，固定源码 `36f4516090e3e2fe88dcbe0840126582cd6fb795`。包含全部本地 DHT 识别、逐报文路由、缓冲生命周期及相关兼容修复；上游基线和原有补丁保留。

DHT、BT、dispatcher 竞态及 router 路由条件/重载/DNS 重入竞态通过；修复 UDP 测试服务器后十轮竞态和 core 全包通过，最终版本标识断言通过。Docker 工作流改为显式手动确认。

Linux amd64/arm64 两份程序及构建元数据从固定提交生成，已核对架构、CGO=0、VCS revision 和 vcs.modified=false。五份附件全部匿名下载，与本地产物及 GitHub 摘要一致。Agent v2.38.0 已锁定该核心；核心不单独替换运行服务器程序。

GitHub 自动工作流因账号账单锁定未执行，未将自动检查计为通过，使用本机验证和固定构建。不发布 Docker 镜像；发布标签及附件不因后补记录改变。详见[核验数据](release-26.12.0-verification.json)。
