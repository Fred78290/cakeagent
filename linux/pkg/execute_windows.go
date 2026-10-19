//go:build windows
// +build windows

package pkg

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
	"unsafe"

	"github.com/Fred78290/cakeagent/pkg/cakeagent"
	"github.com/Fred78290/cakeagent/pkg/utils"
	glog "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

// How long the output readers get to drain once the process has exited. A grandchild that inherited the output
// handle keeps the pipe open past the process' exit, so the readers are closed after this instead of waiting forever.
const outputDrainTimeout = 2 * time.Second

// session is a running command, either attached to a pseudo console (ConPTY, when the client sent a terminal size)
// or to plain pipes.
type session interface {
	// Stdout returns the process' output. Under a pseudo console it carries stderr too.
	Stdout() io.Reader
	// Stderr returns the process' error output, or nil under a pseudo console.
	Stderr() io.Reader
	Write(data []byte) (int, error)
	Resize(rows, cols int32) error
	CloseStdin()
	// Wait blocks until the process exits and returns its exit code.
	Wait() (int, error)
	Kill()
	// CloseOutput is called once the process has exited: the output readers then reach io.EOF after draining what
	// the process wrote.
	CloseOutput()
	// Close releases everything the session still holds.
	Close()
}

// shellCommand returns the interactive shell: PowerShell 7 when installed, Windows PowerShell otherwise, and cmd.exe
// as a last resort. Without a terminal PowerShell is told to read its commands from stdin.
func shellCommand(tty bool) (string, []string) {
	for _, name := range []string{"pwsh.exe", "powershell.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			if tty {
				return path, []string{"-NoLogo"}
			}

			return path, []string{"-NoLogo", "-NoProfile", "-Command", "-"}
		}
	}

	if comspec := os.Getenv("COMSPEC"); comspec != "" {
		return comspec, nil
	}

	return "cmd.exe", nil
}

func resolveCommand(command *cakeagent.CakeAgent_ExecuteRequest_ExecuteCommand, tty bool) (string, []string, error) {
	if command.GetShell() {
		path, args := shellCommand(tty)

		return path, args, nil
	} else if exc := command.GetCommand(); exc != nil {
		path, err := exec.LookPath(exc.GetCommand())

		if err != nil {
			return "", nil, err
		}

		return path, exc.GetArgs(), nil
	}

	return "", nil, fmt.Errorf("wrong protocol")
}

// pipeSession runs the command with redirected stdin, stdout and stderr.
type pipeSession struct {
	cmd     *exec.Cmd
	stdin   *os.File
	stdout  *os.File
	stderr  *os.File
	closing sync.Once
}

func newPipeSession(path string, args []string, dir string) (s *pipeSession, err error) {
	var stdinReader, stdoutWriter, stderrWriter *os.File

	s = &pipeSession{
		cmd: exec.Command(path, args...),
	}

	defer func() {
		// The child has its own copies of these ends: the parent's must go, or the output never reaches EOF.
		for _, f := range []*os.File{stdinReader, stdoutWriter, stderrWriter} {
			if f != nil {
				f.Close()
			}
		}

		if err != nil {
			s.Close()
			s = nil
		}
	}()

	if stdinReader, s.stdin, err = os.Pipe(); err != nil {
		return
	}

	if s.stdout, stdoutWriter, err = os.Pipe(); err != nil {
		return
	}

	if s.stderr, stderrWriter, err = os.Pipe(); err != nil {
		return
	}

	s.cmd.Stdin = stdinReader
	s.cmd.Stdout = stdoutWriter
	s.cmd.Stderr = stderrWriter
	s.cmd.Dir = dir
	s.cmd.Env = os.Environ()

	err = s.cmd.Start()

	return
}

func (s *pipeSession) Stdout() io.Reader {
	return s.stdout
}

func (s *pipeSession) Stderr() io.Reader {
	return s.stderr
}

