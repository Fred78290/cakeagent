//go:build windows

// Package display makes a Windows guest follow the size the host asks for.
//
// Virtualization.framework reconfigures the virtio GPU when the VM's window is resized, and the virtio-win display
// driver (viogpudo) learns the wanted size (VIOGPU_GET_CUSTOM_RESOLUTION escape). Nothing applies it: virtio-win's own
// helper only looks at the first display device, which is Windows' Basic Display Driver here. Watch does it.
//
// It has to run in the logged-on user's session, because only there is there a display to query and change: a
// service lives in session 0. Supervise (see supervise_windows.go) starts it there.
package display

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	glog "github.com/sirupsen/logrus"
)

const (
	// The virtio GPU's PCI hardware id, as EnumDisplayDevices reports it in DeviceID
	virtioGPUDeviceID = `PCI\VEN_1AF4&DEV_1050`

	// VIOGPU_ESCAPE_TYPE of viogpudo
	viogpuGetCustomResolution = 1

	dmPelsWidth  = 0x00080000
	dmPelsHeight = 0x00100000

	// ENUM_CURRENT_SETTINGS
	enumCurrentSettings = 0xFFFFFFFF

	pollInterval = time.Second
)

type displayDevice struct {
	cb           uint32
	deviceName   [32]uint16
	deviceString [128]uint16
	stateFlags   uint32
	deviceID     [128]uint16
	deviceKey    [128]uint16
}

// devMode is DEVMODEW, its display flavour
type devMode struct {
	deviceName         [32]uint16
	specVersion        uint16
	driverVersion      uint16
	size               uint16
	driverExtra        uint16
	fields             uint32
	positionX          int32
	positionY          int32
	displayOrientation uint32
	displayFixedOutput uint32
	color              int16
	duplex             int16
	yResolution        int16
	ttOption           int16
	collate            int16
	formName           [32]uint16
	logPixels          uint16
	bitsPerPel         uint32
	pelsWidth          uint32
	pelsHeight         uint32
	displayFlags       uint32
	displayFrequency   uint32
	icmMethod          uint32
	icmIntent          uint32
	mediaType          uint32
	ditherType         uint32
	reserved1          uint32
	reserved2          uint32
	panningWidth       uint32
	panningHeight      uint32
}

// D3DKMT_OPENADAPTERFROMHDC
type openAdapterFromHdc struct {
	hdc           windows.Handle
	adapter       uint32
	luidLowPart   uint32
	luidHighPart  int32
	vidPnSourceID uint32
}

// D3DKMT_CLOSEADAPTER
type closeAdapter struct {
	adapter uint32
}

// D3DKMT_ESCAPE
type escape struct {
	adapter uint32
	device  uint32
	kind    int32
	flags   uint32
	data    uintptr
	size    uint32
	context uint32
}

// VIOGPU_ESCAPE { USHORT Type; USHORT DataLength; USHORT X; USHORT Y }
type viogpuEscape struct {
	kind   uint16
	length uint16
	x      uint16
	y      uint16
}

// The struct layouts above must match the C ones, or the calls corrupt memory: fail the build instead
var (
	_ [220 - unsafe.Sizeof(devMode{})]byte
	_ [unsafe.Sizeof(devMode{}) - 220]byte
	_ [840 - unsafe.Sizeof(displayDevice{})]byte
	_ [unsafe.Sizeof(displayDevice{}) - 840]byte
	_ [8 - unsafe.Sizeof(viogpuEscape{})]byte
	_ [unsafe.Sizeof(viogpuEscape{}) - 8]byte
	_ [unsafe.Offsetof(escape{}.data) - 16]byte
	_ [16 - unsafe.Offsetof(escape{}.data)]byte
	_ [unsafe.Sizeof(escape{}) - 32]byte
	_ [32 - unsafe.Sizeof(escape{})]byte
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	procEnumDisplayDevices      = user32.NewProc("EnumDisplayDevicesW")
	procEnumDisplaySettings     = user32.NewProc("EnumDisplaySettingsW")
	procChangeDisplaySettingsEx = user32.NewProc("ChangeDisplaySettingsExW")
	procCreateDC                = gdi32.NewProc("CreateDCW")
	procDeleteDC                = gdi32.NewProc("DeleteDC")
	procOpenAdapterFromHdc      = gdi32.NewProc("D3DKMTOpenAdapterFromHdc")
	procCloseAdapter            = gdi32.NewProc("D3DKMTCloseAdapter")
	procEscape                  = gdi32.NewProc("D3DKMTEscape")
)

