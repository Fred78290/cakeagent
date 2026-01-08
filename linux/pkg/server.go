package pkg

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Fred78290/cakeagent/cmd/types"
	"github.com/Fred78290/cakeagent/console"
	"github.com/Fred78290/cakeagent/pkg/cakeagent"
	"github.com/Fred78290/cakeagent/pkg/event"
	"github.com/Fred78290/cakeagent/pkg/mount"
	"github.com/Fred78290/cakeagent/pkg/resize"
	"github.com/Fred78290/cakeagent/pkg/serialport"
	"github.com/Fred78290/cakeagent/pkg/tunnel"
	"github.com/Fred78290/cakeagent/pkg/utils"
	"github.com/Fred78290/cakeagent/version"
	"github.com/creack/pty"
	"github.com/elastic/go-sysinfo"
	"github.com/lima-vm/lima/pkg/guestagent"
	"github.com/lima-vm/lima/pkg/guestagent/api"
	"github.com/mdlayher/vsock"
	"github.com/pbnjay/memory"
	"github.com/pkg/term/termios"
	"github.com/shirou/gopsutil/v4/disk"
	glog "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	strErrFailedToGetExitStatus = "failed to get exit status: %v"
	strErrFailedToRunCommand    = "failed to run command: %v"
	strErrFailedToGetExitCode   = "failed to get exit code: %v"
)

type server struct {
	cakeagent.UnimplementedCakeAgentServiceServer
	agent guestagent.Agent
}

type pipe struct {
	name   string
	input  *os.File
	output *os.File
}

type networkInterface struct {
	Name      string
	Index     int
	Addresses []string
}

func collectDiskInfos() (diskInfos []*cakeagent.CakeAgent_InfoReply_DiskInfo, err error) {
	var partitions []disk.PartitionStat

	if partitions, err = disk.Partitions(true); err != nil {
		return
	}

	diskInfos = make([]*cakeagent.CakeAgent_InfoReply_DiskInfo, 0, len(partitions))

	for _, partition := range partitions {
		if !slices.Contains(partition.Opts, "nobrowse") && !slices.Contains(partition.Opts, "nodev") {
			if diskInfo, err := disk.Usage(partition.Mountpoint); err == nil {
				if diskInfo != nil {
					diskInfos = append(diskInfos, &cakeagent.CakeAgent_InfoReply_DiskInfo{
						Device: partition.Device,
						Mount:  partition.Mountpoint,
						FsType: partition.Fstype,
						Size:   diskInfo.Total,
						Free:   diskInfo.Free,
						Used:   diskInfo.Used,
					})
				}
			}
		}
	}

	return
}

func collectMemoryUsage() *cakeagent.CakeAgent_InfoReply_MemoryInfo {
	memoryTotal := memory.TotalMemory()
	memoryFree := memory.FreeMemory()

	return &cakeagent.CakeAgent_InfoReply_MemoryInfo{
		Total: memoryTotal,
		Free:  memoryFree,
		Used:  memoryTotal - memoryFree,
	}
}

func collectCpuUsage() (*cakeagent.CakeAgent_InfoReply_CpuInfo, error) {
	if usage, err := utils.NewCPUUsage(time.Millisecond * 100); err != nil {
		return nil, err
	} else {
		return usage.Infos(), nil
	}
}

func (p *pipe) Close() {
	if p.input != nil {
		if glog.GetLevel() >= glog.TraceLevel {
			glog.Trace(fmt.Sprintf("Closing %s input pipe", p.name))
		}

		p.input.Close()
		p.input = nil
	}

	if p.output != nil {
		if glog.GetLevel() >= glog.TraceLevel {
			glog.Trace(fmt.Sprintf("Closing %s output pipe", p.name))
		}
		p.output.Close()
		p.output = nil
	}
}

type pseudoTTY struct {
	pty    *os.File
	ptx    *os.File
	stdin  *pipe
	stdout *pipe
	stderr *pipe
}

func (t *pseudoTTY) SetupTrueTTY() (err error) {
	if t.pty == nil {
		return fmt.Errorf("no pty available")
	}

	// Make this a controlling terminal
	if err := unix.IoctlSetInt(int(t.pty.Fd()), unix.TIOCSCTTY, 0); err != nil {
		return fmt.Errorf("failed to set controlling terminal: %v", err)
	}

	// Set terminal attributes for proper terminal behavior
	var terminalSettings unix.Termios
	if err = termios.Tcgetattr(t.pty.Fd(), &terminalSettings); err != nil {
		return fmt.Errorf("failed to get terminal attributes: %v", err)
	}

	// Enable canonical mode and echo
	terminalSettings.Lflag |= unix.ICANON | unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ISIG

	// Set input flags
	terminalSettings.Iflag |= unix.ICRNL | unix.IXON
	terminalSettings.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.IXOFF

	// Set output flags
	terminalSettings.Oflag |= unix.OPOST | unix.ONLCR

	// Set control flags
	terminalSettings.Cflag |= unix.CREAD | unix.CLOCAL
	terminalSettings.Cflag &^= unix.CSTOPB | unix.PARENB

	// Set control characters
	terminalSettings.Cc[unix.VINTR] = 0x03  // Ctrl-C
	terminalSettings.Cc[unix.VQUIT] = 0x1C  // Ctrl-\
	terminalSettings.Cc[unix.VERASE] = 0x7F // DEL
	terminalSettings.Cc[unix.VKILL] = 0x15  // Ctrl-U
	terminalSettings.Cc[unix.VEOF] = 0x04   // Ctrl-D
	terminalSettings.Cc[unix.VSTART] = 0x11 // Ctrl-Q
	terminalSettings.Cc[unix.VSTOP] = 0x13  // Ctrl-S
	terminalSettings.Cc[unix.VSUSP] = 0x1A  // Ctrl-Z

	if err = termios.Tcsetattr(t.pty.Fd(), termios.TCSANOW, &terminalSettings); err != nil {
		return fmt.Errorf("failed to set terminal attributes: %v", err)
	}

	return nil
}

