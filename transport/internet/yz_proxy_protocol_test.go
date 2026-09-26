package internet_test

import (
	"context"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/xtls/xray-core/transport/internet"
)

const (
	yzProxyHeader  = "PROXY TCP4 203.0.113.9 127.0.0.1 40000 443\r\n"
	yzProxyPayload = "hello"
)

type yzProxyResult struct {
	remote string
	data   string
}

// yzProxyExchange 建立一条回环连接，可选先发送 PROXY 头，返回服务端看到的来源和数据。
func yzProxyExchange(t *testing.T, l net.Listener, withHeader bool) yzProxyResult {
	t.Helper()
	sent := yzProxyPayload
	if withHeader {
		sent = yzProxyHeader + yzProxyPayload
	}
	done := make(chan error, 1)
	go func() {
		conn, err := net.Dial("tcp", l.Addr().String())
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		if _, err = io.WriteString(conn, sent); err == nil {
			// 半关闭让服务端读到结尾，再等服务端关闭。
			err = conn.(*net.TCPConn).CloseWrite()
		}
		done <- err
		_, _ = io.Copy(io.Discard, conn)
	}()
	conn, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	remote := conn.RemoteAddr().(*net.TCPAddr).IP.String()
	data, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	return yzProxyResult{remote: remote, data: string(data)}
}

func yzProxyListen(t *testing.T, accept bool) net.Listener {
	t.Helper()
	var sockopt *internet.SocketConfig
	if accept {
		sockopt = &internet.SocketConfig{AcceptProxyProtocol: true}
	}
	l, err := internet.ListenSystem(context.Background(), &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, sockopt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func yzSetTrusted(t *testing.T, prefixes ...string) {
	t.Helper()
	list := make([]netip.Prefix, 0, len(prefixes))
	for _, p := range prefixes {
		list = append(list, netip.MustParsePrefix(p))
	}
	internet.SetProxyProtocolTrustedPrefixes(list)
	t.Cleanup(func() { internet.SetProxyProtocolTrustedPrefixes(nil) })
}

func TestYZProxyProtocolEmptyListNeverTrustsHeaders(t *testing.T) {
	internet.SetProxyProtocolTrustedPrefixes(nil)

	// 未开启 acceptProxyProtocol：不解析，头原样作为数据。
	got := yzProxyExchange(t, yzProxyListen(t, false), true)
	if got.remote != "127.0.0.1" || got.data != yzProxyHeader+yzProxyPayload {
		t.Fatalf("plain listener = %+v", got)
	}

	// 旧开关不能在空名单下重新信任所有来源，也不能阻止普通直连。
	got = yzProxyExchange(t, yzProxyListen(t, true), true)
	if got.remote != "127.0.0.1" || got.data != yzProxyHeader+yzProxyPayload {
		t.Fatalf("legacy listener = %+v", got)
	}
	got = yzProxyExchange(t, yzProxyListen(t, true), false)
	if got.remote != "127.0.0.1" || got.data != yzProxyPayload {
		t.Fatalf("direct listener = %+v", got)
	}
}

func TestYZProxyProtocolTrustedSourceHeaderIsOptional(t *testing.T) {
	yzSetTrusted(t, "127.0.0.1/32")
	for _, accept := range []bool{false, true} {
		l := yzProxyListen(t, accept)
		if got := yzProxyExchange(t, l, true); got.remote != "203.0.113.9" || got.data != yzProxyPayload {
			t.Fatalf("accept=%v with header = %+v", accept, got)
		}
		if got := yzProxyExchange(t, l, false); got.remote != "127.0.0.1" || got.data != yzProxyPayload {
			t.Fatalf("accept=%v without header = %+v", accept, got)
		}
	}
}

func TestYZProxyProtocolUntrustedSourceCannotSpoof(t *testing.T) {
	yzSetTrusted(t, "192.0.2.0/24")
	for _, accept := range []bool{false, true} {
		l := yzProxyListen(t, accept)
		got := yzProxyExchange(t, l, true)
		if got.remote != "127.0.0.1" || got.data != yzProxyHeader+yzProxyPayload {
			t.Fatalf("accept=%v forged header = %+v", accept, got)
		}
		if got := yzProxyExchange(t, l, false); got.remote != "127.0.0.1" || got.data != yzProxyPayload {
			t.Fatalf("accept=%v direct = %+v", accept, got)
		}
	}
}

func TestYZProxyProtocolListAppliesToExistingListener(t *testing.T) {
	internet.SetProxyProtocolTrustedPrefixes(nil)
	l := yzProxyListen(t, true)
	if got := yzProxyExchange(t, l, true); got.remote != "127.0.0.1" {
		t.Fatalf("before update = %+v", got)
	}
	// IPv4 映射写法应与 IPv4 等价。
	yzSetTrusted(t, "::ffff:127.0.0.1/128")
	if got := yzProxyExchange(t, l, true); got.remote != "203.0.113.9" || got.data != yzProxyPayload {
		t.Fatalf("after update = %+v", got)
	}
	internet.SetProxyProtocolTrustedPrefixes(nil)
	if got := yzProxyExchange(t, l, true); got.remote != "127.0.0.1" {
		t.Fatalf("after clear = %+v", got)
	}
}

func TestYZProxyProtocolTrustIsPerListenerContext(t *testing.T) {
	trusted := &internet.ProxyProtocolTrust{}
	untrusted := &internet.ProxyProtocolTrust{}
	trusted.Set([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	untrusted.Set([]netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")})
	listen := func(policy *internet.ProxyProtocolTrust) net.Listener {
		t.Helper()
		ctx := internet.ContextWithProxyProtocolTrust(context.Background(), policy)
		l, err := internet.ListenSystem(ctx, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = l.Close() })
		return l
	}
	first, second := listen(trusted), listen(untrusted)
	if got := yzProxyExchange(t, first, true); got.remote != "203.0.113.9" || got.data != yzProxyPayload {
		t.Fatalf("可信实例未读取来源头: %+v", got)
	}
	if got := yzProxyExchange(t, second, true); got.remote != "127.0.0.1" || got.data != yzProxyHeader+yzProxyPayload {
		t.Fatalf("另一实例错误地沿用可信名单: %+v", got)
	}
	trusted.Set(nil)
	if got := yzProxyExchange(t, first, true); got.remote != "127.0.0.1" {
		t.Fatalf("清空名单后旧监听仍信任来源头: %+v", got)
	}
}
