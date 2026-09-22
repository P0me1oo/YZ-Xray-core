package realityinterop

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/hex"
	"net"
	"testing"
	"time"

	mtls "github.com/metacubex/mihomo/component/tls"
	mutls "github.com/metacubex/utls"
	stls "github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/logger"
	"github.com/xtls/xray-core/testing/realitytest"
)

func connect(t *testing.T, s *realitytest.Server, client string, invalid bool) (net.Conn, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := net.DialTimeout("tcp", s.Address, 5*time.Second)
	if err != nil {
		return nil, err
	}
	raw.SetDeadline(time.Now().Add(5 * time.Second))
	shortID := s.ShortID
	if invalid {
		shortID[0] ^= 0xff
	}
	var conn net.Conn
	if client == "sing-box" {
		cfg, e := stls.NewRealityClient(ctx, logger.NOP(), "reality.test", option.OutboundTLSOptions{
			Enabled: true, ServerName: "reality.test",
			UTLS:    &option.OutboundUTLSOptions{Enabled: true, Fingerprint: "chrome"},
			Reality: &option.OutboundRealityOptions{Enabled: true, PublicKey: base64.RawURLEncoding.EncodeToString(s.PublicKey), ShortID: hex.EncodeToString(shortID[:])},
		})
		if e != nil {
			raw.Close()
			return nil, e
		}
		conn, err = stls.ClientHandshake(ctx, raw, cfg)
	} else {
		pub, e := ecdh.X25519().NewPublicKey(s.PublicKey)
		if e != nil {
			raw.Close()
			return nil, e
		}
		fp, ok := mtls.GetFingerprint("chrome")
		if !ok {
			t.Fatal("缺少 chrome 指纹")
		}
		conn, err = mtls.GetRealityConn(ctx, raw, fp, "reality.test", &mtls.RealityConfig{
			PublicKey: pub, ShortID: shortID, SupportX25519MLKEM768: client == "mihomo-hybrid",
		})
	}
	if err != nil {
		raw.Close()
	} else if client != "sing-box" {
		want := mutls.X25519
		if client == "mihomo-hybrid" && s.HybridTarget {
			want = mutls.X25519MLKEM768
		}
		got := conn.(*mutls.UConn).HandshakeState.ServerHello.ServerShare.Group
		if got != want {
			conn.Close()
			t.Fatalf("mihomo 实际协商算法=%d，预期=%d", got, want)
		}
	}
	return conn, err
}

func TestClientInteroperability(t *testing.T) {
	for _, hybrid := range []bool{false, true} {
		name := "traditional-target"
		if hybrid {
			name = "hybrid-target"
		}
		t.Run(name, func(t *testing.T) {
			s := realitytest.Start(t, hybrid)
			for _, client := range []string{"sing-box", "mihomo-traditional", "mihomo-hybrid"} {
				t.Run(client, func(t *testing.T) {
					for round := 0; round < 3; round++ {
						conn, err := connect(t, s, client, false)
						if err != nil {
							t.Fatal(err)
						}
						realitytest.Echo(t, conn)
					}
				})
				t.Run(client+"-invalid-auth", func(t *testing.T) {
					conn, err := connect(t, s, client, true)
					if conn != nil {
						conn.Close()
					}
					if err == nil {
						t.Fatal("错误认证不应建立代理连接")
					}
				})
			}
			t.Run("mixed-concurrent", func(t *testing.T) {
				for i := 0; i < 12; i++ {
					client := []string{"sing-box", "mihomo-traditional", "mihomo-hybrid"}[i%3]
					t.Run(client, func(t *testing.T) {
						t.Parallel()
						conn, err := connect(t, s, client, false)
						if err != nil {
							t.Fatal(err)
						}
						realitytest.Echo(t, conn)
					})
				}
			})
		})
	}
}