func (t *pseudoTTY) EnableRawMode() (err error) {
	if t.pty == nil {
		return fmt.Errorf("no pty available")
	}

	var terminalSettings unix.Termios
	if err = termios.Tcgetattr(t.pty.Fd(), &terminalSettings); err != nil {
		return fmt.Errorf("failed to get terminal attributes: %v", err)
	}

	termios.Cfmakeraw(&terminalSettings)
	// Disable canonical mode and echo for raw mode
	//terminalSettings.Lflag &^= unix.ICANON | unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ECHONL | unix.ISIG | unix.IEXTEN
	//terminalSettings.Iflag &^= unix.ICRNL | unix.INLCR | unix.IGNCR | unix.IXON | unix.IXOFF
	//terminalSettings.Oflag &^= unix.OPOST

	// Set minimum characters and timeout for non-blocking read
	terminalSettings.Cc[unix.VMIN] = 1
	terminalSettings.Cc[unix.VTIME] = 0

	if err = termios.Tcsetattr(t.pty.Fd(), termios.TCSANOW, &terminalSettings); err != nil {
		return fmt.Errorf("failed to set terminal attributes: %v", err)
	}

	return nil
}

func newPipe(name string, nonblocking bool) (p *pipe, err error) {
	fds := []int{0, 0}

	if err = unix.Pipe(fds); err != nil {
		return nil, fmt.Errorf("failed to create pipe: %v", err)
	}

	p = &pipe{
		name:   name,
		input:  os.NewFile(uintptr(fds[0]), fmt.Sprintf("/dev/%s/0", name)),
		output: os.NewFile(uintptr(fds[1]), fmt.Sprintf("/dev/%s/1", name)),
	}

	syscall.CloseOnExec(fds[0])
	syscall.CloseOnExec(fds[1])
	err = syscall.SetNonblock(fds[0], nonblocking)
	return
}

func newTTY(termSize *cakeagent.CakeAgent_ExecuteRequest_TerminalSize) (tty *pseudoTTY, err error) {
	tty = &pseudoTTY{}

	if tty.stderr, err = newPipe("stderr", true); err != nil {
		return nil, fmt.Errorf("failed to stderr pipe: %v", err)
	}

	if termSize.Cols > 0 && termSize.Rows > 0 {
		if glog.GetLevel() >= glog.TraceLevel {
			glog.Tracef("Opening pty with size %dx%d", termSize.Rows, termSize.Cols)
		}

		if tty.ptx, tty.pty, err = pty.Open(); err == nil {
			/*if err = tty.EnableRawMode(); err != nil {
				return nil, fmt.Errorf("failed to setup true tty: %v", err)
			}*/
			tty.SetTermSize(termSize.Rows, termSize.Cols)
		}
	} else {
		if glog.GetLevel() >= glog.TraceLevel {
			glog.Trace("Opening pty without size")
		}

		if tty.stdin, err = newPipe("stdin", false); err != nil {
			return nil, fmt.Errorf("failed to input pipe pty: %v", err)
		}

		if tty.stdout, err = newPipe("stdout", true); err != nil {
			tty.stdin.Close()
			return nil, fmt.Errorf("failed to stdout pipe: %v", err)
		}
	}

	return
}

func (t *pseudoTTY) IsTTY() bool {
	return t.pty != nil
}

func (t *pseudoTTY) EOF() {
	if glog.GetLevel() >= glog.TraceLevel {
		glog.Trace("EOF")
	}

	if t.ptx != nil {
		t.ptx.Close()
	} else if t.stdin != nil {
		t.stdin.output.Close()
		t.stdin.output = nil
	}
}

func (t *pseudoTTY) Close() {
	if t.pty != nil || t.stdin != nil {
		if glog.GetLevel() >= glog.TraceLevel {
			glog.Trace("Closing pty")
		}

		if t.pty != nil {
			t.pty.Close()
		}

		if t.ptx != nil {
			t.ptx.Close()
		}

		if t.stdin != nil {
			t.stdin.Close()
		}

		if t.stdout != nil {
			t.stdout.Close()
		}
	}

	t.ptx = nil
	t.pty = nil
	t.stdin = nil
	t.stdout = nil
}

func (t *pseudoTTY) StdinReader() *os.File {
	if t.pty != nil {
		return t.pty
	}

	return t.stdin.input
}

func (t *pseudoTTY) StdoutWriter() *os.File {
	if t.pty != nil {
		return t.pty
	}

	return t.stdout.output
}

func (t *pseudoTTY) StderrWriter() *os.File {
	return t.stderr.output
}

func (t *pseudoTTY) StdoutReader() *os.File {
	if t.ptx != nil {
		return t.ptx
	}

	return t.stdout.input
}

func (t *pseudoTTY) StderrReader() *os.File {
	return t.stderr.input
}

