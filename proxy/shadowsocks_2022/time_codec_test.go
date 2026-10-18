package shadowsocks_2022

import (
	"bytes"
	"crypto/rand"
	"testing"
	"time"

	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/net"
)

func TestCalibratedTCPClock(t *testing.T) {
	for _, name := range []string{MethodAES128GCM, MethodAES256GCM, MethodChaCha20Poly1305} {
		t.Run(name, func(t *testing.T) {
			method, err := GetCipherMethod(name)
			if err != nil {
				t.Fatal(err)
			}
			key, salt := make([]byte, method.KeySaltLength), make([]byte, method.KeySaltLength)
			rand.Read(key)
			rand.Read(salt)
			now := time.Now().Add(10 * time.Minute)
			clock := func() time.Time { return now }
			dest := net.TCPDestination(net.LocalHostIP, 443)
			payload := []byte("calibrated-clock")
			var request bytes.Buffer
			if _, err := WriteTCPRequest(&request, method, [][]byte{key}, dest, salt, payload, clock); err != nil {
				t.Fatal(err)
			}
			wire := append([]byte(nil), request.Bytes()...)
			readRequest := func(clocks ...func() time.Time) (*ClientRequestHeader, error) {
				aead, err := method.NewAEAD(DeriveSessionSubKey(key, salt, method.KeySaltLength))
				if err != nil {
					t.Fatal(err)
				}
				offset := method.KeySaltLength
				end := offset + RequestHeaderFixedChunkLength + AEADTagSize
				reader := NewStreamReader(bytes.NewReader(wire[end:]), aead)
				return ReadClientRequestHeaderWithFixed(reader, append([]byte(nil), wire[offset:end]...), clocks...)
			}
			if _, err := readRequest(); err != ErrBadTimestamp {
				t.Fatalf("uncalibrated request: %v", err)
			}
			header, err := readRequest(clock)
			if err != nil {
				t.Fatal(err)
			}
			if header.Destination != dest || !bytes.Equal(header.EarlyData, payload) {
				t.Fatal("request destination or payload changed")
			}
			var response bytes.Buffer
			writer := NewServerStreamWriter(&response, method, key, salt, clock)
			if _, err := writer.Write(payload); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadTCPResponse(bytes.NewReader(response.Bytes()), method, key, salt); err != ErrBadTimestamp {
				t.Fatalf("uncalibrated response: %v", err)
			}
			reader, err := ReadTCPResponse(bytes.NewReader(response.Bytes()), method, key, salt, clock)
			if err != nil {
				t.Fatal(err)
			}
			mb, err := reader.ReadMultiBuffer()
			if err != nil {
				t.Fatal(err)
			}
			defer buf.ReleaseMulti(mb)
			if mb.String() != string(payload) {
				t.Fatal("response payload changed")
			}
		})
	}
}

func TestCalibratedUDPClock(t *testing.T) {
	for _, name := range []string{MethodAES128GCM, MethodAES256GCM, MethodChaCha20Poly1305} {
		t.Run(name, func(t *testing.T) {
			method, err := GetCipherMethod(name)
			if err != nil {
				t.Fatal(err)
			}
			key := make([]byte, method.KeySaltLength)
			rand.Read(key)
			now := time.Now().Add(10 * time.Minute)
			clock := func() time.Time { return now }
			client, err := NewUDPPacketCodec(method, [][]byte{key}, clock)
			if err != nil {
				t.Fatal(err)
			}
			server, err := NewUDPServerCodec(method, key, time.Minute, clock)
			if err != nil {
				t.Fatal(err)
			}
			plainClock, err := NewUDPServerCodec(method, key, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			dest := net.UDPDestination(net.LocalHostIP, 53)
			payload := []byte("clock-reload")
			for range 2 {
				// A new session after clock adjustment must use the same live time callback.
				session, err := client.NewClientSession()
				if err != nil {
					t.Fatal(err)
				}
				request, err := session.EncodePacket(dest, payload)
				if err != nil {
					t.Fatal(err)
				}
				defer request.Release()
				if _, err := plainClock.DecodePacket(request.Bytes()); err != ErrBadTimestamp {
					t.Fatalf("uncalibrated datagram: %v", err)
				}
				decoded, err := server.DecodePacket(request.Bytes())
				if err != nil {
					t.Fatal(err)
				}
				if decoded.Destination != dest || !bytes.Equal(decoded.Payload, payload) {
					t.Fatal("datagram changed")
				}
				if _, err := server.DecodePacket(request.Bytes()); err != ErrPacketIdNotUnique {
					t.Fatalf("replay accepted: %v", err)
				}
				response, err := server.EncodeServerPacket(decoded.SessionID, dest, payload)
				if err != nil {
					t.Fatal(err)
				}
				decoded, err = session.DecodePacket(response)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(decoded.Payload, payload) {
					t.Fatal("response changed")
				}
				now = now.Add(time.Minute)
			}
		})
	}
}