// findGPU returns the name (`\\.\DISPLAYn`) of the virtio GPU. It is not necessarily the first display device.
func findGPU() (string, bool) {
	for i := uint32(0); i < 16; i++ {
		var d displayDevice

		d.cb = uint32(unsafe.Sizeof(d))

		if r, _, _ := procEnumDisplayDevices.Call(0, uintptr(i), uintptr(unsafe.Pointer(&d)), 0); r == 0 {
			continue
		}

		if id := windows.UTF16ToString(d.deviceID[:]); len(id) >= len(virtioGPUDeviceID) && equalFold(id[:len(virtioGPUDeviceID)], virtioGPUDeviceID) {
			return windows.UTF16ToString(d.deviceName[:]), true
		}
	}

	return "", false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]

		if 'a' <= x && x <= 'z' {
			x -= 'a' - 'A'
		}

		if 'a' <= y && y <= 'z' {
			y -= 'a' - 'A'
		}

		if x != y {
			return false
		}
	}

	return true
}

// openAdapter opens the D3DKMT adapter behind a display device, 0 when it can't.
func openAdapter(name string) uint32 {
	pname, err := windows.UTF16PtrFromString(name)

	if err != nil {
		return 0
	}

	hdc, _, _ := procCreateDC.Call(0, uintptr(unsafe.Pointer(pname)), 0, 0)

	if hdc == 0 {
		return 0
	}

	defer procDeleteDC.Call(hdc)

	open := openAdapterFromHdc{hdc: windows.Handle(hdc)}

	if status, _, _ := procOpenAdapterFromHdc.Call(uintptr(unsafe.Pointer(&open))); int32(status) != 0 {
		return 0
	}

	return open.adapter
}

func closeAdapterHandle(adapter uint32) {
	c := closeAdapter{adapter: adapter}

	procCloseAdapter.Call(uintptr(unsafe.Pointer(&c)))
}

var escapeData viogpuEscape

// wantedSize asks the driver for the size the host wants. ok is false when the call fails, which means the adapter
// went away (driver reset).
func wantedSize(adapter uint32) (width, height uint32, ok bool) {
	// The driver's data is reached through a uintptr inside the escape struct, which keeps nothing alive and, on a
	// goroutine stack, would not follow a stack move: use memory that never moves
	escapeData = viogpuEscape{kind: viogpuGetCustomResolution, length: 4}

	e := escape{
		adapter: adapter,
		data:    uintptr(unsafe.Pointer(&escapeData)),
		size:    uint32(unsafe.Sizeof(escapeData)),
	}

	status, _, _ := procEscape.Call(uintptr(unsafe.Pointer(&e)))

	if int32(status) != 0 {
		return 0, 0, false
	}

	return uint32(escapeData.x), uint32(escapeData.y), true
}

// apply changes the display's mode to width x height, unless it already is.
func apply(name string, width, height uint32) {
	pname, err := windows.UTF16PtrFromString(name)

	if err != nil {
		return
	}

	var mode devMode

	mode.size = uint16(unsafe.Sizeof(mode))

	if r, _, _ := procEnumDisplaySettings.Call(uintptr(unsafe.Pointer(pname)), enumCurrentSettings, uintptr(unsafe.Pointer(&mode))); r == 0 {
		return
	}

	if mode.pelsWidth == width && mode.pelsHeight == height {
		return
	}

	mode.pelsWidth = width
	mode.pelsHeight = height
	mode.fields = dmPelsWidth | dmPelsHeight

	r, _, _ := procChangeDisplaySettingsEx.Call(uintptr(unsafe.Pointer(pname)), uintptr(unsafe.Pointer(&mode)), 0, 0, 0)

	glog.Infof("display %s: %dx%d, result %d", name, width, height, int32(r))
}

// Watch applies the size the host asks for until stop is closed. A change of what the host asks for is applied (and
// the first value), not whatever differs from the current mode, so a resolution the user picks in Windows' settings
// isn't undone every second.
func Watch(stop <-chan struct{}) {
	var (
		name    string
		adapter uint32
		lastW   uint32
		lastH   uint32
	)

	ticker := time.NewTicker(pollInterval)

	defer ticker.Stop()
	defer func() {
		if adapter != 0 {
			closeAdapterHandle(adapter)
		}
	}()

	for {
		if adapter == 0 {
			if gpu, found := findGPU(); found {
				if adapter = openAdapter(gpu); adapter != 0 {
					name = gpu

					glog.Infof("display: following the host's size on %s", name)
				}
			}
		}

		if adapter != 0 {
			if width, height, ok := wantedSize(adapter); !ok {
				// The adapter went away (driver reset): look for it again
				closeAdapterHandle(adapter)
				adapter = 0
			} else if width >= 640 && height >= 480 && (width != lastW || height != lastH) {
				apply(name, width, height)

				lastW, lastH = width, height
			}
		}

		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}
