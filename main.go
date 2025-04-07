//go:build windows
// +build windows

package main

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	WH_KEYBOARD_LL      = 13
	WM_KEYDOWN          = 0x0100
	WM_KEYUP            = 0x0101
	VK_K                = 0x4B
	VK_LWIN             = 0x5B
	VK_RWIN             = 0x5C
	VK_MEDIA_PLAY_PAUSE = 0xB3
	KEYEVENTF_EXT       = 0x0001
	KEYEVENTF_UP        = 0x0002
)

var (
	user32        = syscall.MustLoadDLL("user32.dll")
	setHook       = user32.MustFindProc("SetWindowsHookExW")
	nextHook      = user32.MustFindProc("CallNextHookEx")
	getMsg        = user32.MustFindProc("GetMessageW")
	keybd         = user32.MustFindProc("keybd_event")
	unhookWinHook = user32.MustFindProc("UnhookWindowsHookEx")

	kernel32 = syscall.MustLoadDLL("kernel32.dll")
	getmod   = kernel32.MustFindProc("GetModuleHandleW")

	winDown    bool
	hookHandle uintptr
)

type kbd struct {
	Code uint32
	Scan uint32
	Flag uint32
	Time uint32
	Info uintptr
}

// Callback function for keyboard hook
func proc(code int, wp, lp uintptr) uintptr {
	if code < 0 {
		r, _, _ := nextHook.Call(0, uintptr(code), wp, lp)
		return r
	}

	k := (*kbd)(unsafe.Pointer(lp))

	// Optimize check order for most common case first
	if wp == WM_KEYDOWN {
		if k.Code == VK_LWIN || k.Code == VK_RWIN {
			winDown = true
		} else if winDown && k.Code == VK_K {
			// Send media play/pause
			keybd.Call(VK_MEDIA_PLAY_PAUSE, 0, KEYEVENTF_EXT, 0)
			keybd.Call(VK_MEDIA_PLAY_PAUSE, 0, KEYEVENTF_EXT|KEYEVENTF_UP, 0)
			return 1 // Block default Win+K action
		}
	} else if wp == WM_KEYUP && (k.Code == VK_LWIN || k.Code == VK_RWIN) {
		winDown = false
	}

	r, _, _ := nextHook.Call(0, uintptr(code), wp, lp)
	return r
}

// Cleanup resources when exiting
func cleanup() {
	if hookHandle != 0 {
		unhookWinHook.Call(hookHandle)
		hookHandle = 0
	}

	// Release DLLs to prevent resource leaks
	user32.Release()
	kernel32.Release()
}

func main() {
	// Lock this thread to the OS thread since it's handling the Windows message loop
	runtime.LockOSThread()

	// Ensure system can handle our background load
	runtime.GOMAXPROCS(1) // This app doesn't need multiple cores

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cleanup()
		os.Exit(0)
	}()

	defer cleanup() // Ensure cleanup on exit

	// Install keyboard hook
	mod, _, _ := getmod.Call(0)
	handle, _, _ := setHook.Call(WH_KEYBOARD_LL, syscall.NewCallback(proc), mod, 0)
	if handle == 0 {
		os.Exit(1) // Failed to set hook
	}
	hookHandle = handle

	// Message loop - keep it minimal for startup use
	var m struct{ Hwnd, Msg, Wp, Lp, Time, Pt uintptr }
	for {
		// Minimize CPU usage in message loop
		ret, _, _ := getMsg.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 {
			// WM_QUIT received
			break
		}
	}
}
