package reality

import (
	"bytes"
	"crypto/mlkem"
	"testing"
)

func TestRealityAuthPublicKey(t *testing.T) {
	x := keyShare{X25519, bytes.Repeat([]byte{1}, 32)}
	m := keyShare{X25519MLKEM768, bytes.Repeat([]byte{2}, mlkem.EncapsulationKeySize768+32)}
	for _, tc := range []struct {
		name   string
		shares []keyShare
		want   []byte
	}{
		{"traditional", []keyShare{x}, x.data},
		{"hybrid-only", []keyShare{m}, m.data[mlkem.EncapsulationKeySize768:]},
		{"hybrid-first", []keyShare{m, x}, x.data},
		{"traditional-first", []keyShare{x, m}, x.data},
		{"unknown-group", []keyShare{{CurveID(0xaaaa), []byte{0}}, x}, x.data},
		{"empty", nil, nil},
		{"duplicate-traditional", []keyShare{x, x}, nil},
		{"duplicate-hybrid", []keyShare{m, m}, nil},
		{"duplicate-after-traditional", []keyShare{m, x, m}, nil},
		{"short-traditional", []keyShare{{X25519, []byte{1}}, m}, nil},
		{"short-hybrid", []keyShare{x, {X25519MLKEM768, []byte{1}}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, valid := realityAuthPublicKey(tc.shares)
			if valid != (tc.want != nil) || !bytes.Equal(got, tc.want) {
				t.Fatal("认证密钥选择或异常输入处理不符合预期")
			}
		})
	}
}
