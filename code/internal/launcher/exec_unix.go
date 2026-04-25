//go:build !windows

package launcher

import "syscall"

func sysProcAttr() *syscall.SysProcAttr {
	return nil
}
