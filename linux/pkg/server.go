package pkg

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/Fred78290/cakeagent/cmd/types"
	"github.com/Fred78290/cakeagent/pkg/cakeagent"
	"github.com/Fred78290/cakeagent/pkg/mount"
	"github.com/Fred78290/cakeagent/pkg/serialport"
	"github.com/creack/pty"
	"github.com/elastic/go-sysinfo"
	"github.com/mdlayher/vsock"
	"github.com/pbnjay/memory"
	glog "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type server struct {
	cakeagent.UnimplementedAgentServer
}

func setTermSize(fd uintptr, rows int32, cols int32) (err error) {
	dimensions := unix.Winsize{
		Row:    uint16(rows),
		Col:    uint16(cols),
		Xpixel: 0,
		Ypixel: 0,
	}

	if err := unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &dimensions); err != nil {
		return err
	}

	return nil
}

func (s *server) Info(ctx context.Context, req *emptypb.Empty) (reply *cakeagent.InfoReply, err error) {
	memoryTotal := memory.TotalMemory()
	memoryFree := memory.FreeMemory()

	reply = &cakeagent.InfoReply{
		Ipaddresses: []string{},
		CpuCount:    int32(runtime.NumCPU()),
		Memory: &cakeagent.InfoReply_MemoryInfo{
			Total: memoryTotal,
			Free:  memoryFree,
			Used:  memoryTotal - memoryFree,
		},
	}

	// Retrieve system information
	if reply.Hostname, err = os.Hostname(); err != nil {
		return nil, err
	}

	// Get system uptime
	if host, err := sysinfo.Host(); err == nil {
		info := host.Info()

		reply.Osname = info.OS.Name
		reply.Release = info.OS.Codename
		reply.Version = info.OS.Version
		reply.Uptime = uint64(time.Now().Nanosecond() - info.BootTime.Nanosecond())
	}

	// Get IP addresses
	if interfaces, err := net.Interfaces(); err != nil {
		return nil, err
	} else {
		for _, iface := range interfaces {
			if addrs, err := iface.Addrs(); err == nil {
				for _, addr := range addrs {
					var ip net.IP
					switch v := addr.(type) {
					case *net.IPNet:
						ip = v.IP
					case *net.IPAddr:
						ip = v.IP
					}
					if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
						reply.Ipaddresses = append(reply.Ipaddresses, ip.String())
					}
				}
			}
		}
	}

	return reply, nil
}

func (s *server) execute(command *cakeagent.ExecuteCommand, stream cakeagent.Agent_ExecuteServer) (err error) {
	if ptx, pty, e := pty.Open(); e != nil {
		err = fmt.Errorf("failed to open pty: %v", e)
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		stderrInput, stderrOutput := io.Pipe()
		home, _ := os.UserHomeDir()
		var wg sync.WaitGroup
		var message cakeagent.ShellResponse

		wg.Add(3)

		doCancel := func() {
			if ctx.Err() == nil {
				cancel()
			}
		}

		fowardOutput := func(ctx context.Context, channel int, reader io.Reader) {
			input := bufio.NewReader(reader)
			buffer := make([]byte, 1024)

			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					if available, err := input.Read(buffer); err != nil {
						if err != io.EOF && err != io.ErrClosedPipe {
							glog.Errorf("Error reading output: %v", err)
						}
						doCancel()
					} else if available > 0 {
						var message *cakeagent.ShellResponse
						if channel == 0 {
							message = &cakeagent.ShellResponse{
								Response: &cakeagent.ShellResponse_Stdout{
									Stdout: buffer[:available],
								},
							}
						} else {
							message = &cakeagent.ShellResponse{
								Response: &cakeagent.ShellResponse_Stderr{
									Stderr: buffer[:available],
								},
							}
						}

						if err = stream.Send(message); err != nil {
							glog.Errorf("Failed to send output: %v", err)
							doCancel()
						}
					}
				}
			}
		}

		fowardStdin := func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					if request, err := stream.Recv(); err != nil {
						if err != io.EOF {
							glog.Errorf("Failed to receive message: %v", err)
						}
						doCancel()
					} else if msg := request.GetInput(); msg != nil {
						setTermSize(pty.Fd(), msg.Rows, msg.Cols)
						if len(msg.Datas) > 0 {
							if _, err := ptx.Write(msg.Datas); err != nil {
								glog.Errorf("Failed to write to stdin: %v", err)
								doCancel()
							}
						}
					}
				}
			}

		}

		cleanup := func() {
			ptx.Close()
			pty.Close()
			stderrInput.Close()
			stderrOutput.Close()

			wg.Wait()

			glog.Debug("Shell session ended")
		}

		setTermSize(pty.Fd(), command.Rows, command.Cols)

		cmd := exec.CommandContext(ctx, command.Command, command.Args...)
		cmd.Stdin = pty
		cmd.Stdout = pty
		cmd.Stderr = stderrOutput
		cmd.Env = os.Environ()
		cmd.Dir = home
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid:  true,
			Setctty: true,
		}

		go fowardOutput(ctx, 0, ptx)
		go fowardOutput(ctx, 1, stderrInput)
		go fowardStdin()

		defer cleanup()

		if err = cmd.Start(); err == nil {
			if err = cmd.Wait(); err != nil {
				if err.Error() != "context canceled" {
					glog.Errorf("Failed to wait for command: %v", err)
				}
			}

			doCancel()

			message = cakeagent.ShellResponse{
				Response: &cakeagent.ShellResponse_Stdout{
					Stdout: []byte("\r"),
				},
			}

			stream.Send(&message)
		} else {
			glog.Errorf("Failed to start command: %v", err)
		}

		if err != nil && err.Error() != "context canceled" {
			message = cakeagent.ShellResponse{
				Response: &cakeagent.ShellResponse_Stderr{
					Stderr: []byte(err.Error() + "\n"),
				},
			}

			stream.Send(&message)
		}

		message = cakeagent.ShellResponse{
			Response: &cakeagent.ShellResponse_ExitCode{
				ExitCode: int32(cmd.ProcessState.ExitCode()),
			},
		}

		stream.Send(&message)
	}

	return
}

