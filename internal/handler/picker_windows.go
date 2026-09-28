//go:build windows

package handler

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 纯 Go 原生文件夹选择框（无 cgo、无外挂进程）：
// 进程内直接调 COM 的 IFileDialog（CLSID_FileOpenDialog，Win10/11 的现代文件夹对话框）。
//
// 前台问题：宿主是 -H windowsgui 的无窗口进程，用户最后点击的是浏览器，
// 我们没有任何前台权限，弹出的对话框会沉在浏览器后面。解法是等对话框窗口一出现，
// 由监控协程直接对它动手：先 SetWindowPos(HWND_TOPMOST) 兜底保证可见，
// 再 AttachThreadInput 借用前台线程的输入队列拿到 SetForegroundWindow 的许可，
// 最后 SetForegroundWindow + SetFocus 抢焦点。给 ShowDialog 传 TopMost owner
// 之所以无效，是因为 #32770 通用对话框不继承 owner 的 topmost 样式；
// 直接操作对话框窗口本身没有这个问题。

const (
	clsctxInprocServer      = 0x1
	coinitApartmentThreaded = 0x2
	coinitDisableOle1Dde    = 0x4

	fosPickFolders     = 0x20
	fosForceFilesystem = 0x40

	sigdnFileSysPath = 0x80058000

	hresultCancelled = 0x800704C7 // HRESULT_FROM_WIN32(ERROR_CANCELLED)
	hrRPCChangedMode = 0x80010106 // RPC_E_CHANGED_MODE

	swpNosize    = 0x1
	swpNomove    = 0x2
	swpShowWindow = 0x40

	hwndTopmost   = ^uintptr(0) // (HWND)-1
)

// CLSID_FileOpenDialog / IID_IFileDialog / IID_IShellItem（shobjidl_core.h）
var (
	clsidFileOpenDialog = windows.GUID{
		Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE,
		Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7},
	}
	iidIFileDialog = windows.GUID{
		Data1: 0x42F85136, Data2: 0xDB7E, Data3: 0x439C,
		Data4: [8]byte{0x85, 0xF1, 0xE4, 0x07, 0x5D, 0x13, 0x5F, 0xC8},
	}
	iidIShellItem = windows.GUID{
		Data1: 0x43826D1E, Data2: 0xE718, Data3: 0x42EE,
		Data4: [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE},
	}
)

var (
	ole32                 = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx    = ole32.NewProc("CoInitializeEx")
	procCoUninitialize    = ole32.NewProc("CoUninitialize")
	procCoCreateInstance  = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree     = ole32.NewProc("CoTaskMemFree")

	user32                    = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow   = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcId = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput     = user32.NewProc("AttachThreadInput")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procSetActiveWindow       = user32.NewProc("SetActiveWindow")
	procSetFocus              = user32.NewProc("SetFocus")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procBringWindowToTop      = user32.NewProc("BringWindowToTop")
	procEnumThreadWindows     = user32.NewProc("EnumThreadWindows")
	procGetClassNameW         = user32.NewProc("GetClassNameW")
	procIsWindowVisible       = user32.NewProc("IsWindowVisible")
)

// fileDialogVtbl 是 IFileDialog 的 COM vtable，字段顺序 = IDL 声明顺序，一个都不能动。
// （IUnknown 3 个 + IModalWindow::Show 1 个 + IFileDialog 23 个，共 27 个槽位。）
type fileDialogVtbl struct {
	// IUnknown
	queryInterface, addRef, release uintptr
	// IModalWindow
	show uintptr
	// IFileDialog
	setFileTypes, setFileTypeIndex, getFileTypeIndex, advise, unadvise uintptr
	setOptions, getOptions                                             uintptr
	setDefaultFolder, setFolder, getFolder, getCurrentSelection        uintptr
	setFileName, getFileName, setTitle, setOkButtonLabel, setFileNameLabel uintptr
	getResult, addPlace, setDefaultExtension, close                    uintptr
	setClientGuid, clearClientData, setFilter                          uintptr
}

type fileDialog struct {
	vtbl *fileDialogVtbl
}

// shellItemVtbl：IUnknown 3 个 + IShellItem 5 个。只用 getDisplayName。
type shellItemVtbl struct {
	queryInterface, addRef, release                          uintptr
	bindToHandler, getParent, getDisplayName, getAttributes, compare uintptr
}

type shellItem struct {
	vtbl *shellItemVtbl
}

