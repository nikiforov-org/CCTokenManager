package main

// CC Token Manager Setup: one amd64 program, which Windows 11 on Arm emulates,
// carrying the app for both architectures and installing the native one. Run
// with /uninstall, as its Uninstall entry does, it takes everything back.

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

// The payloads are staged here by the build.sh beside this file, which alone
// can build this package: go:embed needs them.
//
//go:embed payload/amd64.exe
var payloadAMD64 []byte

//go:embed payload/arm64.exe
var payloadARM64 []byte

// version is set from the build.sh beside this file with -ldflags -X, for
// the Programs and Features entry.
var version = "dev"

const (
	appName        = "CC Token Manager"
	binName        = "CCTokenManager.exe"
	uninstallName  = "uninstall.exe"
	mutexName      = `Local\org.nikiforov.CCTokenManager` // matches system_windows.go's onlyInstance
	runKey         = `Software\Microsoft\Windows\CurrentVersion\Run`
	uninstallKey   = `Software\Microsoft\Windows\CurrentVersion\Uninstall\CCTokenManager`
	createNoWindow = 0x08000000
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")

	pCreateMutexW    = kernel32.NewProc("CreateMutexW")
	pCloseHandle     = kernel32.NewProc("CloseHandle")
	pIsWow64Process2 = kernel32.NewProc("IsWow64Process2")

	pRegCreateKeyExW = advapi32.NewProc("RegCreateKeyExW")
	pRegSetValueExW  = advapi32.NewProc("RegSetValueExW")
	pRegOpenKeyExW   = advapi32.NewProc("RegOpenKeyExW")
	pRegDeleteValueW = advapi32.NewProc("RegDeleteValueW")
	pRegDeleteKeyW   = advapi32.NewProc("RegDeleteKeyW")
	pCredEnumerateW  = advapi32.NewProc("CredEnumerateW")
	pCredDeleteW     = advapi32.NewProc("CredDeleteW")
	pCredFree        = advapi32.NewProc("CredFree")

	pCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	pCoCreateInstance = ole32.NewProc("CoCreateInstance")

	pTaskDialog = comctl32.NewProc("TaskDialog")
)

// COM calls need one, and the same, OS thread throughout.
func init() { runtime.LockOSThread() }

func main() {
	silent, doUninstall := false, false
	for _, a := range os.Args[1:] {
		switch strings.ToLower(a) {
		case "/s", "/silent":
			silent = true
		case "/uninstall":
			doUninstall = true
		}
	}
	if doUninstall {
		uninstall(silent)
	} else {
		install(silent)
	}
}

// install puts CCTokenManager.exe where it belongs, with a Start Menu
// shortcut and a Programs and Features entry, and offers to launch it.
func install(silent bool) {
	notify := func(head, detail string) {
		if !silent {
			alert(head, detail)
		}
	}
	if running() {
		notify("CC Token Manager is running.", "Quit it from its tray icon (right-click, then Quit), then run Setup again.")
		os.Exit(1)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		notify("Could not install CC Token Manager.", err.Error())
		os.Exit(1)
	}
	dir := filepath.Join(cache, "Programs", appName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		notify("Could not install CC Token Manager.", err.Error())
		os.Exit(1)
	}

	// An earlier install's exe moves aside first: a file in use can be renamed,
	// not written over.
	target := filepath.Join(dir, binName)
	if _, err := os.Lstat(target); err == nil {
		os.Rename(target, target+".old")
	}
	payload := payloadAMD64
	if isARM64() {
		payload = payloadARM64
	}
	if err := os.WriteFile(target, payload, 0o755); err != nil {
		notify("Could not install CC Token Manager.", "Quit it from its tray icon, then run Setup again.")
		os.Exit(1)
	}
	os.Remove(target + ".old")

	if exePath, err := os.Executable(); err == nil {
		if self, err := os.ReadFile(exePath); err == nil {
			os.WriteFile(filepath.Join(dir, uninstallName), self, 0o755)
		}
	}
	if cfg, err := os.UserConfigDir(); err == nil {
		shortcut(filepath.Join(cfg, "Microsoft", "Windows", "Start Menu", "Programs", appName+".lnk"), target)
	}
	registerUninstall(dir, target, filepath.Join(dir, uninstallName))

	if silent {
		return
	}
	if ask("CC Token Manager is installed.", "Installed at "+dir+". Launch it now?") {
		exec.Command(target).Start()
	}
}

