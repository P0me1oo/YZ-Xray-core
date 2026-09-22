package reality

import "crypto/mlkem"

// realityAuthPublicKey 兼容传统和混合握手，但不放宽密钥长度与重复项检查。
// 认证密钥优先采用独立 X25519，与 Xray、mihomo 和 sing-box 客户端保持一致。
// 实际 TLS 密钥交换仍沿用目标站点协商出的算法，不在这里强制降级。
func realityAuthPublicKey(shares []keyShare) ([]byte, bool) {
	var classical, hybrid []byte
	for _, share := range shares {
		switch share.group {
		case X25519:
			if classical != nil || len(share.data) != 32 {
				return nil, false
			}
			classical = share.data
		case X25519MLKEM768:
			if hybrid != nil || len(share.data) != mlkem.EncapsulationKeySize768+32 {
				return nil, false
			}
			hybrid = share.data[mlkem.EncapsulationKeySize768:]
		}
	}
	if classical != nil {
		return classical, true
	}
	return hybrid, hybrid != nil
}
