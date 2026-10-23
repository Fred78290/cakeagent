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
	"fmt"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

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

// ChangeDisplaySettingsEx results (DISP_CHANGE_*)
const (
	dispChangeSuccessful = 0
	dispChangeRestart    = 1
	dispChangeFailed     = -1
	dispChangeBadMode    = -2

	// How many one-second tries a size gets before the closest supported one is used instead: the driver learns the
	// host's size at once but Windows only knows the new mode once it has been told about it, so the first tries may
	// fail with DISP_CHANGE_BADMODE
	retriesBeforeFallback = 4

	maxModes = 4096
)

func changeResultName(r int32) string {
	switch r {
	case dispChangeSuccessful:
		return "ok"
	case dispChangeRestart:
		return "restart needed"
	case dispChangeFailed:
		return "failed"
	case dispChangeBadMode:
		return "bad mode"
	case -3:
		return "not updated"
	case -4:
		return "bad flags"
	case -5:
		return "bad parameter"
	case -6:
		return "bad dual view"
	}

	return "unknown"
}

// currentMode returns the display's current mode, false when it can't be read.
func currentMode(pname *uint16) (devMode, bool) {
	var mode devMode

	mode.size = uint16(unsafe.Sizeof(mode))

	r, _, _ := procEnumDisplaySettings.Call(uintptr(unsafe.Pointer(pname)), enumCurrentSettings, uintptr(unsafe.Pointer(&mode)))

	return mode, r != 0
}

// apply changes the display's mode to width x height, unless it already is. done is true when the display now is at
// that size; result is what ChangeDisplaySettingsEx said (0 when nothing had to change).
func apply(name string, width, height uint32) (done bool, result int32) {
	pname, err := windows.UTF16PtrFromString(name)

	if err != nil {
		return false, dispChangeBadMode
	}

	mode, ok := currentMode(pname)

	if !ok {
		return false, dispChangeFailed
	}

	if mode.pelsWidth == width && mode.pelsHeight == height {
		return true, dispChangeSuccessful
	}

	mode.pelsWidth = width
	mode.pelsHeight = height
	mode.fields = dmPelsWidth | dmPelsHeight

	r, _, _ := procChangeDisplaySettingsEx.Call(uintptr(unsafe.Pointer(pname)), uintptr(unsafe.Pointer(&mode)), 0, 0, 0)

	result = int32(r)

	return result == dispChangeSuccessful || result == dispChangeRestart, result
}

// closestMode returns the largest mode the display supports that fits in width x height (the closest one when none
// does), false when it lists none.
func closestMode(name string, width, height uint32) (uint32, uint32, bool) {
	pname, err := windows.UTF16PtrFromString(name)

	if err != nil {
		return 0, 0, false
	}

	current, _ := currentMode(pname)

	var (
		bestW, bestH uint32
		found, fits  bool
		bestScore    uint64
		listed       []string
	)

	defer func() {
		glog.Infof("display %s: current %dx%dx%d, %d modes listed: %s", name, current.pelsWidth, current.pelsHeight, current.bitsPerPel, len(listed), strings.Join(listed, " "))
	}()

	for i := uintptr(0); i < maxModes; i++ {
		var mode devMode

		mode.size = uint16(unsafe.Sizeof(mode))

		if r, _, _ := procEnumDisplaySettings.Call(uintptr(unsafe.Pointer(pname)), i, uintptr(unsafe.Pointer(&mode))); r == 0 {
			break
		}

		listed = append(listed, fmt.Sprintf("%dx%dx%d", mode.pelsWidth, mode.pelsHeight, mode.bitsPerPel))

		// Keep the colour depth the display runs at
		if current.bitsPerPel != 0 && mode.bitsPerPel != current.bitsPerPel {
			continue
		}

		w, h := mode.pelsWidth, mode.pelsHeight

		if w <= width && h <= height {
			// Among the modes that fit, the biggest
			if score := uint64(w) * uint64(h); !fits || score > bestScore {
				bestW, bestH, bestScore, fits, found = w, h, score, true, true
			}
		} else if !fits {
			// None fits yet: the one that overshoots the least
			dw, dh := int64(w)-int64(width), int64(h)-int64(height)

			if dw < 0 {
				dw = -dw
			}

			if dh < 0 {
				dh = -dh
			}

			if score := uint64(dw + dh); !found || score < bestScore {
				bestW, bestH, bestScore, found = w, h, score, true
			}
		}
	}

	return bestW, bestH, found
}

// DISPLAY_DEVICE state flags, DM_POSITION and the CDS_* flags of ChangeDisplaySettingsEx
const (
	stateAttachedToDesktop = 0x00000001
	stateMirroringDriver   = 0x00000008

	dmPosition = 0x00000020

	cdsUpdateRegistry = 0x00000001
	cdsSetPrimary     = 0x00000010
	cdsNoReset        = 0x10000000

	// Under SOFTWARE\Caker: a DWORD set to 1 leaves Windows' Basic Display Driver on the desktop next to the GPU
	keepBasicValue = "KeepBasicDisplay"
)

func keepBasicDisplay() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, settingsKey, registry.QUERY_VALUE)

	if err != nil {
		return false
	}

	defer key.Close()

	value, _, err := key.GetIntegerValue(keepBasicValue)

	return err == nil && value != 0
}

