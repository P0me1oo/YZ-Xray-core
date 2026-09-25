package internet

import (
	"net/netip"
	"sync/atomic"

	"github.com/pires/go-proxyproto"
	"github.com/xtls/xray-core/common/net"
)

// YZ 补丁：可信转发机名单。
//
// 名单内的来源可以在连接开头附带 PROXY 头，内核用头里的真实地址作为连接来源；
// 不带头的连接照常处理。名单外的来源不解析 PROXY 头，连接原样交给协议层，
// 伪造的头会被当作普通数据而认证失败，无法冒充来源地址。
//
// 名单由嵌入方（YZboard-Node）在运行时整体替换，已建立的监听在下一次接收连接时
// 使用新名单，无需重载内核。名单为空时保留上游 acceptProxyProtocol 的原有行为。
var proxyProtocolTrusted atomic.Pointer[[]netip.Prefix]

// SetProxyProtocolTrustedPrefixes 整体替换可信转发机名单，传空表示清空。
func SetProxyProtocolTrustedPrefixes(prefixes []netip.Prefix) {
	list := make([]netip.Prefix, 0, len(prefixes))
	for _, prefix := range prefixes {
		if !prefix.IsValid() {
			continue
		}
		addr, bits := prefix.Addr(), prefix.Bits()
		if addr.Is4In6() && bits >= 96 {
			addr, bits = addr.Unmap(), bits-96
		}
		list = append(list, netip.PrefixFrom(addr, bits).Masked())
	}
	if len(list) == 0 {
		proxyProtocolTrusted.Store(nil)
		return
	}
	proxyProtocolTrusted.Store(&list)
}

// proxyProtocolPolicy 决定单个连接如何处理 PROXY 头。
//   - 名单为空：沿用上游行为，开启 acceptProxyProtocol 时要求所有连接携带头，否则不解析；
//   - 名单非空：名单内来源可选携带头，名单外来源不解析，与是否开启 acceptProxyProtocol 无关。
func proxyProtocolPolicy(upstream net.Addr, legacyRequire bool) proxyproto.Policy {
	list := proxyProtocolTrusted.Load()
	if list == nil {
		if legacyRequire {
			return proxyproto.REQUIRE
		}
		return proxyproto.SKIP
	}
	tcp, ok := upstream.(*net.TCPAddr)
	if !ok {
		return proxyproto.SKIP
	}
	addr := tcp.AddrPort().Addr().Unmap()
	for _, prefix := range *list {
		if prefix.Contains(addr) {
			return proxyproto.USE
		}
	}
	return proxyproto.SKIP
}

// wrapProxyProtocolListener 给流式监听加上按来源判断的 PROXY 头处理。
// SKIP 直接返回原始连接，名单外的直连用户不经过任何额外包装。
func wrapProxyProtocolListener(l net.Listener, legacyRequire bool) net.Listener {
	return &proxyproto.Listener{
		Listener: l,
		ConnPolicy: func(options proxyproto.ConnPolicyOptions) (proxyproto.Policy, error) {
			return proxyProtocolPolicy(options.Upstream, legacyRequire), nil
		},
	}
}