// uninstall takes back everything install put down, after asking whether the
// saved profiles and tokens should go too.
func uninstall(silent bool) {
	if running() {
		if !silent {
			alert("CC Token Manager is running.", "Quit it from its tray icon (right-click, then Quit), then run Uninstall again.")
		}
		os.Exit(1)
	}
	cache, _ := os.UserCacheDir()
	dir := filepath.Join(cache, "Programs", appName)

	deleteProfiles := false
	if !silent {
		deleteProfiles = ask("Remove CC Token Manager.",
			`Delete the saved profiles in %USERPROFILE%\.CCTokenManager, and their tokens in Credential Manager, too, or keep them? Yes deletes them.`)
	}
	if cfg, err := os.UserConfigDir(); err == nil {
		os.Remove(filepath.Join(cfg, "Microsoft", "Windows", "Start Menu", "Programs", appName+".lnk"))
	}
	deleteRunValue()
	deleteUninstallKey()
	if deleteProfiles {
		if home, err := os.UserHomeDir(); err == nil {
			os.RemoveAll(filepath.Join(home, ".CCTokenManager"))
		}
		deleteTokens()
	}
	os.Remove(filepath.Join(dir, binName))
	selfDelete(dir)
}

// credential is the start of CREDENTIALW, as far as deleteTokens reads it.
type credential struct {
	Flags, Type uint32
	TargetName  *uint16
}

// deleteTokens removes the tokens the app keeps in Credential Manager, each
// named after the app and a profile.
func deleteTokens() {
	var n uint32
	var list **credential
	if r, _, _ := pCredEnumerateW.Call(uintptr(unsafe.Pointer(u16(appName+"/*"))), 0,
		uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&list))); r == 0 {
		return
	}
	defer pCredFree.Call(uintptr(unsafe.Pointer(list)))
	for _, c := range unsafe.Slice(list, n) {
		pCredDeleteW.Call(uintptr(unsafe.Pointer(c.TargetName)), uintptr(c.Type), 0)
	}
}

// selfDelete removes dir, which holds the running uninstall.exe, a moment
// after this process is gone: a running exe cannot simply delete its file.
func selfDelete(dir string) {
	script := `Start-Sleep -Seconds 2; Remove-Item -LiteralPath '` + strings.ReplaceAll(dir, "'", "''") + `' -Recurse -Force`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	cmd.Start()
}

// running reports whether CC Token Manager is running, the same way its own
// onlyInstance does: the mutex lives as long as the process holding it.
func running() bool {
	h, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16(mutexName))))
	if h != 0 {
		pCloseHandle.Call(h)
	}
	return err == syscall.Errno(183) // ERROR_ALREADY_EXISTS
}

// isARM64 asks for the machine's processor, not this process's, which under
// x64 emulation is x64.
func isARM64() bool {
	var process, native uint16
	if pIsWow64Process2.Find() != nil {
		return false
	}
	r, _, _ := pIsWow64Process2.Call(^uintptr(0), // the current process
		uintptr(unsafe.Pointer(&process)), uintptr(unsafe.Pointer(&native)))
	return r != 0 && native == 0xAA64 // IMAGE_FILE_MACHINE_ARM64
}

