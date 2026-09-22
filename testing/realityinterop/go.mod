module github.com/xtls/xray-core/testing/realityinterop

go 1.27

require (
	github.com/metacubex/mihomo v0.0.0-20260530011440-fc8c5a24b169
	github.com/sagernet/sing v0.9.0-beta.4
	github.com/sagernet/sing-box v1.14.0
	github.com/xtls/xray-core v1.260327.0
)

require (
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/caddyserver/certmagic v0.25.3-0.20260421143802-60d9d8b415d6 // indirect
	github.com/caddyserver/zerossl v0.1.5 // indirect
	github.com/cloudflare/circl v1.6.5 // indirect
	github.com/database64128/netx-go v0.1.1 // indirect
	github.com/database64128/tfo-go/v2 v2.3.2 // indirect
	github.com/florianl/go-nfqueue/v2 v2.1.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/gofrs/uuid/v5 v5.5.1 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/juju/ratelimit v1.0.2 // indirect
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/klauspost/cpuid/v2 v2.4.0 // indirect
	github.com/libdns/acmedns v0.5.0 // indirect
	github.com/libdns/alidns v1.0.6 // indirect
	github.com/libdns/cloudflare v0.2.2 // indirect
	github.com/libdns/libdns v1.1.1 // indirect
	github.com/logrusorgru/aurora v2.0.3+incompatible // indirect
	github.com/mdlayher/netlink v1.11.2 // indirect
	github.com/mdlayher/socket v0.6.0 // indirect
	github.com/metacubex/cpu v0.1.1 // indirect
	github.com/metacubex/hkdf v0.1.0 // indirect
	github.com/metacubex/hpke v0.1.0 // indirect
	github.com/metacubex/http v0.1.6 // indirect
	github.com/metacubex/mlkem v0.1.0 // indirect
	github.com/metacubex/randv2 v0.2.0 // indirect
	github.com/metacubex/sing v0.5.7 // indirect
	github.com/metacubex/tls v0.1.6 // indirect
	github.com/metacubex/utls v1.8.7 // indirect
	github.com/mholt/acmez/v3 v3.1.6 // indirect
	github.com/miekg/dns v1.1.73 // indirect
	github.com/mroth/weightedrand/v2 v2.1.0 // indirect
	github.com/pires/go-proxyproto v0.15.0 // indirect
	github.com/refraction-networking/utls v1.8.3-0.20260301010127-aa6edf4b11af // indirect
	github.com/sagernet/fswatch v0.1.2 // indirect
	github.com/sagernet/gvisor v0.0.0-20260727.0-sing-box-mod.1 // indirect
	github.com/sagernet/netlink v0.0.0-20260814022025-64455d367bbf // indirect
	github.com/sagernet/nftables v0.3.0-mod.4 // indirect
	github.com/sagernet/sing-tun v0.9.0-beta.4 // indirect
	github.com/sagernet/sing-usbip v0.0.0-20260817040617-28bd42667eca // indirect
	github.com/samber/lo v1.53.0 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/zeebo/blake3 v0.2.4 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	go.uber.org/zap/exp v0.3.0 // indirect
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

// 仅供本地互通测试，不进入核心或 Node 的正式依赖。
replace github.com/xtls/xray-core => ../..

replace github.com/sagernet/sing-box => github.com/P0me1oo/YZ-sing-box v1.14.0-yz.2