func (s *server) Execute(stream cakeagent.Agent_ExecuteServer) (err error) {
	var request *cakeagent.ShellRequest
	var command *cakeagent.ExecuteCommand

	if request, err = stream.Recv(); err != nil {
		glog.Errorf("Failed to receive command message: %v", err)
		return
	}

	if command = request.GetCommand(); command == nil {
		err = fmt.Errorf("empty command")
		return
	}

	return s.execute(command, stream)
}

func (s *server) Shell(stream cakeagent.Agent_ExecuteServer) (err error) {
	command := &cakeagent.ExecuteCommand{
		Command: "bash",
		Args:    []string{"-i", "-l"},
		Rows:    25,
		Cols:    80,
	}

	if command.Command, err = exec.LookPath(command.Command); err != nil {
		command.Command = "/bin/sh"
	}

	return s.execute(command, stream)
}

func (s *server) Mount(ctx context.Context, request *cakeagent.MountRequest) (*cakeagent.MountReply, error) {
	return mount.Mount(ctx, request)
}
func (s *server) Umount(ctx context.Context, request *cakeagent.MountRequest) (*cakeagent.MountReply, error) {
	return mount.Umount(ctx, request)
}

func createListener(listen string) (listener net.Listener, err error) {
	var u *url.URL

	if u, err = url.Parse(listen); err != nil {
		err = fmt.Errorf("failed to parse address: %s", err)
	} else {
		var hostname = u.Hostname()

		if u.Scheme == "virtio" {
			if listener, err = serialport.Listen("/dev/virtio-ports/" + hostname); err != nil {
				err = fmt.Errorf("failed to listen on virtio: %v", err)
			}
		} else if u.Scheme == "vsock" {
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
		} else if u.Scheme == "tcp" {
			var port int

			if u.Port() == "" {
				port = 5000
			} else if port, err = strconv.Atoi(u.Port()); err != nil {
				err = fmt.Errorf("invalid port: %v", err)
			} else {
				listener, err = net.Listen("tcp", fmt.Sprintf("%s:%d", hostname, port))
			}
		} else if u.Scheme == "unix" {
			listener, err = net.Listen("unix", u.Path)
		} else {
			err = fmt.Errorf("unsupported address scheme: %s", u.Scheme)
		}
	}

	return
}

func StartServer(cfg *types.Config) (grpcServer *grpc.Server, err error) {
	var listener net.Listener

	if listener, err = createListener(cfg.Address); err != nil {
		err = fmt.Errorf("failed to parse address: %s", err)
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

			cakeagent.RegisterAgentServer(grpcServer, &server{})

			if err = grpcServer.Serve(listener); err != nil {
				err = fmt.Errorf("failed to serve: %s", err)
			}
		}
	}

	return
}
