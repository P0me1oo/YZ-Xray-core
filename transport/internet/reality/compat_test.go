package reality_test

import (
	"context"
	"net"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/testing/realitytest"
	"github.com/xtls/xray-core/transport/internet/reality"
)

func TestRealityHybridAndTraditionalTargets(t *testing.T) {
	for _, hybrid := range []bool{false, true} {
		name := "traditional-target"
		if hybrid {
			name = "hybrid-target"
		}
		t.Run(name, func(t *testing.T) {
			s := realitytest.Start(t, hybrid)
			for round := 0; round < 3; round++ {
				raw, err := net.DialTimeout("tcp", s.Address, 5*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				raw.SetDeadline(time.Now().Add(5 * time.Second))
				conn, err := reality.UClient(raw, &reality.Config{
					Fingerprint: "chrome", ServerName: "reality.test", PublicKey: s.PublicKey, ShortId: s.ShortID[:],
				}, context.Background(), xnet.TCPDestination(xnet.LocalHostIP, 443))
				if err != nil {
					raw.Close()
					t.Fatal(err)
				}
				got := conn.(*reality.UConn).HandshakeState.ServerHello.ServerShare.Group
				want := utls.X25519
				if hybrid {
					want = utls.X25519MLKEM768
				}
				if got != want {
					conn.Close()
					t.Fatalf("协商算法=%d，预期=%d", got, want)
				}
				realitytest.Echo(t, conn)
			}
		})
	}
}
