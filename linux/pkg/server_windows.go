//go:build windows
// +build windows

package pkg

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Fred78290/cakeagent/cmd/types"
	"github.com/Fred78290/cakeagent/pkg/cakeagent"
	"github.com/Fred78290/cakeagent/pkg/mount"
	"github.com/Fred78290/cakeagent/pkg/resize"
	"github.com/Fred78290/cakeagent/pkg/utils"
	"github.com/Fred78290/cakeagent/version"
	"github.com/elastic/go-sysinfo"
	"github.com/shirou/gopsutil/v4/disk"
	psnet "github.com/shirou/gopsutil/v4/net"
	glog "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	strErrFailedToGetExitStatus = "failed to get exit status: %v"
	strErrFailedToRunCommand    = "failed to run command: %v"
)

type server struct {
	cakeagent.UnimplementedCakeAgentServiceServer
}

type MountVirtioFSRequest struct {
	Name     string
	Target   string
	Uid      int
	Gid      int
	Readonly bool
	Early    bool
}

func collectDiskInfos() (diskInfos []*cakeagent.CakeAgent_InfoReply_DiskInfo, err error) {
	var partitions []disk.PartitionStat

	if partitions, err = disk.Partitions(true); err != nil {
		return
	}

	diskInfos = make([]*cakeagent.CakeAgent_InfoReply_DiskInfo, 0, len(partitions))

	for _, partition := range partitions {
		if !slices.Contains(partition.Opts, "nobrowse") && !slices.Contains(partition.Opts, "nodev") {
			if diskInfo, usageErr := disk.Usage(partition.Mountpoint); usageErr == nil && diskInfo != nil {
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

	return
}

func collectMemoryUsage() *cakeagent.CakeAgent_InfoReply_MemoryInfo {
	memUsage := utils.GetMemoryUsage()

	return &cakeagent.CakeAgent_InfoReply_MemoryInfo{
		Total:     memUsage.Total,
		Free:      memUsage.Free,
		Used:      memUsage.Used,
		SwapTotal: memUsage.SwapTotal,
		SwapFree:  memUsage.SwapFree,
		SwapUsed:  memUsage.SwapUsed,
	}
}

func collectCpuUsage() (*cakeagent.CakeAgent_InfoReply_CpuInfo, error) {
	if usage, err := utils.NewCPUUsage(time.Millisecond * 100); err != nil {
		return nil, err
	} else {
		return usage.Infos(), nil
	}
}

func (s *server) IpAddresses() ([]*cakeagent.CakeAgent_InfoReply_NetworkInfo, error) {
	var result []*cakeagent.CakeAgent_InfoReply_NetworkInfo

	ioCounters := make(map[string]psnet.IOCountersStat)
	if counters, err := psnet.IOCounters(true); err == nil {
		for _, c := range counters {
			ioCounters[c.Name] = c
		}
	}

	if interfaces, err := net.Interfaces(); err != nil {
		return nil, err
	} else {
		for _, iface := range interfaces {
			if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagRunning != 0 && iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) == 0 {
				c := ioCounters[iface.Name]
				inf := &cakeagent.CakeAgent_InfoReply_NetworkInfo{
					Interface:       iface.Name,
					MacAddress:      iface.HardwareAddr.String(),
					Addresses:       []string{},
					Mtu:             int32(iface.MTU),
					BytesReceived:   uint64(c.BytesRecv),
					BytesSent:       uint64(c.BytesSent),
					PacketsReceived: uint64(c.PacketsRecv),
					PacketsSent:     uint64(c.PacketsSent),
				}

				if addrs, err := iface.Addrs(); err == nil {
					for _, addr := range addrs {
						switch v := addr.(type) {
						case *net.IPNet:
							if ip := v.IP; ip != nil && !ip.IsLoopback() && ip.IsGlobalUnicast() {
								inf.Addresses = append(inf.Addresses, v.String())
							}
						case *net.IPAddr:
							if ip := v.IP; ip != nil && !ip.IsLoopback() && ip.IsGlobalUnicast() {
								inf.Addresses = append(inf.Addresses, ip.String())
							}
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
	var numOfProcesses int32

	if cpuInfo, err = collectCpuUsage(); err != nil {
		return nil, err
	}

	if diskInfos, err = collectDiskInfos(); err != nil {
		return nil, err
	}

	if process, processErr := sysinfo.Processes(); processErr == nil {
		numOfProcesses = int32(len(process))
	}

	reply = &cakeagent.CakeAgent_InfoReply{
		NetworkInfos:   []*cakeagent.CakeAgent_InfoReply_NetworkInfo{},
		CpuCount:       int32(runtime.NumCPU()),
		DiskInfos:      diskInfos,
		Cpu:            cpuInfo,
		AgentVersion:   version.VERSION,
		Memory:         collectMemoryUsage(),
		NumOfProcesses: numOfProcesses,
	}

	if reply.Hostname, err = os.Hostname(); err != nil {
		return nil, err
	}

	if host, hostErr := sysinfo.Host(); hostErr == nil {
		info := host.Info()

		reply.Osname = info.OS.Platform
		reply.Release = info.OS.Codename
		reply.Version = info.OS.Version
		reply.Uptime = uint64(time.Now().Nanosecond() - info.BootTime.Nanosecond())
	}

	if addresses, addrErr := s.IpAddresses(); addrErr != nil {
		return nil, addrErr
	} else {
		reply.NetworkInfos = append(reply.NetworkInfos, addresses...)
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

	return reply, nil
}

func (s *server) ResizeDisk(context.Context, *cakeagent.CakeAgent_Empty) (*cakeagent.CakeAgent_ResizeReply, error) {
	if err := resize.ResizeDisk(); err != nil {
		return &cakeagent.CakeAgent_ResizeReply{
			Response: &cakeagent.CakeAgent_ResizeReply_Failure{
				Failure: err.Error(),
			},
		}, nil
	}

	return &cakeagent.CakeAgent_ResizeReply{
		Response: &cakeagent.CakeAgent_ResizeReply_Success{
			Success: true,
		},
	}, nil
}

func (s *server) Mount(ctx context.Context, request *cakeagent.CakeAgent_MountRequest) (*cakeagent.CakeAgent_MountReply, error) {
	return mount.Mount(ctx, request)
}

func (s *server) Umount(ctx context.Context, request *cakeagent.CakeAgent_MountRequest) (*cakeagent.CakeAgent_MountReply, error) {
	return mount.Umount(ctx, request)
}

func (s *server) Shutdown(ctx context.Context, req *cakeagent.CakeAgent_Empty) (reply *cakeagent.CakeAgent_RunReply, err error) {
	home, _ := os.UserHomeDir()
	reply = &cakeagent.CakeAgent_RunReply{}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command("shutdown", "/s", "/t", "0")
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

	return
}

func (s *server) Run(ctx context.Context, req *cakeagent.CakeAgent_RunCommand) (reply *cakeagent.CakeAgent_RunReply, err error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	home, _ := os.UserHomeDir()
	reply = &cakeagent.CakeAgent_RunReply{}

	arguments := append([]string{req.Command.GetCommand()}, req.Command.Args...)
	cmd := exec.Command("cmd", "/C", strings.Join(arguments, " "))
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
			if cpuInfo, collectErr := usage.Collect(); collectErr != nil {
				glog.Warnf("Failed to collect CPU usage: %v", collectErr)
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

func CurrentUsage() (usage *utils.CPUUsage, err error) {
	return utils.NewCPUUsage(0)
}

func GetSystemInfo() (*cakeagent.CakeAgent_InfoReply, error) {
	s := &server{}
	return s.Info(context.Background(), &cakeagent.CakeAgent_Empty{})
}

func PingService(message string) (*cakeagent.CakeAgent_PingReply, error) {
	s := &server{}

	req := &cakeagent.CakeAgent_PingRequest{
		Message:   message,
		Timestamp: time.Now().UnixNano(),
	}

	return s.Ping(context.Background(), req)
}

func MountService(mounts []MountVirtioFSRequest) (*cakeagent.CakeAgent_MountReply, error) {
	s := &server{}

	var protoMounts []*cakeagent.CakeAgent_MountRequest_MountVirtioFS
	for _, m := range mounts {
		protoMounts = append(protoMounts, &cakeagent.CakeAgent_MountRequest_MountVirtioFS{
			Name:     m.Name,
			Target:   m.Target,
			Uid:      int32(m.Uid),
			Gid:      int32(m.Gid),
			Readonly: m.Readonly,
			Early:    m.Early,
		})
	}

	req := &cakeagent.CakeAgent_MountRequest{
		Mounts: protoMounts,
	}

	return s.Mount(context.Background(), req)
}

func ParseMountString(mountStr string, early bool) (*MountVirtioFSRequest, error) {
	if mountStr == "" {
		return nil, fmt.Errorf("empty mount string")
	}

	parts := strings.Split(mountStr, ",")
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid mount string format")
	}

	nameTarget := strings.Split(parts[0], ":")
	if len(nameTarget) != 2 {
		return nil, fmt.Errorf("mount string must be in format 'name:target[,options]'")
	}

	mount := &MountVirtioFSRequest{
		Name:     strings.TrimSpace(nameTarget[0]),
		Target:   strings.TrimSpace(nameTarget[1]),
		Uid:      1000,
		Gid:      1000,
		Readonly: false,
		Early:    early,
	}

	if mount.Name == "" {
		return nil, fmt.Errorf("mount name cannot be empty")
	}
	if mount.Target == "" {
		return nil, fmt.Errorf("mount target cannot be empty")
	}

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

func createListener(listen string) (listener net.Listener, err error) {
	var u *url.URL

	if u, err = url.Parse(listen); err != nil {
		return nil, fmt.Errorf("failed to parse address: %w", err)
	}

	hostname := u.Hostname()

	switch u.Scheme {
	case "tcp":
		port := 5000
		if u.Port() != "" {
			if port, err = strconv.Atoi(u.Port()); err != nil {
				return nil, fmt.Errorf("invalid port: %w", err)
			}
		}

		if hostname == "" {
			hostname = "127.0.0.1"
		}

		return net.Listen("tcp", fmt.Sprintf("%s:%d", hostname, port))
	case "unix", "virtio", "vsock":
		return nil, fmt.Errorf("%s transport is not supported on windows", u.Scheme)
	default:
		return nil, fmt.Errorf("unsupported address scheme: %s", u.Scheme)
	}
}

func StartServer(cfg *types.Config) (grpcServer *grpc.Server, err error) {
	var listener net.Listener

	if listener, err = createListener(cfg.Address); err != nil {
		return nil, fmt.Errorf("failed to parse address: %w", err)
	}

	if cfg.TlsCert != "" && cfg.TlsKey != "" && cfg.CaCert != "" {
		certPool := x509.NewCertPool()

		if certificate, e := tls.LoadX509KeyPair(cfg.TlsCert, cfg.TlsKey); e != nil {
			return nil, fmt.Errorf("failed to read certificate files: %w", e)
		} else if bs, e := os.ReadFile(cfg.CaCert); e != nil {
			return nil, fmt.Errorf("failed to read client ca cert: %w", e)
		} else if ok := certPool.AppendCertsFromPEM(bs); !ok {
			return nil, fmt.Errorf("failed to append client certs")
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

	glog.Infof("Starting gRPC server on %s", cfg.Address)
	cakeagent.RegisterCakeAgentServiceServer(grpcServer, &server{})

	if err = grpcServer.Serve(listener); err != nil {
		return nil, fmt.Errorf("failed to serve: %w", err)
	}

	return grpcServer, nil
}