func (s *pipeSession) Write(data []byte) (int, error) {
	if s.stdin == nil {
		return 0, io.ErrClosedPipe
	}

	return s.stdin.Write(data)
}

func (s *pipeSession) Resize(rows, cols int32) error {
	return nil
}

func (s *pipeSession) CloseStdin() {
	if s.stdin != nil {
		s.stdin.Close()
	}
}

func (s *pipeSession) Wait() (int, error) {
	if err := s.cmd.Wait(); err != nil {
		var exitErr *exec.ExitError

		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}

		return 1, err
	}

	return s.cmd.ProcessState.ExitCode(), nil
}

func (s *pipeSession) Kill() {
	if s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
}

// CloseOutput has nothing to do: the write ends belong to the process alone, so its exit closes them.
func (s *pipeSession) CloseOutput() {
}

func (s *pipeSession) Close() {
	s.CloseStdin()

	s.closing.Do(func() {
		for _, f := range []*os.File{s.stdout, s.stderr} {
			if f != nil {
				f.Close()
			}
		}
	})
}

// ptySession runs the command attached to a pseudo console, so interactive programs (PowerShell's line editor,
// cmd.exe, full screen tools) behave as they would in a terminal window. The pseudo console speaks VT sequences on
// both ends, which is what the client's terminal expects.
type ptySession struct {
	lock         sync.Mutex
	console      windows.Handle
	process      windows.Handle
	stdinWriter  *os.File
	stdoutReader *os.File
	closeConsole sync.Once
	closeOutput  sync.Once
}

func newPtySession(path string, args []string, dir string, rows, cols int32) (s *ptySession, err error) {
	var inputReader, inputWriter, outputReader, outputWriter windows.Handle

	if err = windows.CreatePipe(&inputReader, &inputWriter, nil, 0); err != nil {
		return nil, fmt.Errorf("failed to create input pipe: %w", err)
	}

	if err = windows.CreatePipe(&outputReader, &outputWriter, nil, 0); err != nil {
		windows.CloseHandle(inputReader)
		windows.CloseHandle(inputWriter)

		return nil, fmt.Errorf("failed to create output pipe: %w", err)
	}

	s = &ptySession{
		stdinWriter:  os.NewFile(uintptr(inputWriter), "conpty-stdin"),
		stdoutReader: os.NewFile(uintptr(outputReader), "conpty-stdout"),
	}

	defer func() {
		// The pseudo console duplicated the ends it uses.
		windows.CloseHandle(inputReader)
		windows.CloseHandle(outputWriter)

		if err != nil {
			s.Close()
			s = nil
		}
	}()

	if err = windows.CreatePseudoConsole(consoleSize(rows, cols), inputReader, outputWriter, 0, &s.console); err != nil {
		s.console = 0

		return s, fmt.Errorf("failed to create pseudo console: %w", err)
	}

	var attributes *windows.ProcThreadAttributeListContainer

	if attributes, err = windows.NewProcThreadAttributeList(1); err != nil {
		return s, fmt.Errorf("failed to create attribute list: %w", err)
	}

	defer attributes.Delete()

	if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&s.console)), unsafe.Sizeof(s.console)); err != nil {
		return s, fmt.Errorf("failed to attach pseudo console: %w", err)
	}

	var commandLine, directory *uint16
	var processInfo windows.ProcessInformation

	if commandLine, err = windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{path}, args...))); err != nil {
		return
	}

	if dir != "" {
		if directory, err = windows.UTF16PtrFromString(dir); err != nil {
			return
		}
	}

	startupInfo := &windows.StartupInfoEx{
		ProcThreadAttributeList: attributes.List(),
	}

	startupInfo.Cb = uint32(unsafe.Sizeof(*startupInfo))
	// Null std handles: otherwise a parent whose own handles are redirected (a service, a debugger) hands them to the
	// child, which then writes there instead of to the pseudo console.
	startupInfo.Flags = windows.STARTF_USESTDHANDLES

	if err = windows.CreateProcess(nil, commandLine, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT, nil, directory, &startupInfo.StartupInfo, &processInfo); err != nil {
		return s, fmt.Errorf("failed to start %s: %w", path, err)
	}

	windows.CloseHandle(processInfo.Thread)
	s.process = processInfo.Process

	return s, nil
}