func (t *pseudoTTY) Input() *os.File {
	return t.stderr.input
}

func (t *pseudoTTY) SetTermSize(rows int32, cols int32) (err error) {
	if t.pty != nil {
		if glog.GetLevel() >= glog.TraceLevel {
			glog.Trace("SetTermSize")
		}

		dimensions := unix.Winsize{
			Row:    uint16(rows),
			Col:    uint16(cols),
			Xpixel: 0,
			Ypixel: 0,
		}

		err = unix.IoctlSetWinsize(int(t.pty.Fd()), unix.TIOCSWINSZ, &dimensions)
	}

	return
}

func (t *pseudoTTY) WriteToStdin(data []byte) (n int, err error) {
	if t.ptx != nil {
		n, err = t.ptx.Write(data)
	} else if t.stdin != nil {
		n, err = t.stdin.output.Write(data)
	}
	return
}

func (s *server) IpAddresses() ([]networkInterface, error) {
	var result []networkInterface

	if interfaces, err := net.Interfaces(); err != nil {
		return nil, err
	} else {
		// We need to find interface with an address
		// that is not a loopback address
		// and that is not a link local address
		// and that is not a multicast address
		// and that is not a broadcast address
		// and that is not a point to point address
		for _, iface := range interfaces {
			if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
				inf := networkInterface{
					Name:      iface.Name,
					Index:     iface.Index,
					Addresses: []string{},
				}

				if addrs, err := iface.Addrs(); err == nil {
					for _, addr := range addrs {
						var ip net.IP
						switch v := addr.(type) {
						case *net.IPNet:
							ip = v.IP
						case *net.IPAddr:
							ip = v.IP
						}

						if ip != nil && !ip.IsLoopback() && ip.IsGlobalUnicast() {
							inf.Addresses = append(inf.Addresses, ip.String())
						}
					}

					sort.Slice(inf.Addresses, func(i, j int) bool {
						return len(inf.Addresses[i]) < len(inf.Addresses[j])
					})

					result = append(result, inf)
				}
			}
		}
	}

	return result, nil
}

func (s *server) Info(ctx context.Context, req *cakeagent.CakeAgent_Empty) (reply *cakeagent.CakeAgent_InfoReply, err error) {
	var cpuInfo *cakeagent.CakeAgent_InfoReply_CpuInfo
	var diskInfos []*cakeagent.CakeAgent_InfoReply_DiskInfo

	// Prepare CPU info
	if cpuInfo, err = collectCpuUsage(); err != nil {
		return nil, err
	}

	if diskInfos, err = collectDiskInfos(); err != nil {
		return nil, err
	}

	reply = &cakeagent.CakeAgent_InfoReply{
		Ipaddresses:  []string{},
		CpuCount:     int32(runtime.NumCPU()),
		DiskInfos:    diskInfos,
		Cpu:          cpuInfo,
		AgentVersion: version.VERSION,
		Memory:       collectMemoryUsage(),
	}

	// Retrieve system information
	if reply.Hostname, err = os.Hostname(); err != nil {
		return nil, err
	}

	// Get system uptime
	if host, err := sysinfo.Host(); err == nil {
		info := host.Info()

		reply.Osname = info.OS.Platform
		reply.Release = info.OS.Codename
		reply.Version = info.OS.Version
		reply.Uptime = uint64(time.Now().Nanosecond() - info.BootTime.Nanosecond())
	}

	// Get IP addresses
	if addresses, err := s.IpAddresses(); err != nil {
		return nil, err
	} else {
		for _, address := range addresses {
			reply.Ipaddresses = append(reply.Ipaddresses, address.Addresses...)
		}
	}

	return reply, nil
}

func (s *server) Ping(ctx context.Context, req *cakeagent.CakeAgent_PingRequest) (reply *cakeagent.CakeAgent_PingReply, err error) {
	now := time.Now().UnixNano()

	reply = &cakeagent.CakeAgent_PingReply{
		Message:           fmt.Sprintf("pong: %s", req.Message),
		RequestTimestamp:  req.Timestamp,
		ResponseTimestamp: now,
	}

	glog.Debugf("Ping received: %s, responding at %d", req.Message, now)

	return reply, nil
}

func CurrentUsage() (usage *utils.CPUUsage, err error) {
	return utils.NewCPUUsage(0)
}

// GetSystemInfo returns system information using the same logic as the gRPC Info method
func GetSystemInfo() (*cakeagent.CakeAgent_InfoReply, error) {
	agent, err := event.NewAgent(time.Second)
	if err != nil {
		return nil, err
	}

	s := &server{
		agent: agent,
	}

	ctx := context.Background()
	empty := &cakeagent.CakeAgent_Empty{}

	return s.Info(ctx, empty)
}

// PingService sends a ping request to the server and returns the response
func PingService(message string) (*cakeagent.CakeAgent_PingReply, error) {
	agent, err := event.NewAgent(time.Second)
	if err != nil {
		return nil, err
	}

	s := &server{
		agent: agent,
	}

	ctx := context.Background()
	req := &cakeagent.CakeAgent_PingRequest{
		Message:   message,
		Timestamp: time.Now().UnixNano(),
	}

	return s.Ping(ctx, req)
}

