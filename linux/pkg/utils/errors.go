package utils

import (
	"context"
	"io"
	"os"
	"os/exec"
	"syscall"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func IsGRPCCancel(err error) bool {
	if err == nil {
		return false
	}

	if status, ok := status.FromError(err); ok {
		return status.Code() == codes.Canceled
	}

	return false
}

func IsEINTR(err error) bool {
	if err == nil {
		return false
	}

	if opErr, ok := err.(*os.PathError); ok {
		if syscallErr, ok := opErr.Err.(syscall.Errno); ok {
			if syscallErr == syscall.EINTR {
				return true
			}
		}
	}

	if syscallErr, ok := err.(syscall.Errno); ok {
		if syscallErr == syscall.EINTR {
			return true
		}
	}

	if opErr, ok := err.(*os.SyscallError); ok {
		if syscallErr, ok := opErr.Err.(syscall.Errno); ok {
			if syscallErr == syscall.EINTR {
				return true
			}
		}
	}

	if opErr, ok := err.(*os.LinkError); ok {
		if syscallErr, ok := opErr.Err.(syscall.Errno); ok {
			if syscallErr == syscall.EINTR {
				return true
			}
		}
	}

	return false
}

func IsUnexpectedError(err error) bool {

	if err == nil {
		return false
	}

	if _, ok := err.(*exec.ExitError); ok {
		return false
	}

	if err == io.EOF || err == io.ErrClosedPipe || err == context.Canceled {
		return false
	}

	return true
}

func IsEAGAIN(err error) bool {
	if err == nil {
		return false
	}

	if opErr, ok := err.(*os.PathError); ok {
		if syscallErr, ok := opErr.Err.(syscall.Errno); ok {
			if syscallErr == syscall.EAGAIN {
				return true
			}
		}
	}

	if syscallErr, ok := err.(syscall.Errno); ok {
		if syscallErr == syscall.EAGAIN {
			return true
		}
	}

	if opErr, ok := err.(*os.SyscallError); ok {
		if syscallErr, ok := opErr.Err.(syscall.Errno); ok {
			if syscallErr == syscall.EAGAIN {
				return true
			}
		}
	}

	if opErr, ok := err.(*os.LinkError); ok {
		if syscallErr, ok := opErr.Err.(syscall.Errno); ok {
			if syscallErr == syscall.EAGAIN {
				return true
			}
		}
	}

	return false
}