func consoleSize(rows, cols int32) windows.Coord {
	return windows.Coord{X: int16(cols), Y: int16(rows)}
}

func (s *ptySession) Stdout() io.Reader {
	return s.stdoutReader
}

func (s *ptySession) Stderr() io.Reader {
	return nil
}

func (s *ptySession) Write(data []byte) (int, error) {
	if s.stdinWriter == nil {
		return 0, io.ErrClosedPipe
	}

	return s.stdinWriter.Write(data)
}

func (s *ptySession) Resize(rows, cols int32) error {
	if s.console == 0 || rows <= 0 || cols <= 0 {
		return nil
	}

	return windows.ResizePseudoConsole(s.console, consoleSize(rows, cols))
}

func (s *ptySession) CloseStdin() {
	if s.stdinWriter != nil {
		// A console program sees end of input as Ctrl-Z, not as a closed pipe.
		s.stdinWriter.Write([]byte{0x1a, '\r'})
	}
}

func (s *ptySession) Wait() (int, error) {
	var exitCode uint32

	if _, err := windows.WaitForSingleObject(s.process, windows.INFINITE); err != nil {
		return 1, err
	}

	if err := windows.GetExitCodeProcess(s.process, &exitCode); err != nil {
		return 1, err
	}

	return int(exitCode), nil
}

func (s *ptySession) Kill() {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.process != 0 {
		windows.TerminateProcess(s.process, 1)
	}
}

// CloseOutput closes the pseudo console, which flushes what is left and closes its end of the output pipe. On older
// builds of Windows ClosePseudoConsole blocks until the output has been read, so it runs while the reader still runs.
func (s *ptySession) CloseOutput() {
	s.closeConsole.Do(func() {
		if s.console != 0 {
			windows.ClosePseudoConsole(s.console)
		}
	})
}

func (s *ptySession) Close() {
	s.CloseOutput()

	s.closeOutput.Do(func() {
		if s.stdinWriter != nil {
			s.stdinWriter.Close()
		}

		if s.stdoutReader != nil {
			s.stdoutReader.Close()
		}

		s.lock.Lock()
		defer s.lock.Unlock()

		if s.process != 0 {
			windows.CloseHandle(s.process)
			s.process = 0
		}
	})
}

