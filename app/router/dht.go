package router

import (
	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/features/routing"
	"github.com/xtls/xray-core/proxy/blackhole"
)

// HasDHTBlockRule avoids changing the UDP copying path of instances without a
// bound BT block. The guard still checks the actual selected rule per datagram.
func (r *Router) HasDHTBlockRule() bool {
	if r.script != nil || r.ohm == nil {
		return false
	}
	for _, rule := range *r.rules.Load() {
		if !rule.bitTorrent || rule.Balancer != nil {
			continue
		}
		handler := r.ohm.GetHandler(rule.Tag)
		if handler != nil && handler.ProxySettings() != nil &&
			handler.ProxySettings().Type == serial.GetMessageType(&blackhole.Config{}) {
			return true
		}
	}
	return false
}

// PickDHTRoute reuses rule ordering and conditions without firing webhooks for
// non-blocking traffic. Only explicit BT field rules qualify; scripts retain
// their original behavior and do not implicitly opt into packet filtering.
func (r *Router) PickDHTRoute(ctx routing.Context) (routing.Route, error) {
	if r.script != nil {
		return nil, common.ErrNoClue
	}
	rule, matchedContext, err := r.pickRouteInternal(ctx)
	if err != nil {
		return nil, err
	}
	if !rule.bitTorrent || rule.Balancer != nil {
		return nil, common.ErrNoClue
	}
	return &Route{Context: matchedContext, outboundTag: rule.Tag, ruleTag: rule.RuleTag}, nil
}
