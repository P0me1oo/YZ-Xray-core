package shadowsocks_2022

import (
	"context"
	"time"

	"github.com/sagernet/sing/common/ntp"
)

// timeFuncFromContext 允许嵌入式调用方为 SS2022 提供校准时间。
// 未注册时间服务时返回 nil，由 sing-shadowsocks 保持原有 time.Now() 行为。
func timeFuncFromContext(ctx context.Context) func() time.Time {
	return ntp.TimeFuncFromContext(ctx)
}