// MountService performs mount operations and returns the response
func MountService(mounts []MountVirtioFSRequest) (*cakeagent.CakeAgent_MountReply, error) {
	agent, err := event.NewAgent(time.Second)
	if err != nil {
		return nil, err
	}

	s := &server{
		agent: agent,
	}

	ctx := context.Background()

	// Convert input to proto format
	var protoMounts []*cakeagent.CakeAgent_MountRequest_MountVirtioFS
	for _, mount := range mounts {
		protoMounts = append(protoMounts, &cakeagent.CakeAgent_MountRequest_MountVirtioFS{
			Name:     mount.Name,
			Target:   mount.Target,
			Uid:      int32(mount.Uid),
			Gid:      int32(mount.Gid),
			Readonly: mount.Readonly,
			Early:    mount.Early,
		})
	}

	req := &cakeagent.CakeAgent_MountRequest{
		Mounts: protoMounts,
	}

	return s.Mount(ctx, req)
}

// MountVirtioFSRequest represents a mount request for the CLI
type MountVirtioFSRequest struct {
	Name     string
	Target   string
	Uid      int
	Gid      int
	Readonly bool
	Early    bool
}

// ParseMountString parses a mount string in the format "name:target[,uid=X,gid=Y,ro,early]"
// Examples:
//   - "share:/mnt/share"
//   - "data:/data,uid=1000,gid=1000"
//   - "logs:/var/log,uid=0,gid=0,ro"
//   - "config:/etc/config,ro,early"
func ParseMountString(mountStr string, early bool) (*MountVirtioFSRequest, error) {
	if mountStr == "" {
		return nil, fmt.Errorf("empty mount string")
	}

	parts := strings.Split(mountStr, ",")
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid mount string format")
	}

	// Parse name:target
	nameTarget := strings.Split(parts[0], ":")
	if len(nameTarget) != 2 {
		return nil, fmt.Errorf("mount string must be in format 'name:target[,options]'")
	}

	mount := &MountVirtioFSRequest{
		Name:     strings.TrimSpace(nameTarget[0]),
		Target:   strings.TrimSpace(nameTarget[1]),
		Uid:      1000, // Default user
		Gid:      1000, // Default group
		Readonly: false,
		Early:    early,
	}

	if mount.Name == "" {
		return nil, fmt.Errorf("mount name cannot be empty")
	}
	if mount.Target == "" {
		return nil, fmt.Errorf("mount target cannot be empty")
	}

	// Parse options
	for i := 1; i < len(parts); i++ {
		option := strings.TrimSpace(parts[i])
		if option == "" {
			continue
		}

		if option == "ro" || option == "readonly" {
			mount.Readonly = true
		} else if option == "early" {
			mount.Early = true
		} else if strings.HasPrefix(option, "uid=") {
			uidStr := strings.TrimPrefix(option, "uid=")
			if uid, err := strconv.Atoi(uidStr); err != nil {
				return nil, fmt.Errorf("invalid uid value '%s': %v", uidStr, err)
			} else {
				mount.Uid = uid
			}
		} else if strings.HasPrefix(option, "gid=") {
			gidStr := strings.TrimPrefix(option, "gid=")
			if gid, err := strconv.Atoi(gidStr); err != nil {
				return nil, fmt.Errorf("invalid gid value '%s': %v", gidStr, err)
			} else {
				mount.Gid = gid
			}
		} else {
			return nil, fmt.Errorf("unknown mount option: %s", option)
		}
	}

	return mount, nil
}

// ParseMountStrings parses multiple mount strings and returns a slice of MountVirtioFSRequest
func ParseMountStrings(mountStrings []string, early bool) ([]MountVirtioFSRequest, error) {
	if len(mountStrings) == 0 {
		return nil, fmt.Errorf("no mount strings provided")
	}

	mounts := make([]MountVirtioFSRequest, 0, len(mountStrings))
	for i, mountStr := range mountStrings {
		mount, err := ParseMountString(mountStr, early)
		if err != nil {
			return nil, fmt.Errorf("error parsing mount string %d ('%s'): %v", i+1, mountStr, err)
		}
		mounts = append(mounts, *mount)
	}

	return mounts, nil
}

func (s *server) Shutdown(ctx context.Context, req *cakeagent.CakeAgent_Empty) (reply *cakeagent.CakeAgent_RunReply, err error) {
	home, _ := os.UserHomeDir()
	reply = &cakeagent.CakeAgent_RunReply{}

	if runtime.GOOS == "darwin" {
		cmd := exec.Command("shutdown", "-h", "+1s")

		cmd.Dir = home
		cmd.Env = os.Environ()

		if err = cmd.Start(); err != nil {
			if exiterr, ok := err.(*exec.ExitError); ok {
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					reply.ExitCode = int32(status.ExitStatus())
				} else {
					reply.Stderr = []byte(fmt.Sprintf(strErrFailedToGetExitStatus, err))
					reply.ExitCode = 1
				}
			} else {
				reply.Stderr = []byte(fmt.Sprintf(strErrFailedToRunCommand, err))
				reply.ExitCode = 1
			}

			err = nil
		}
	} else {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		cmd := exec.Command("shutdown", "now")

		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Dir = home
		cmd.Env = os.Environ()

		if err = cmd.Run(); err != nil {
			if exiterr, ok := err.(*exec.ExitError); ok {
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					reply.ExitCode = int32(status.ExitStatus())
					err = nil
				} else {
					return nil, fmt.Errorf(strErrFailedToGetExitStatus, err)
				}
			} else {
				return nil, fmt.Errorf(strErrFailedToRunCommand, err)
			}
		}

		if b := stdout.Bytes(); len(b) > 0 {
			reply.Stdout = b
		}

		if b := stderr.Bytes(); len(b) > 0 {
			reply.Stderr = b
		}
	}

	return
}

