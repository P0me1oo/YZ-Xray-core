package dispatcher

import (
	"testing"

	"github.com/xtls/xray-core/common/net"
)

type relayTestCounter struct{ value int64 }

func (c *relayTestCounter) Value() int64 { return c.value }
func (c *relayTestCounter) Set(value int64) int64 {
	previous := c.value
	c.value = value
	return previous
}
func (c *relayTestCounter) Add(value int64) int64 {
	c.value += value
	return c.value - value
}

func TestRelayUserCounterName(t *testing.T) {
	got := relayUserCounterName("user@example.com", net.Port(443), "uplink")
	if got != "user>>>user@example.com>>>relay>>>443>>>traffic>>>uplink" {
		t.Fatalf("counter name = %q", got)
	}
}

func TestCombinedCounterUpdatesBothCounters(t *testing.T) {
	first := &relayTestCounter{}
	second := &relayTestCounter{}
	combined := &combinedCounter{first: first, second: second}

	combined.Add(128)
	if first.Value() != 128 || second.Value() != 128 {
		t.Fatalf("after Add: first=%d second=%d", first.Value(), second.Value())
	}
	combined.Set(0)
	if first.Value() != 0 || second.Value() != 0 {
		t.Fatalf("after Set: first=%d second=%d", first.Value(), second.Value())
	}
}
