module github.com/Fred78290/cakeagent

go 1.23.7

toolchain go1.24.2

require (
	github.com/alecthomas/kingpin v2.2.6+incompatible
	github.com/creack/pty v1.1.24
	github.com/elastic/go-sysinfo v1.15.0
	github.com/mdlayher/vsock v1.2.1
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
	github.com/samber/lo v1.49.1
	github.com/shirou/gopsutil/v4 v4.25.4
	github.com/sirupsen/logrus v1.9.3
	golang.org/x/exp v0.0.0-20250408133849-7e4ce0ab07d0
	golang.org/x/net v0.35.0
	golang.org/x/sys v0.32.0
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.5
	howett.net/plist v1.0.1
)

require (
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b // indirect
	github.com/ebitengine/purego v0.8.2 // indirect
	github.com/elastic/go-windows v1.0.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/mdlayher/socket v0.5.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/sync v0.13.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250224174004-546df14abb99 // indirect
)

replace github.com/mdlayher/vsock v1.2.1 => github.com/Fred78290/vsock v0.0.1