func (s *server) Run(ctx context.Context, req *cakeagent.CakeAgent_RunCommand) (reply *cakeagent.CakeAgent_RunReply, err error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	home, _ := os.UserHomeDir()
	reply = &cakeagent.CakeAgent_RunReply{}

	arguments := append([]string{req.Command.GetCommand()}, req.Command.Args...)

	cmd := exec.Command("/bin/sh", "-c", strings.Join(arguments, " "))

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Dir = home
	cmd.Env = os.Environ()

	if len(req.Input) > 0 {
		cmd.Stdin = bytes.NewReader(req.Input)
	}

	if err = cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				reply.ExitCode = int32(status.ExitStatus())
				err = nil
			} else {
				return nil, fmt.Errorf(strErrFailedToGetExitStatus, err)
			}
		} else {
			return nil, fmt.Errorf(strErrFailedToRunCommand, err)
		}
	}

	if b := stdout.Bytes(); len(b) > 0 {
		reply.Stdout = b
	}

	if b := stderr.Bytes(); len(b) > 0 {
		reply.Stderr = b
	}

	return
}

func (s *server) execute(command *cakeagent.CakeAgent_ExecuteRequest_ExecuteCommand, termSize *cakeagent.CakeAgent_ExecuteRequest_TerminalSize, stream cakeagent.CakeAgentService_ExecuteServer) (err error) {
	if tty, e := newTTY(termSize); e != nil {
		err = fmt.Errorf("failed to open pty: %v", e)
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		home, _ := os.UserHomeDir()
		var wg sync.WaitGroup
		var message cakeagent.CakeAgent_ExecuteResponse
		var cmd *exec.Cmd

		doCancel := func(reason string) {
			if reason != "" {
				glog.Error(reason)
			}

			if ctx.Err() == nil {
				if glog.GetLevel() >= glog.TraceLevel {
					glog.Trace("Canceling context")
				}
				cancel()
			}

			tty.Close()
		}

		fowardOutput := func(ctx context.Context, channel int, input *os.File) {
			var name string
			buffer := make([]byte, 1024)
			var totalSent uint64 = 0

			if channel == 0 {
				name = "stdout"
			} else {
				name = "stderr"
			}

			wg.Add(1)
			defer wg.Done()

			for {
				var readfd unix.FdSet
				var timeout = unix.Timeval{
					Sec:  0,
					Usec: 100,
				}

				select {
				case <-ctx.Done():
					if glog.GetLevel() >= glog.TraceLevel {
						glog.Tracef("Closing output %s, totalSent=%d", name, totalSent)
					}
					return
				default:
					readfd.Zero()
					readfd.Set(int(input.Fd()))

					if _, err := unix.Select(int(input.Fd())+1, &readfd, nil, nil, &timeout); err != nil {
						if utils.IsEINTR(err) {
							continue
						} else {
							glog.Errorf("Error selecting %s: %v", name, err)
							return
						}
					}

					if readfd.IsSet(int(input.Fd())) {
						if available, err := input.Read(buffer); err != nil {
							if utils.IsEAGAIN(err) {
								if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
									glog.Tracef("EOF %s, totalSent=%d, process exited", name, totalSent)
									return
								}
								// Sleep a bit to avoid busy waiting
								time.Sleep(10 * time.Nanosecond)
							} else {
								if err != io.EOF && err != io.ErrClosedPipe {
									doCancel(fmt.Sprintf("Error reading output: %v", err))
								}

								if glog.GetLevel() >= glog.TraceLevel {
									glog.Tracef("EOF %s, totalSent=%d", name, totalSent)
								}
								return
							}
						} else if available > 0 {
							if glog.GetLevel() >= glog.TraceLevel {
								glog.Tracef("reading %s: %d", name, len(buffer[:available]))
							}

							totalSent += uint64(len(buffer[:available]))

							var message *cakeagent.CakeAgent_ExecuteResponse

							if channel == 0 {
								message = &cakeagent.CakeAgent_ExecuteResponse{
									Response: &cakeagent.CakeAgent_ExecuteResponse_Stdout{
										Stdout: buffer[:available],
									},
								}
							} else {
								message = &cakeagent.CakeAgent_ExecuteResponse{
									Response: &cakeagent.CakeAgent_ExecuteResponse_Stderr{
										Stderr: buffer[:available],
									},
								}
							}

							if err = stream.Send(message); err != nil {
								doCancel(fmt.Sprintf("Failed to send %s: %v", name, err))
							}

							if glog.GetLevel() >= glog.TraceLevel {
								glog.Tracef("Sent %s: %d", name, len(buffer[:available]))
							}
						} else {
							if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
								if glog.GetLevel() >= glog.TraceLevel {
									glog.Trace("Command is still running, no data to read")
								}
							} else {
								if glog.GetLevel() >= glog.TraceLevel {
									glog.Tracef("EOF reading output %d, totalSent=%d, %v", channel, totalSent, err)
								}

								return
							}
						}
					} else if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
						glog.Tracef("EOF %s, totalSent=%d, process exited", name, totalSent)
						return
					}
				}
			}
		}

		fowardStdin := func() {
			for {
				select {
				case <-ctx.Done():
					if glog.GetLevel() >= glog.TraceLevel {
						glog.Trace("Closing stdin")
					}
					return
				default:
					if request, err := stream.Recv(); err != nil {
						if !utils.IsGRPCCancel(err) {
							if err != io.EOF && err != context.Canceled {
								doCancel(fmt.Sprintf("Failed to receive message: %v", err))
							}
						}
					} else if size := request.GetSize(); size != nil {
						tty.SetTermSize(size.Rows, size.Cols)
					} else if input := request.GetInput(); input != nil {
						if len(input) > 0 {
							if _, err := tty.WriteToStdin(input); err != nil {
								doCancel(fmt.Sprintf("Failed to write to stdin: %v", err))
							}

							if glog.GetLevel() >= glog.TraceLevel {
								glog.Tracef("Wrote to stdin: %d", len(input))
							}
						}
					} else if request.GetEof() {
						tty.EOF()
						return
					} else {
						glog.Error("Unknown message")
					}
				}
			}
		}

		cleanup := func() {
			if glog.GetLevel() >= glog.TraceLevel {
				glog.Trace("Cleanup")
			}

			wg.Wait()
			cancel()
			tty.Close()

			if cmd != nil {
				message = cakeagent.CakeAgent_ExecuteResponse{
					Response: &cakeagent.CakeAgent_ExecuteResponse_ExitCode{
						ExitCode: int32(cmd.ProcessState.ExitCode()),
					},
				}
			} else {
				message = cakeagent.CakeAgent_ExecuteResponse{
					Response: &cakeagent.CakeAgent_ExecuteResponse_ExitCode{
						ExitCode: 1,
					},
				}
			}

			if err = stream.Send(&message); err != nil {
				glog.Errorf("Failed to send exit code: %v", err)
			}

			if glog.GetLevel() >= glog.TraceLevel {
				glog.Trace("Shell session ended")
			}
		}

		if command.GetShell() {
			shell := "bash"

			if shell, err = exec.LookPath(shell); err != nil {
				shell = "/bin/sh"
				err = nil
			}
			cmd = exec.CommandContext(ctx, shell, "-i", "-l")
		} else if exc := command.GetCommand(); exc != nil {
			cmd = exec.CommandContext(ctx, exc.GetCommand(), exc.GetArgs()...)
		} else {
			err = fmt.Errorf("wrong protocol")
		}

		if err == nil {
			isTTY := tty.IsTTY()

			cmd.Stdin = tty.StdinReader()
			cmd.Stdout = tty.StdoutWriter()
			cmd.Stderr = tty.StderrWriter()
			cmd.Env = os.Environ()
			cmd.Dir = home
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Setsid:  isTTY,
				Setctty: isTTY,
				//Noctty:  !isTTY,
			}

			defer cleanup()

			if err = cmd.Start(); err == nil {

				if glog.GetLevel() >= glog.TraceLevel {
					glog.Trace("Command started")
				}

				go fowardOutput(ctx, 0, tty.StdoutReader())
				go fowardOutput(ctx, 1, tty.StderrReader())
				go fowardStdin()

				message = cakeagent.CakeAgent_ExecuteResponse{
					Response: &cakeagent.CakeAgent_ExecuteResponse_Established{
						Established: true,
					},
				}

				if err = stream.Send(&message); err != nil {
					glog.Errorf("Failed to send established message: %v", err)
					cmd.Process.Signal(syscall.SIGKILL)
				} else if err = cmd.Wait(); err != nil {
					if utils.IsUnexpectedError(err) {
						glog.Errorf("Failed to wait for command: %v", err)
					}
				}

				if glog.GetLevel() >= glog.TraceLevel {
					glog.Trace("Command exited")
				}
			} else {
				glog.Errorf("Failed to start command: %v", err)
			}

			if command.GetShell() {
				message = cakeagent.CakeAgent_ExecuteResponse{
					Response: &cakeagent.CakeAgent_ExecuteResponse_Stderr{
						Stderr: []byte("\r"),
					},
				}

				stream.Send(&message)
			}
		}

		if utils.IsUnexpectedError(err) {
			message = cakeagent.CakeAgent_ExecuteResponse{
				Response: &cakeagent.CakeAgent_ExecuteResponse_Stderr{
					Stderr: []byte(err.Error() + "\r\n"),
				},
			}

			stream.Send(&message)
		}
	}

	glog.Debug("Leave execute")

	return
}