// registerUninstall lists CC Token Manager in Settings > Apps, with its
// uninstaller pointed at the copy of this program install left beside it.
func registerUninstall(dir, target, uninstaller string) {
	var h syscall.Handle
	if r, _, _ := pRegCreateKeyExW.Call(uintptr(syscall.HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(u16(uninstallKey))), 0, 0, 0, syscall.KEY_SET_VALUE, 0,
		uintptr(unsafe.Pointer(&h)), 0); r != 0 {
		return
	}
	defer syscall.RegCloseKey(h)
	str := func(name, value string) {
		w := wide(value)
		pRegSetValueExW.Call(uintptr(h), uintptr(unsafe.Pointer(u16(name))), 0, syscall.REG_SZ,
			uintptr(unsafe.Pointer(&w[0])), uintptr(2*len(w)))
	}
	dword := func(name string, value uint32) {
		pRegSetValueExW.Call(uintptr(h), uintptr(unsafe.Pointer(u16(name))), 0, 4, // REG_DWORD
			uintptr(unsafe.Pointer(&value)), 4)
	}
	str("DisplayName", appName)
	str("DisplayVersion", version)
	str("Publisher", "org.nikiforov")
	str("InstallLocation", dir)
	str("DisplayIcon", target)
	str("UninstallString", `"`+uninstaller+`" /uninstall`)
	dword("NoModify", 1)
	dword("NoRepair", 1)
}

func deleteRunValue() {
	var h syscall.Handle
	if r, _, _ := pRegOpenKeyExW.Call(uintptr(syscall.HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(u16(runKey))), 0, syscall.KEY_SET_VALUE, uintptr(unsafe.Pointer(&h))); r == 0 {
		pRegDeleteValueW.Call(uintptr(h), uintptr(unsafe.Pointer(u16(appName))))
		syscall.RegCloseKey(h)
	}
}

func deleteUninstallKey() {
	pRegDeleteKeyW.Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(u16(uninstallKey))))
}

var (
	clsidShellLink = guid{0x00021401, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidShellLinkW  = guid{0x000214F9, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidPersistFile = guid{0x0000010B, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

type guid struct {
	Data1        uint32
	Data2, Data3 uint16
	Data4        [8]byte
}

// shortcut makes a Start Menu entry at path for target, so it can be found
// and started without its tray icon or the login item.
func shortcut(path, target string) {
	pCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED
	var link unsafe.Pointer
	if r, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, 1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidShellLinkW)), uintptr(unsafe.Pointer(&link))); r != 0 {
		return
	}
	defer com(link, 2)                                               // Release
	com(link, 20, uintptr(unsafe.Pointer(u16(target))))              // SetPath
	com(link, 9, uintptr(unsafe.Pointer(u16(filepath.Dir(target))))) // SetWorkingDirectory
	var file unsafe.Pointer
	if com(link, 0, uintptr(unsafe.Pointer(&iidPersistFile)), uintptr(unsafe.Pointer(&file))) != 0 { // QueryInterface
		return
	}
	defer com(file, 2)
	os.MkdirAll(filepath.Dir(path), 0o755)
	com(file, 6, uintptr(unsafe.Pointer(u16(path))), 1) // Save(path, TRUE)
}

// com calls method n of a COM object: its place in the object's table of
// methods, which starts with IUnknown's three.
func com(obj unsafe.Pointer, n int, args ...uintptr) uintptr {
	method := *(*uintptr)(unsafe.Add(*(*unsafe.Pointer)(obj), n*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(method, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

func dialog(head, detail string, buttons, icon uintptr) int32 {
	var pressed int32
	pTaskDialog.Call(0, 0, uintptr(unsafe.Pointer(u16("CC Token Manager"))),
		uintptr(unsafe.Pointer(u16(head))), uintptr(unsafe.Pointer(u16(detail))),
		buttons, icon, uintptr(unsafe.Pointer(&pressed)))
	return pressed
}

func alert(head, detail string) { dialog(head, detail, 1, 0xFFFF) } // TDCBF_OK_BUTTON, TD_WARNING_ICON

func ask(head, detail string) bool {
	return dialog(head, detail, 2|4, 0) == 6 // TDCBF_YES_BUTTON | TDCBF_NO_BUTTON; IDYES
}

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func wide(s string) []uint16 {
	if w, err := syscall.UTF16FromString(s); err == nil {
		return w
	}
	return []uint16{0}
}
