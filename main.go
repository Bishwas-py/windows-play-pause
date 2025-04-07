//go:build windows
// +build windows

package main

import (
	"os"
	"os/signal"
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
	user32   = syscall.MustLoadDLL("user32.dll")
	setHook  = user32.MustFindProc("SetWindowsHookExW")
	nextHook = user32.MustFindProc("CallNextHookEx")
	getMsg   = user32.MustFindProc("GetMessageW")
	keybd    = user32.MustFindProc("keybd_event")
	getmod   = syscall.MustLoadDLL("kernel32.dll").MustFindProc("GetModuleHandleW")
	winDown  bool
)

type kbd struct {
	Code uint32
	Scan uint32
	Flag uint32
	Time uint32
	Info uintptr
}

func proc(code int, wp, lp uintptr) uintptr {
	if code < 0 {
		r, _, _ := nextHook.Call(0, uintptr(code), wp, lp)
		return r
	}

	k := (*kbd)(unsafe.Pointer(lp))

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

func main() {
	// Set up clean termination
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() { <-c; os.Exit(0) }()

	// Install keyboard hook
	mod, _, _ := getmod.Call(0)
	setHook.Call(WH_KEYBOARD_LL, syscall.NewCallback(proc), mod, 0)

	// Message loop
	var m struct{ Hwnd, Msg, Wp, Lp, Time, Pt uintptr }
	for {
		getMsg.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
	}
}