func (s *server) Execute(stream cakeagent.CakeAgentService_ExecuteServer) (err error) {
	var request *cakeagent.CakeAgent_ExecuteRequest
	var command *cakeagent.CakeAgent_ExecuteRequest_ExecuteCommand
	var termSize *cakeagent.CakeAgent_ExecuteRequest_TerminalSize

	if request, err = stream.Recv(); err != nil {
		glog.Errorf("Failed to receive command message: %v", err)
		return
	}

	if termSize = request.GetSize(); termSize == nil {
		return fmt.Errorf("empty term size")
	}

	if request, err = stream.Recv(); err != nil {
		glog.Errorf("Failed to receive command message: %v", err)
		return
	}

	if command = request.GetCommand(); command == nil {
		return fmt.Errorf("empty command")
	}

	return s.execute(command, termSize, stream)
}

func (s *server) Tunnel(stream cakeagent.CakeAgentService_TunnelServer) error {
	if glog.GetLevel() >= glog.TraceLevel {
		glog.Trace("Tunnel requested")
	}

	if tunnelServer, err := tunnel.NewTunnelServer(stream); err == nil {
		if glog.GetLevel() >= glog.TraceLevel {
			glog.Trace("Tunnel server created")
		}

		return tunnelServer.Stream(nil)
	} else {
		glog.Errorf("Failed to create tunnel server: %v", err)

		// If we have an error, we need to send it back to the client
		message := &cakeagent.CakeAgent_TunnelMessage{
			Message: &cakeagent.CakeAgent_TunnelMessage_Error{
				Error: err.Error(),
			},
		}

		stream.Send(message)

		return err
	}
}

