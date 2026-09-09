package converter

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	caxaUser32      = syscall.NewLazyDLL("user32.dll")
	caxaEnumWindows = caxaUser32.NewProc("EnumWindows")
	caxaWindowText  = caxaUser32.NewProc("GetWindowTextW")
	caxaWindowClass = caxaUser32.NewProc("GetClassNameW")
	caxaWindowPID   = caxaUser32.NewProc("GetWindowThreadProcessId")
	caxaVisible     = caxaUser32.NewProc("IsWindowVisible")
	caxaEnabled     = caxaUser32.NewProc("IsWindowEnabled")
	caxaDlgItem     = caxaUser32.NewProc("GetDlgItem")
	caxaPostMessage = caxaUser32.NewProc("PostMessageW")
	caxaProcessName = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")
)

func caxaWindowString(proc *syscall.LazyProc, hwnd uintptr) string {
	var buf [512]uint16
	proc.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:])
}

// Run outside CAXA: opening the startup document can block before its plugin is ready.
func watchCaxaFontDialogs(ctx context.Context) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				caxaEnumWindows.Call(caxaFontDialogCallback, 0)
			}
		}
	}()
	return func() { cancel(); <-done }
}

var caxaFontDialogCallback = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
	visible, _, _ := caxaVisible.Call(hwnd)
	if visible == 0 || caxaWindowString(caxaWindowClass, hwnd) != "#32770" ||
		caxaWindowString(caxaWindowText, hwnd) != "\u6307\u5b9a\u5f62\u6587\u4ef6" {
		return 1
	}
	var pid uint32
	caxaWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	process, err := syscall.OpenProcess(0x1000, false, pid)
	if err != nil {
		return 1
	}
	defer syscall.CloseHandle(process)
	var path [32768]uint16
	size := uint32(len(path))
	ok, _, _ := caxaProcessName.Call(uintptr(process), 0, uintptr(unsafe.Pointer(&path[0])), uintptr(unsafe.Pointer(&size)))
	if ok == 0 || !strings.EqualFold(filepath.Base(syscall.UTF16ToString(path[:])), "CDRAFT_M.exe") {
		return 1
	}
	button, _, _ := caxaDlgItem.Call(hwnd, 2654)
	if button == 0 || caxaWindowString(caxaWindowText, button) != "\u5168\u90e8\u53d6\u6d88" {
		return 1
	}
	enabled, _, _ := caxaEnabled.Call(button)
	if enabled != 0 {
		// WM_COMMAND/BN_CLICKED does not require the dialog to be active, unlike BM_CLICK.
		sent, _, err := caxaPostMessage.Call(hwnd, 0x0111, 2654, button)
		if sent == 0 {
			log.Printf("[CAD Converter] font dialog Cancel All failed: pid=%d error=%v", pid, err)
		} else {
			log.Printf("[CAD Converter] font dialog Cancel All posted: pid=%d hwnd=%d", pid, hwnd)
		}
	}
	return 1
})
