package pkg

import (
	"bufio"
	"bytes"
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
	"strings"
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

type pipe struct {
	input  *io.PipeReader
	output *io.PipeWriter
}

func (p *pipe) Close() {
	p.input.Close()
	p.output.Close()
}

type pseudoTTY struct {
	pty    *os.File
	ptx    *os.File
	stdin  *pipe
	stdout *pipe
}

func newPipe() (p *pipe) {
	p = &pipe{}
	p.input, p.output = io.Pipe()
	return
}

func newTTY(termSize *cakeagent.TerminalSize) (tty *pseudoTTY, err error) {
	tty = &pseudoTTY{}

	if termSize.Cols > 0 && termSize.Rows > 0 {
		if glog.GetLevel() >= glog.TraceLevel {
			glog.Tracef("Opening pty with size %dx%d", termSize.Rows, termSize.Cols)
		}

		if tty.ptx, tty.pty, err = pty.Open(); err == nil {
			tty.SetTermSize(termSize.Rows, termSize.Cols)
		}
	} else {
		if glog.GetLevel() >= glog.TraceLevel {
			glog.Trace("Opening pty without size")
		}

		tty.stdin = newPipe()
		tty.stdout = newPipe()
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

func (t *pseudoTTY) Stdin() io.Reader {
	if t.pty != nil {
		return t.pty
	}

	return t.stdin.input
}

func (t *pseudoTTY) Stdout() io.Writer {
	if t.pty != nil {
		return t.pty
	}

	return t.stdout.output
}

func (t *pseudoTTY) Input() io.Reader {
	if t.ptx != nil {
		return t.ptx
	}

	return t.stdout.input
}

func (t *pseudoTTY) SetTermSize(rows int32, cols int32) (err error) {
	if t.pty != nil {
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

func (t *pseudoTTY) Write(data []byte) (n int, err error) {
	if t.ptx != nil {
		n, err = t.ptx.Write(data)
	} else if t.stdin != nil {
		n, err = t.stdin.output.Write(data)
	}
	return
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

func (s *server) Run(ctx context.Context, req *cakeagent.RunCommand) (reply *cakeagent.ExecuteReply, err error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	home, _ := os.UserHomeDir()
	reply = &cakeagent.ExecuteReply{}

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
				return nil, fmt.Errorf("failed to get exit status: %v", err)
			}
		} else {
			return nil, fmt.Errorf("failed to run command: %v", err)
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

func (s *server) execute(command *cakeagent.ExecuteCommand, termSize *cakeagent.TerminalSize, stream cakeagent.Agent_ExecuteServer) (err error) {
	if tty, e := newTTY(termSize); e != nil {
		err = fmt.Errorf("failed to open pty: %v", e)
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		stderr := newPipe()
		home, _ := os.UserHomeDir()
		var wg sync.WaitGroup
		var message cakeagent.ExecuteResponse
		var cmd *exec.Cmd

		doCancel := func() {
			if ctx.Err() == nil {
				if glog.GetLevel() >= glog.TraceLevel {
					glog.Trace("Canceling context")
				}
				cancel()
			}
			tty.Close()
			stderr.Close()
		}

		fowardOutput := func(ctx context.Context, channel int, reader io.Reader) {
			input := bufio.NewReader(reader)
			buffer := make([]byte, 1024)

			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					if glog.GetLevel() >= glog.TraceLevel {
						glog.Tracef("Closing output %d", channel)
					}
					return
				default:
					if available, err := input.Read(buffer); err != nil {
						if err != io.EOF && err != io.ErrClosedPipe {
							glog.Errorf("Error reading output: %v", err)
							doCancel()
						}

						if glog.GetLevel() >= glog.TraceLevel {
							glog.Tracef("EOF reading output %d", channel)
						}
						return
					} else if available > 0 {
						if glog.GetLevel() >= glog.TraceLevel {
							glog.Tracef("reading output: %d", len(buffer[:available]))
						}

						var message *cakeagent.ExecuteResponse
						if channel == 0 {
							message = &cakeagent.ExecuteResponse{
								Response: &cakeagent.ExecuteResponse_Stdout{
									Stdout: buffer[:available],
								},
							}
						} else {
							message = &cakeagent.ExecuteResponse{
								Response: &cakeagent.ExecuteResponse_Stderr{
									Stderr: buffer[:available],
								},
							}
						}

						if err = stream.Send(message); err != nil {
							glog.Errorf("Failed to send output: %v", err)
							doCancel()
						}

						if glog.GetLevel() >= glog.TraceLevel {
							glog.Tracef("Sent output: %d", len(buffer[:available]))
						}
					} else if glog.GetLevel() >= glog.TraceLevel {
						glog.Trace("No data to read")
					}
				}
			}
		}

		fowardStdin := func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					if glog.GetLevel() >= glog.TraceLevel {
						glog.Trace("Closing stdin")
					}
					return
				default:
					if request, err := stream.Recv(); err != nil {
						if err != io.EOF && err != context.Canceled {
							glog.Errorf("Failed to receive message: %v", err)
							doCancel()
						}
					} else {
						if size := request.GetSize(); size != nil {
							tty.SetTermSize(size.Rows, size.Cols)
						} else if input := request.GetInput(); input != nil {
							if len(input) > 0 {
								if _, err := tty.Write(input); err != nil {
									glog.Errorf("Failed to write to stdin: %v", err)
									doCancel()
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
		}

		cleanup := func() {
			doCancel()

			wg.Wait()

			if glog.GetLevel() >= glog.TraceLevel {
				glog.Trace("Shell session ended")
			}
		}

		if command.GetShell() {
			shell := "bash"

			if shell, err = exec.LookPath(shell); err != nil {
				shell = "/bin/sh"
			}
			cmd = exec.CommandContext(ctx, shell, "-i", "-l")
		} else if exc := command.GetCommand(); exc != nil {
			cmd = exec.CommandContext(ctx, exc.GetCommand(), exc.GetArgs()...)
		} else {
			err = fmt.Errorf("wrong protocol")
		}

		if err == nil {
			isTTY := tty.IsTTY()

			cmd.Stdin = tty.Stdin()
			cmd.Stdout = tty.Stdout()
			cmd.Stderr = stderr.output
			cmd.Env = os.Environ()
			cmd.Dir = home
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Setsid:  isTTY,
				Setctty: isTTY,
				//Noctty:  !isTTY,
			}

			wg.Add(3)

			go fowardOutput(ctx, 0, tty.Input())
			go fowardOutput(ctx, 1, stderr.input)
			go fowardStdin()

			defer cleanup()

			if err = cmd.Start(); err == nil {
				if err = cmd.Wait(); err != nil {
					if err != io.EOF && err != io.ErrClosedPipe && err != context.Canceled {
						glog.Errorf("Failed to wait for command: %v", err)
					}
				} else if glog.GetLevel() >= glog.TraceLevel {
					glog.Trace("Command exited")
				}
			} else {
				glog.Errorf("Failed to start command: %v", err)
			}

			if command.GetShell() {
				message = cakeagent.ExecuteResponse{
					Response: &cakeagent.ExecuteResponse_Stderr{
						Stderr: []byte("\r"),
					},
				}

				stream.Send(&message)
			}
		}

		if err != nil && err != io.EOF && err != io.ErrClosedPipe && err != context.Canceled {
			message = cakeagent.ExecuteResponse{
				Response: &cakeagent.ExecuteResponse_Stderr{
					Stderr: []byte(err.Error() + "\r\n"),
				},
			}

			stream.Send(&message)
		}

		if cmd != nil {
			message = cakeagent.ExecuteResponse{
				Response: &cakeagent.ExecuteResponse_ExitCode{
					ExitCode: int32(cmd.ProcessState.ExitCode()),
				},
			}
		} else {
			message = cakeagent.ExecuteResponse{
				Response: &cakeagent.ExecuteResponse_ExitCode{
					ExitCode: 1,
				},
			}
		}

		if err = stream.Send(&message); err != nil {
			glog.Errorf("Failed to send exit code: %v", err)
		}
	}

	glog.Debug("Leave execute")

	return
}

func (s *server) Execute(stream cakeagent.Agent_ExecuteServer) (err error) {
	var request *cakeagent.ExecuteRequest
	var command *cakeagent.ExecuteCommand
	var termSize *cakeagent.TerminalSize

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