func (s *server) Mount(ctx context.Context, request *cakeagent.CakeAgent_MountRequest) (*cakeagent.CakeAgent_MountReply, error) {
	return mount.Mount(ctx, request)
}

func (s *server) Umount(ctx context.Context, request *cakeagent.CakeAgent_MountRequest) (*cakeagent.CakeAgent_MountReply, error) {
	return mount.Umount(ctx, request)
}

func (s *server) ResizeDisk(context.Context, *cakeagent.CakeAgent_Empty) (*cakeagent.CakeAgent_ResizeReply, error) {
	if err := resize.ResizeDisk(); err == nil {
		return &cakeagent.CakeAgent_ResizeReply{
			Response: &cakeagent.CakeAgent_ResizeReply_Success{
				Success: true,
			},
		}, nil
	} else {
		return &cakeagent.CakeAgent_ResizeReply{
			Response: &cakeagent.CakeAgent_ResizeReply_Failure{
				Failure: err.Error(),
			},
		}, nil
	}
}

func (s *server) Events(_ *cakeagent.CakeAgent_Empty, stream cakeagent.CakeAgentService_EventsServer) error {
	var ipv6 string
	var ipv4 string

	sendError := func(err string) {
		response := &cakeagent.CakeAgent_TunnelPortForwardEvent{
			Event: &cakeagent.CakeAgent_TunnelPortForwardEvent_Error{
				Error: err,
			},
		}

		if err := stream.Send(response); err != nil {
			glog.Errorf("Failed to send error message: %v", err)
		}
	}

	hostAddres := func(ip string) string {
		switch ip {
		case "0.0.0.0":
			return ipv4
		case "::":
			if ipv6 == "" {
				return ipv4
			}

			return ipv6
		}

		return ip
	}

	if interfaces, err := s.IpAddresses(); err != nil {
		sendError("failed to get network interfaces")
		glog.Errorf("Failed to get network interfaces: %v", err)
	} else if s.agent == nil {
		sendError("agent not initialized")
		glog.Error("agent not initialized")
	} else {
		for _, str := range interfaces[0].Addresses {
			if addr, err := netip.ParseAddr(str); err == nil {
				if glog.GetLevel() >= glog.TraceLevel {
					glog.Tracef("Found IP address: %s, len:%d", str, addr.BitLen())
				}

				if addr.Is4() {
					if ipv4 == "" {
						ipv4 = str
						if glog.GetLevel() >= glog.TraceLevel {
							glog.Tracef("Found IPv4: %s", ipv4)
						}
					}
				} else if addr.Is6() {
					if ipv6 == "" {
						ipv6 = str
						if glog.GetLevel() >= glog.TraceLevel {
							glog.Tracef("Found IPv6: %s", ipv6)
						}
					}
				}
			}
		}

		if glog.GetLevel() >= glog.TraceLevel {
			glog.Tracef("Found %d network interfaces: %s", len(interfaces), utils.ToJSON(interfaces))
		}

		responses := make(chan *api.Event)

		go s.agent.Events(stream.Context(), responses)

		for response := range responses {
			forwardEvent := &cakeagent.CakeAgent_TunnelPortForwardEvent_ForwardEvent{
				AddedPorts:   make([]*cakeagent.CakeAgent_TunnelPortForwardEvent_TunnelPortForward, 0, len(response.LocalPortsAdded)),
				RemovedPorts: make([]*cakeagent.CakeAgent_TunnelPortForwardEvent_TunnelPortForward, 0, len(response.LocalPortsRemoved)),
			}

			for _, port := range response.LocalPortsAdded {
				if ip := net.ParseIP(port.Ip); ip != nil {
					if !ip.IsMulticast() && !ip.IsLinkLocalUnicast() && !ip.IsLoopback() {
						forwardEvent.AddedPorts = append(forwardEvent.AddedPorts, &cakeagent.CakeAgent_TunnelPortForwardEvent_TunnelPortForward{
							Ip:   hostAddres(port.Ip),
							Port: port.Port,
						})
					}
				}
			}

			for _, port := range response.LocalPortsRemoved {
				if ip := net.ParseIP(port.Ip); ip != nil {
					if !ip.IsMulticast() && !ip.IsLinkLocalUnicast() && !ip.IsLoopback() {
						forwardEvent.RemovedPorts = append(forwardEvent.RemovedPorts, &cakeagent.CakeAgent_TunnelPortForwardEvent_TunnelPortForward{
							Ip:   hostAddres(port.Ip),
							Port: port.Port,
						})
					}
				}
			}
			message := &cakeagent.CakeAgent_TunnelPortForwardEvent{
				Event: &cakeagent.CakeAgent_TunnelPortForwardEvent_ForwardEvent_{
					ForwardEvent: forwardEvent,
				},
			}

			if err := stream.Send(message); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *server) CurrentUsage(req *cakeagent.CakeAgent_CurrentUsageRequest, stream grpc.ServerStreamingServer[cakeagent.CakeAgent_CurrentUsageReply]) (err error) {
	if req.Frequency == 0 {
		req.Frequency = 1
	}
	var usage *utils.CPUUsage

	if usage, err = utils.NewCPUUsage(time.Millisecond * 100); err != nil {
		glog.Errorf("Failed to setup CPU usage: %v", err)
		return err
	}

	frequency := time.Second / time.Duration(req.Frequency)
	ticker := time.NewTicker(frequency)
	defer ticker.Stop()

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-ticker.C:
			// Get detailed CPU times
			if cpuInfo, err := usage.Collect(); err != nil {
				glog.Warnf("Failed to collect CPU usage: %v", err)
				continue
			} else {
				reply := cakeagent.CakeAgent_CurrentUsageReply{
					CpuCount: int32(runtime.NumCPU()),
					CpuInfos: cpuInfo,
					Memory:   collectMemoryUsage(),
				}

				if err = stream.Send(&reply); err != nil {
					glog.Errorf("Failed to send current usage data: %v", err)
					return err
				}
			}
		}
	}
}

