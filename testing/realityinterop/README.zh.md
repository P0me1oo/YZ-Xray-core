# REALITY 客户端互通测试

此独立测试模块不进入核心或 Node 的正式依赖。通过相对路径使用本仓库的开发源码，外部客户端固定版本保存在 `go.mod`、`go.sum`。

在本目录执行：

```powershell
go test -tags with_utls -count=1 -v .
```

测试使用 mihomo `fc8c5a24b16991f98cd736950c17d1aa306a5041` 和 YZ-sing-box `v1.14.0-yz.2` 的真实 REALITY 客户端实现，仅连接本机临时端口；每次随机生成身份、证书和数据，不写入配置文件。测试结束自动关闭服务和连接。

覆盖传统与混合两种目标站点、mihomo 保留和移除混合握手信息、sing-box 传统握手、双向 32 KiB 传输、连续重连、同节点混合并发连接及错误短标识认证。

这些测试验证 REALITY 握手与加密数据通道，不等同于真实服务器部署验收，也不代表所有历史客户端版本均兼容。核心自身的 `transport/internet/reality` 测试另行断言实际协商算法。
