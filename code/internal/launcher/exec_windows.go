//go:build windows

package launcher

import "syscall"

func sysProcAttr() *syscall.SysProcAttr {
	// CREATE_NO_WINDOW = 0x08000000
	return &syscall.SysProcAttr{HideWindow: true}
}