// attachedDisplays lists the displays on the desktop: the GPU's one is reported separately, since it is the one that stays
func attachedDisplays(gpu string) (gpuAttached bool, others []string) {
	for i := uint32(0); i < 16; i++ {
		var d displayDevice

		d.cb = uint32(unsafe.Sizeof(d))

		if r, _, _ := procEnumDisplayDevices.Call(0, uintptr(i), uintptr(unsafe.Pointer(&d)), 0); r == 0 {
			continue
		}

		if d.stateFlags&stateAttachedToDesktop == 0 || d.stateFlags&stateMirroringDriver != 0 {
			continue
		}

		if name := windows.UTF16ToString(d.deviceName[:]); name == gpu {
			gpuAttached = true
		} else {
			others = append(others, name)
		}
	}

	return gpuAttached, others
}

// useOnlyGPU makes the virtio GPU's display the only one on the desktop.
//
// The firmware framebuffer the installer relies on to boot Windows is bound to Windows' Basic Display Driver, which
// stays as a second display next to the virtio GPU. Only the GPU's one is wired to the VM's view, but Windows extends
// the desktop onto both, so windows and the taskbar can land on the one nobody sees. The Basic Display adapter is not
// disabled (it is the fallback until the GPU driver loads): it is only taken off the desktop, like "show only on 2" in
// Windows' display settings. The GPU becomes the primary display first, so nothing is left without one.
func useOnlyGPU(gpu string) {
	gpuAttached, others := attachedDisplays(gpu)

	glog.Infof("display: %s on the desktop: %v, other displays on the desktop: %v", gpu, gpuAttached, others)

	if !gpuAttached || len(others) == 0 {
		return
	}

	pgpu, err := windows.UTF16PtrFromString(gpu)

	if err != nil {
		return
	}

	mode, ok := currentMode(pgpu)

	if !ok {
		return
	}

	mode.positionX, mode.positionY = 0, 0
	mode.fields = dmPosition | dmPelsWidth | dmPelsHeight

	flags := uintptr(cdsSetPrimary | cdsUpdateRegistry | cdsNoReset)

	if r, _, _ := procChangeDisplaySettingsEx.Call(uintptr(unsafe.Pointer(pgpu)), uintptr(unsafe.Pointer(&mode)), 0, flags, 0); int32(r) != dispChangeSuccessful {
		glog.Warnf("display %s: can't make it the primary display (%s, %d), leaving the other displays", gpu, changeResultName(int32(r)), int32(r))

		return
	}

	for _, other := range others {
		pother, err := windows.UTF16PtrFromString(other)

		if err != nil {
			continue
		}

		// A display with no size and no position is detached from the desktop
		detach := devMode{fields: dmPosition | dmPelsWidth | dmPelsHeight}

		detach.size = uint16(unsafe.Sizeof(detach))

		if r, _, _ := procChangeDisplaySettingsEx.Call(uintptr(unsafe.Pointer(pother)), uintptr(unsafe.Pointer(&detach)), 0, uintptr(cdsUpdateRegistry|cdsNoReset), 0); int32(r) != dispChangeSuccessful {
			glog.Warnf("display %s: can't take it off the desktop (%s, %d)", other, changeResultName(int32(r)), int32(r))
		} else {
			glog.Infof("display %s: taken off the desktop, %s is the only display", other, gpu)
		}
	}

	// Apply the changes made with CDS_NORESET
	if r, _, _ := procChangeDisplaySettingsEx.Call(0, 0, 0, 0, 0); int32(r) != dispChangeSuccessful {
		glog.Warnf("display: applying the display changes failed (%s, %d)", changeResultName(int32(r)), int32(r))
	}
}

// Watch applies the size the host asks for until stop is closed. A change of what the host asks for is applied (and
// the first value), not whatever differs from the current mode, so a resolution the user picks in Windows' settings
// isn't undone every second. A size Windows refuses is tried again for a few seconds, then the closest supported
// one is used, so a host window bigger than what the display supports doesn't leave the guest stuck.
func Watch(stop <-chan struct{}) {
	var (
		name    string
		adapter uint32
		wantW   uint32
		wantH   uint32
		pending bool
		tries   int
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

					if !keepBasicDisplay() {
						useOnlyGPU(name)
					}
				}
			}
		}

		if adapter != 0 {
			if width, height, ok := wantedSize(adapter); !ok {
				// The adapter went away (driver reset): look for it again
				closeAdapterHandle(adapter)
				adapter = 0
			} else {
				if width >= 640 && height >= 480 && (width != wantW || height != wantH) {
					wantW, wantH, pending, tries = width, height, true, 0
				}

				if pending {
					done, result := apply(name, wantW, wantH)

					if done {
						if result != dispChangeSuccessful || tries > 0 {
							glog.Infof("display %s: %dx%d, %s", name, wantW, wantH, changeResultName(result))
						} else {
							glog.Infof("display %s: %dx%d", name, wantW, wantH)
						}

						pending = false
					} else if tries++; tries > retriesBeforeFallback {
						glog.Warnf("display %s: %dx%d refused (%s, %d), using the closest supported size", name, wantW, wantH, changeResultName(result), result)

						if w, h, found := closestMode(name, wantW, wantH); found {
							if done, result = apply(name, w, h); done {
								glog.Infof("display %s: %dx%d instead of %dx%d", name, w, h, wantW, wantH)
							} else {
								glog.Warnf("display %s: %dx%d refused too (%s, %d)", name, w, h, changeResultName(result), result)
							}
						} else {
							glog.Warnf("display %s: no supported mode found", name)
						}

						// Whatever happened, wait for the host to ask for something else
						pending = false
					}
				}
			}
		}

		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}
