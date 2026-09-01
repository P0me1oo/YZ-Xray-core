package shadowsocks_2022

import (
	"context"
	"testing"
	"time"

	"github.com/sagernet/sing/common/ntp"
	"github.com/sagernet/sing/service"
)

type fixedTimeService struct {
	now time.Time
}

func (s fixedTimeService) TimeFunc() func() time.Time {
	return func() time.Time { return s.now }
}

func TestTimeFuncFromContext(t *testing.T) {
	if got := timeFuncFromContext(context.Background()); got != nil {
		t.Fatal("timeFuncFromContext() without service must return nil")
	}

	want := time.Unix(1_800_000_000, 0)
	ctx := service.ContextWithDefaultRegistry(context.Background())
	service.MustRegister[ntp.TimeService](ctx, fixedTimeService{now: want})

	now := timeFuncFromContext(ctx)
	if now == nil {
		t.Fatal("timeFuncFromContext() returned nil with registered service")
	}
	if got := now(); !got.Equal(want) {
		t.Fatalf("timeFuncFromContext() = %v, want %v", got, want)
	}
}