func createListener(listen string) (listener net.Listener, err error) {
	var u *url.URL

	if u, err = url.Parse(listen); err != nil {
		err = fmt.Errorf("failed to parse address: %s", err)
	} else {
		var hostname = u.Hostname()

		switch u.Scheme {
		case "virtio":
			if listener, err = serialport.Listen("/dev/virtio-ports/" + hostname); err != nil {
				err = fmt.Errorf("failed to listen on virtio: %v", err)
			}
		case "vsock":
			var port int
			const vsockFailed = "failed to listen on vsock: %v"

			if u.Port() == "" {
				port = 5000
			} else if port, err = strconv.Atoi(u.Port()); err != nil {
				err = fmt.Errorf("invalid port: %v", err)
			} else if hostname == "any" {
				if listener, err = vsock.Listen(uint32(port), nil); err != nil {
					err = fmt.Errorf(vsockFailed, err)
				}
			} else if hostname == "host" {
				if listener, err = vsock.ListenContextID(vsock.Host, uint32(port), nil); err != nil {
					err = fmt.Errorf(vsockFailed, err)
				}
			} else if hostname == "hypervisor" {
				if listener, err = vsock.ListenContextID(vsock.Hypervisor, uint32(port), nil); err != nil {
					err = fmt.Errorf(vsockFailed, err)
				}
			} else {
				var cid int

				if cid, err = strconv.Atoi(hostname); err != nil {
					err = fmt.Errorf("invalid cid: %v", err)
				} else if listener, err = vsock.ListenContextID(uint32(cid), uint32(port), nil); err != nil {
					err = fmt.Errorf(vsockFailed, err)
				}
			}
		case "tcp":
			var port int

			if u.Port() == "" {
				port = 5000
			} else if port, err = strconv.Atoi(u.Port()); err != nil {
				err = fmt.Errorf("invalid port: %v", err)
			} else {
				listener, err = net.Listen("tcp", fmt.Sprintf("%s:%d", hostname, port))
			}
		case "unix":
			listener, err = net.Listen("unix", u.Path)
		default:
			err = fmt.Errorf("unsupported address scheme: %s", u.Scheme)
		}
	}

	return
}

func StartServer(cfg *types.Config) (grpcServer *grpc.Server, err error) {
	var listener net.Listener
	var agent guestagent.Agent

	if listener, err = createListener(cfg.Address); err != nil {
		err = fmt.Errorf("failed to parse address: %s", err)
	} else if agent, err = event.NewAgent(cfg.TickEvent); err != nil {
		err = fmt.Errorf("failed to create agent: %s", err)
	} else {
		if cfg.TlsCert != "" && cfg.TlsKey != "" && cfg.CaCert != "" {

			certPool := x509.NewCertPool()

			if certificate, e := tls.LoadX509KeyPair(cfg.TlsCert, cfg.TlsKey); e != nil {
				err = fmt.Errorf("failed to read certificate files: %s", e)
			} else if bs, e := os.ReadFile(cfg.CaCert); e != nil {
				err = fmt.Errorf("failed to read client ca cert: %s", e)
			} else if ok := certPool.AppendCertsFromPEM(bs); !ok {
				err = fmt.Errorf("failed to append client certs")
			} else {
				transportCreds := credentials.NewTLS(&tls.Config{
					Certificates: []tls.Certificate{certificate},
					ClientAuth:   tls.RequireAndVerifyClientCert,
					ClientCAs:    certPool,
					RootCAs:      certPool,
					ServerName:   "localhost",
				})

				grpcServer = grpc.NewServer(grpc.Creds(transportCreds))
			}
		} else {
			grpcServer = grpc.NewServer()
		}

		if err == nil {
			glog.Infof("Starting gRPC server on %s", cfg.Address)

			cakeagent.RegisterCakeAgentServiceServer(grpcServer, &server{
				agent: agent,
			})

			var ctx context.Context
			var cancel context.CancelFunc

			if console := console.NewConsole(); console != nil {
				ctx, cancel = context.WithCancel(context.Background())

				go console.Forward(ctx)
			}

			if err = grpcServer.Serve(listener); err != nil {
				err = fmt.Errorf("failed to serve: %s", err)
			}

			if cancel != nil {
				cancel()
			}
		}
	}

	return
}
