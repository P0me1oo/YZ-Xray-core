// Package realitytest 为 REALITY 兼容测试提供仅监听回环地址的目标站点和回显服务。
package realitytest

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"io"
	"math/big"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	reality "github.com/xtls/xray-core/third_party/reality"
)

type Server struct {
	HybridTarget bool
	Address      string
	PublicKey    []byte
	ShortID      [8]byte
	Config       *reality.Config
}

// Start 每次生成临时身份和证书，测试结束后关闭监听及所有连接。
// hybridTarget 为 false 时模拟只支持传统握手的目标站点。
func Start(t testing.TB, hybridTarget bool) *Server {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"reality.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		// REALITY 需要足够大的证书记录来容纳替换后的认证证书。
		ExtraExtensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1}, Value: make([]byte, 1024)}},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	curves := []tls.CurveID{tls.X25519}
	if hybridTarget {
		curves = []tls.CurveID{tls.X25519MLKEM768, tls.X25519}
	}
	target, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		MinVersion:   tls.VersionTLS13, CurvePreferences: curves,
		NextProtos: []string{"h2", "http/1.1"}, SessionTicketsDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		target.Close()
		t.Fatal(err)
	}
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		target.Close()
		listener.Close()
		t.Fatal(err)
	}
	s := &Server{Address: listener.Addr().String(), PublicKey: privateKey.PublicKey().Bytes(), HybridTarget: hybridTarget}
	if _, err = rand.Read(s.ShortID[:]); err != nil {
		t.Fatal(err)
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	s.Config = &reality.Config{
		DialContext: dialer.DialContext, Type: "tcp", Dest: target.Addr().String(),
		PrivateKey: privateKey.Bytes(), ServerNames: map[string]bool{"reality.test": true},
		ShortIds: map[[8]byte]bool{s.ShortID: true}, MaxTimeDiff: time.Minute,
		SessionTicketsDisabled: true,
	}
	// 已知本地目标不发会话票据，不启动上游的后台网络探测。
	for i := 0; i < 3; i++ {
		cacheKey := s.Config.Dest + " reality.test " + strconv.Itoa(i)
		reality.GlobalPostHandshakeRecordsLens.Store(cacheKey, []int{})
		t.Cleanup(func() { reality.GlobalPostHandshakeRecordsLens.Delete(cacheKey) })
	}
	var wg sync.WaitGroup
	var connections sync.Map
	serve := func(l net.Listener, handle func(net.Conn)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				connections.Store(conn, struct{}{})
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer conn.Close()
					defer connections.Delete(conn)
					conn.SetDeadline(time.Now().Add(8 * time.Second))
					handle(conn)
				}()
			}
		}()
	}
	serve(target, func(conn net.Conn) { io.Copy(io.Discard, conn) })
	serve(listener, func(conn net.Conn) {
		secured, err := reality.Server(context.Background(), conn, s.Config)
		if err == nil {
			defer secured.Close()
			io.Copy(secured, secured)
		}
	})
	t.Cleanup(func() {
		listener.Close()
		target.Close()
		connections.Range(func(key, _ any) bool { key.(net.Conn).Close(); return true })
		wg.Wait()
	})
	return s
}

// Echo 验证双向数据传输，失败时不输出身份或握手数据。
func Echo(t testing.TB, conn net.Conn) {
	t.Helper()
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	payload := make([]byte, 32768)
	rand.Read(payload)
	writeResult := make(chan error, 1)
	go func() { _, err := conn.Write(payload); writeResult <- err }()
	got := make([]byte, len(payload))
	_, readErr := io.ReadFull(conn, got)
	writeErr := <-writeResult
	if readErr != nil || writeErr != nil {
		t.Fatalf("双向传输失败: read=%v write=%v", readErr, writeErr)
	}
	for i := range payload {
		if got[i] != payload[i] {
			t.Fatal("回显数据不一致")
		}
	}
}