// pickFolderCmd 弹原生文件夹选择框，返回选中的绝对路径；取消返回空串、nil error。
// 整个调用锁死 OS 线程：IFileDialog 要求创建与 Show 在同一个 STA 线程上，
// Go 协程会在线程间漂移，必须 LockOSThread。请求会阻塞到用户选择/取消（设计行为）。
func pickFolderCmd() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded|coinitDisableOle1Dde)
	switch hr {
	case 0, 1: // S_OK / S_FALSE
		defer procCoUninitialize.Call()
	case hrRPCChangedMode:
		return "", fmt.Errorf("当前线程 COM 套间模式冲突，无法弹框")
	default:
		return "", fmt.Errorf("CoInitializeEx 失败: 0x%08X", hr)
	}

	var raw unsafe.Pointer
	hr, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIFileDialog)),
		uintptr(unsafe.Pointer(&raw)),
	)
	if hr != 0 {
		return "", fmt.Errorf("创建文件对话框失败: 0x%08X", hr)
	}
	dlg := (*fileDialog)(raw)
	self := uintptr(unsafe.Pointer(dlg))
	vt := dlg.vtbl
	defer syscall.SyscallN(vt.release, self)

	var opts uint32
	if hr, _, _ = syscall.SyscallN(vt.getOptions, self, uintptr(unsafe.Pointer(&opts))); hr != 0 {
		return "", fmt.Errorf("GetOptions 失败: 0x%08X", hr)
	}
	if hr, _, _ = syscall.SyscallN(vt.setOptions, self, uintptr(opts|fosPickFolders|fosForceFilesystem)); hr != 0 {
		return "", fmt.Errorf("SetOptions 失败: 0x%08X", hr)
	}
	title, _ := windows.UTF16PtrFromString("选择工作目录")
	_, _, _ = syscall.SyscallN(vt.setTitle, self, uintptr(unsafe.Pointer(title))) // best effort

	// 对话框创建在当前线程上；先把线程 ID 交给监控协程，它负责把弹框顶到前台。
	dialogTid := windows.GetCurrentThreadId()
	watch := &dialogWatch{}
	stop := make(chan struct{})
	watchMu.Lock()
	watchByTid[dialogTid] = watch
	watchMu.Unlock()
	defer func() {
		watchMu.Lock()
		delete(watchByTid, dialogTid)
		watchMu.Unlock()
		close(stop)
	}()
	go foregroundWatcher(dialogTid, watch, stop)

	// owner 传 0：宿主本来就没有可见窗口。Show 阻塞到对话框关闭。
	hr, _, _ = syscall.SyscallN(vt.show, self, 0)
	if hr != 0 {
		if hr == hresultCancelled {
			return "", nil
		}
		return "", fmt.Errorf("对话框返回 0x%08X", hr)
	}

	var item unsafe.Pointer
	if hr, _, _ = syscall.SyscallN(vt.getResult, self, uintptr(unsafe.Pointer(&item))); hr != 0 {
		return "", fmt.Errorf("GetResult 失败: 0x%08X", hr)
	}
	si := (*shellItem)(item)
	siSelf := uintptr(unsafe.Pointer(si))
	defer syscall.SyscallN(si.vtbl.release, siSelf)

	var p *uint16
	if hr, _, _ = syscall.SyscallN(si.vtbl.getDisplayName, siSelf, sigdnFileSysPath, uintptr(unsafe.Pointer(&p))); hr != 0 {
		return "", fmt.Errorf("GetDisplayName 失败: 0x%08X", hr)
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
	return utf16PtrToString(p), nil
}

// dialogWatch 是 EnumThreadWindows 的落点。回调经由 Windows 派发，不能直接回传 Go 指针
// （uintptr→unsafe.Pointer 过不了 vet 也确实有 GC 风险），改用「线程 ID → watch」注册表。
type dialogWatch struct {
	hwnd atomic.Uintptr
}

var (
	watchMu    sync.Mutex
	watchByTid = map[uint32]*dialogWatch{}
)

// enumCallback 注册一次全局复用；NewCallback 不允许并发注册。
var enumCallback = windows.NewCallback(enumWndProc)

func enumWndProc(hwnd, lparam uintptr) uintptr {
	watchMu.Lock()
	w, ok := watchByTid[uint32(lparam)]
	watchMu.Unlock()
	if !ok {
		return 1
	}
	if r, _, _ := procIsWindowVisible.Call(hwnd); r == 0 {
		return 1
	}
	var cls [8]uint16 // "#32770" 共 7 字符
	procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&cls[0])), 8)
	if windows.UTF16ToString(cls[:]) != "#32770" {
		return 1
	}
	w.hwnd.CompareAndSwap(0, hwnd)
	return 0 // 找到了，停止枚举
}

// foregroundWatcher 每 50ms 扫一遍对话框线程的窗口，出现后立刻顶到前台（最多等 5 秒）。
func foregroundWatcher(dialogTid uint32, watch *dialogWatch, stop <-chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for i := 0; i < 100; i++ {
		select {
		case <-stop:
			return
		default:
		}
		procEnumThreadWindows.Call(uintptr(dialogTid), enumCallback, uintptr(dialogTid))
		if h := watch.hwnd.Load(); h != 0 {
			bringToFront(h, dialogTid)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// bringToFront 抢前台三部曲：TOPMOST 兜底 → AttachThreadInput 借输入队列 → SetForegroundWindow。
func bringToFront(h uintptr, dialogTid uint32) {
	curTid := uintptr(windows.GetCurrentThreadId())
	fg, _, _ := procGetForegroundWindow.Call()
	fgTid := uint32(0)
	if fg != 0 {
		t, _, _ := procGetWindowThreadProcId.Call(fg, 0)
		fgTid = uint32(t)
	}
	attach := func(other uint32, on bool) {
		if other == 0 || uintptr(other) == curTid {
			return
		}
		v := uintptr(0)
		if on {
			v = 1
		}
		procAttachThreadInput.Call(curTid, uintptr(other), v)
	}
	attach(fgTid, true)
	attach(dialogTid, true)
	defer func() {
		attach(fgTid, false)
		attach(dialogTid, false)
	}()
	// TOPMOST 是保底：即使抢前台失败，对话框也保证浮在浏览器上面
	procSetWindowPos.Call(h, hwndTopmost, 0, 0, 0, 0, swpNomove|swpNosize|swpShowWindow)
	procBringWindowToTop.Call(h)
	procSetActiveWindow.Call(h)
	procSetForegroundWindow.Call(h)
	procSetFocus.Call(h)
}

func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(p), 2*n)) != 0 {
		n++
	}
	return windows.UTF16ToString(unsafe.Slice(p, n))
}