func (s *server) execute(command *cakeagent.CakeAgent_ExecuteRequest_ExecuteCommand, termSize *cakeagent.CakeAgent_ExecuteRequest_TerminalSize, stream cakeagent.CakeAgentService_ExecuteServer) (err error) {
	var sess session
	var sendLock sync.Mutex

	send := func(message *cakeagent.CakeAgent_ExecuteResponse) error {
		sendLock.Lock()
		defer sendLock.Unlock()

		return stream.Send(message)
	}

	tty := termSize.GetRows() > 0 && termSize.GetCols() > 0
	home, _ := os.UserHomeDir()

	if path, args, e := resolveCommand(command, tty); e != nil {
		err = e
	} else if tty {
		sess, err = newPtySession(path, args, home, termSize.GetRows(), termSize.GetCols())
	} else {
		sess, err = newPipeSession(path, args, home)
	}

	if err != nil {
		glog.Errorf("Failed to start command: %v", err)

		return send(&cakeagent.CakeAgent_ExecuteResponse{
			Response: &cakeagent.CakeAgent_ExecuteResponse_Established{
				Established: &cakeagent.CakeAgent_ExecuteResponse_EstablishedResponse{
					Success: false,
					Reason:  err.Error(),
				},
			},
		})
	}

	defer sess.Close()

	if err = send(&cakeagent.CakeAgent_ExecuteResponse{
		Response: &cakeagent.CakeAgent_ExecuteResponse_Established{
			Established: &cakeagent.CakeAgent_ExecuteResponse_EstablishedResponse{
				Success: true,
				Reason:  "Success",
			},
		},
	}); err != nil {
		glog.Errorf("Failed to send established message: %v", err)
		sess.Kill()
		sess.Wait()

		return err
	}

	var readers sync.WaitGroup

	forward := func(name string, reader io.Reader, wrap func([]byte) *cakeagent.CakeAgent_ExecuteResponse) {
		defer readers.Done()

		buffer := make([]byte, 4096)

		for {
			available, readErr := reader.Read(buffer)

			if available > 0 {
				data := make([]byte, available)
				copy(data, buffer[:available])

				if sendErr := send(wrap(data)); sendErr != nil {
					glog.Errorf("Failed to send %s: %v", name, sendErr)
					sess.Kill()

					return
				}
			}

			if readErr != nil {
				if readErr != io.EOF && !errors.Is(readErr, os.ErrClosed) && !errors.Is(readErr, windows.ERROR_BROKEN_PIPE) {
					glog.Debugf("Reading %s ended: %v", name, readErr)
				}

				return
			}
		}
	}

	readers.Add(1)
	go forward("stdout", sess.Stdout(), func(data []byte) *cakeagent.CakeAgent_ExecuteResponse {
		return &cakeagent.CakeAgent_ExecuteResponse{Response: &cakeagent.CakeAgent_ExecuteResponse_Stdout{Stdout: data}}
	})

	if stderr := sess.Stderr(); stderr != nil {
		readers.Add(1)
		go forward("stderr", stderr, func(data []byte) *cakeagent.CakeAgent_ExecuteResponse {
			return &cakeagent.CakeAgent_ExecuteResponse{Response: &cakeagent.CakeAgent_ExecuteResponse_Stderr{Stderr: data}}
		})
	}

	// Input from the client. Recv returns once the handler has returned, so this goroutine never outlives the stream.
	go func() {
		for {
			request, recvErr := stream.Recv()

			if recvErr != nil {
				if recvErr != io.EOF && !utils.IsGRPCCancel(recvErr) {
					glog.Errorf("Failed to receive message: %v", recvErr)
				}

				// The client is gone: nobody will read the output nor send more input.
				if recvErr != io.EOF {
					sess.Kill()
				}

				return
			}

			if size := request.GetSize(); size != nil {
				if resizeErr := sess.Resize(size.Rows, size.Cols); resizeErr != nil {
					glog.Warnf("Failed to resize pseudo console: %v", resizeErr)
				}
			} else if input := request.GetInput(); input != nil {
				if _, writeErr := sess.Write(input); writeErr != nil {
					glog.Errorf("Failed to write to stdin: %v", writeErr)
				}
			} else if request.GetEof() {
				sess.CloseStdin()
			} else {
				glog.Error("Unknown message")
			}
		}
	}()

	exitCode, waitErr := sess.Wait()

	if waitErr != nil {
		glog.Errorf("Failed to wait for command: %v", waitErr)
	}

	sess.CloseOutput()

	drained := make(chan struct{})

	go func() {
		readers.Wait()
		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(outputDrainTimeout):
		// A reader blocked on a pipe that a grandchild still holds open cannot be interrupted; it is left behind
		// and ends with that grandchild.
		glog.Debug("Output still open after the process exited, closing it")
		sess.Close()
	}

	if command.GetShell() && tty {
		send(&cakeagent.CakeAgent_ExecuteResponse{
			Response: &cakeagent.CakeAgent_ExecuteResponse_Stderr{
				Stderr: []byte("\r"),
			},
		})
	}

	if err = send(&cakeagent.CakeAgent_ExecuteResponse{
		Response: &cakeagent.CakeAgent_ExecuteResponse_ExitCode{
			ExitCode: int32(exitCode),
		},
	}); err != nil {
		glog.Errorf("Failed to send exit code: %v", err)
	}

	glog.Debug("Leave execute")

	return err
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
