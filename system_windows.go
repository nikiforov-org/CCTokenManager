package main

// What differs on Windows: ~/.claude becomes a junction, which needs no admin
// rights, and as a file cannot be one, the profile's .claude.json moves into
// ~/.claude.json's place while the profile is applied.

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// unsafeInName is what a folder named after a profile cannot hold.
const unsafeInName = `\/:*?"<>|`

// homeShown stands for the home folder in messages.
const homeShown = "%USERPROFILE%"

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	pCreateMutexW     = kernel32.NewProc("CreateMutexW")
	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pRegCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	pRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
)

var pExpandEnvironmentStringsW = kernel32.NewProc("ExpandEnvironmentStringsW")

// swapNote says where the .claude.json standing in ~/.claude.json's place came
// from, so that it goes back there even after a crash.
var swapNote = filepath.Join(profilesDir, "claude.json.from")

// onlyInstance reports whether this is the one copy of the app running. A copy
// started again shows the window of the one already running and leaves.
func onlyInstance() bool {
	_, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16(`Local\org.nikiforov.CCTokenManager`))))
	if err != syscall.Errno(183) { // ERROR_ALREADY_EXISTS; the mutex lives as long as the process
		return true
	}
	if w, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16(windowClass))), 0); w != 0 {
		pAllowSetForegroundWindow.Call(0xFFFFFFFF) // ASFW_ANY: this copy was just started, and may hand the front over
		pPostMessageW.Call(w, wmShow, 0, 0)
	}
	return false
}

// watchSignals has nothing to watch: a Windows app hears of a logoff or a
// shutdown through its window.
func watchSignals() {}

// cliInstalled finds claude as a terminal opened now would. Without it the app
// has nothing to switch, so it says so and does not start.
func cliInstalled() bool {
	if findClaude(freshPath()) == "" {
		alert("Claude Code is not installed.",
			"CC Token Manager switches Claude Code between profiles and found no claude on the PATH. Install Claude Code, then start CC Token Manager again.", false)
		return false
	}
	return true
}

// standIn makes ~/.claude a junction to the profile's folder and moves the
// .claude.json from that folder into ~/.claude.json's place.
func standIn(dir string) error {
	swapOut()
	state := filepath.Join(dir, ".claude.json")
	if err := markOnboarded(state); err != nil {
		return err
	}
	if err := junction(claudeDir, dir); err != nil {
		return err
	}
	linked = dir
	return swapIn(state)
}

// standDown undoes standIn: the profile's .claude.json goes back to its folder,
// the junction goes, and what was moved aside comes back.
func standDown() {
	swapOut()
	if ourLink(claudeDir) {
		os.Remove(claudeDir)
	}
	for _, path := range []string{claudeDir, claudeJSON} {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			os.Rename(path+".default", path)
		}
	}
}

// junction makes path a junction to target, after moving aside whatever stood
// there that the app did not put there.
func junction(path, target string) error {
	if _, err := os.Lstat(path); err == nil {
		if ourLink(path) {
			err = os.Remove(path)
		} else {
			err = aside(path)
		}
		if err != nil {
			return err
		}
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return err
	}
	h, err := syscall.CreateFile(u16(path), syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		os.Remove(path)
		return err
	}
	// A mount point's reparse data: the target as the kernel spells it, then as
	// people do, each followed by a null.
	sub, shown := utf16.Encode([]rune(`\??\`+target)), utf16.Encode([]rune(target))
	names := append(append(append(sub, 0), shown...), 0)
	buf := make([]byte, 16+2*len(names))
	binary.LittleEndian.PutUint32(buf[0:], 0xA0000003) // IO_REPARSE_TAG_MOUNT_POINT
	binary.LittleEndian.PutUint16(buf[4:], uint16(len(buf)-8))
	binary.LittleEndian.PutUint16(buf[10:], uint16(2*len(sub)))
	binary.LittleEndian.PutUint16(buf[12:], uint16(2*len(sub)+2))
	binary.LittleEndian.PutUint16(buf[14:], uint16(2*len(shown)))
	for i, c := range names {
		binary.LittleEndian.PutUint16(buf[16+2*i:], c)
	}
	var n uint32
	err = syscall.DeviceIoControl(h, 0x000900A4, &buf[0], uint32(len(buf)), nil, 0, &n, nil) // FSCTL_SET_REPARSE_POINT
	syscall.CloseHandle(h)
	if err != nil {
		os.Remove(path)
	}
	return err
}

// swapIn moves the profile's state file into ~/.claude.json's place, after
// moving aside what stood there. The note is written first: whatever happens
// after it, swapOut knows where the file belongs.
func swapIn(state string) error {
	if _, err := os.Lstat(claudeJSON); err == nil {
		if err := aside(claudeJSON); err != nil {
			return err
		}
	}
	if err := os.WriteFile(swapNote, []byte(state), 0o600); err != nil {
		return err
	}
	return move(state, claudeJSON)
}

// swapOut moves a profile's state file from ~/.claude.json's place back into
// its folder, as the note says.
func swapOut() {
	back, err := os.ReadFile(swapNote)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(string(back)), 0o700)
	if err := move(claudeJSON, string(back)); err == nil || os.IsNotExist(err) {
		os.Remove(swapNote)
	}
}

// move renames src to dst, copying when they are on different disks, and tries
// again for a moment while a running claude has the file open.
func move(src, dst string) error {
	var err error
	for range 20 {
		if err = os.Rename(src, dst); err == nil || os.IsNotExist(err) {
			return err
		}
		if errors.Is(err, syscall.Errno(17)) { // ERROR_NOT_SAME_DEVICE
			data, err := os.ReadFile(src)
			if err == nil {
				err = os.WriteFile(dst, data, 0o600)
			}
			if err == nil {
				err = os.Remove(src)
			}
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

// freshPath is the PATH a terminal opened now gets: the system's and the
// user's, as the registry holds them. The app's own dates from its start.
func freshPath() string {
	var parts []string
	for _, k := range []struct {
		root syscall.Handle
		key  string
	}{
		{syscall.HKEY_LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`},
		{syscall.HKEY_CURRENT_USER, `Environment`},
	} {
		if v := regString(k.root, k.key, "Path"); v != "" {
			parts = append(parts, expand(v))
		}
	}
	if len(parts) == 0 {
		return os.Getenv("PATH")
	}
	return strings.Join(parts, ";")
}

// regString is the string value name of the registry key, or "".
func regString(root syscall.Handle, key, name string) string {
	var h syscall.Handle
	if syscall.RegOpenKeyEx(root, u16(key), 0, syscall.KEY_READ, &h) != nil {
		return ""
	}
	defer syscall.RegCloseKey(h)
	var typ, n uint32
	if syscall.RegQueryValueEx(h, u16(name), nil, &typ, nil, &n) != nil || n == 0 {
		return ""
	}
	buf := make([]uint16, n/2+1)
	if syscall.RegQueryValueEx(h, u16(name), nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &n) != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// expand is s with its %VARIABLES% put in, as a REG_EXPAND_SZ needs.
func expand(s string) string {
	n, _, _ := pExpandEnvironmentStringsW.Call(uintptr(unsafe.Pointer(u16(s))), 0, 0)
	if n == 0 {
		return s
	}
	buf := make([]uint16, n)
	pExpandEnvironmentStringsW.Call(uintptr(unsafe.Pointer(u16(s))), uintptr(unsafe.Pointer(&buf[0])), n)
	return syscall.UTF16ToString(buf)
}

// findClaude is claude on path, found as `where claude` finds it: folder by
// folder, with each extension PATHEXT names.
func findClaude(path string) string {
	exts := strings.Split(strings.ToLower(os.Getenv("PATHEXT")), ";")
	if os.Getenv("PATHEXT") == "" {
		exts = []string{".com", ".exe", ".bat", ".cmd"}
	}
	for _, dir := range filepath.SplitList(path) {
		for _, ext := range exts {
			if dir == "" || ext == "" {
				continue
			}
			p := filepath.Join(dir, "claude"+ext)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p
			}
		}
	}
	return ""
}

// claudeCommand is claude as a terminal opened now finds it, with args, run
// with that PATH and no console window. A batch file runs through cmd.exe.
func claudeCommand(ctx context.Context, args ...string) (*exec.Cmd, string) {
	path := freshPath()
	cliPath := findClaude(path)
	if cliPath == "" {
		return nil, "Claude Code is not on your PATH."
	}
	var cmd *exec.Cmd
	if ext := strings.ToLower(filepath.Ext(cliPath)); ext == ".cmd" || ext == ".bat" {
		line := `"` + comspec() + `" /d /s /c ""` + cliPath + `"`
		for _, a := range args {
			line += " " + syscall.EscapeArg(a)
		}
		cmd = exec.CommandContext(ctx, comspec())
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line + `"`, HideWindow: true, CreationFlags: createNoWindow}
	} else {
		cmd = exec.CommandContext(ctx, cliPath, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	}
	cmd.Env = append(os.Environ(), "PATH="+path)
	return cmd, ""
}

const createNoWindow = 0x08000000

func comspec() string {
	if c := os.Getenv("ComSpec"); c != "" {
		return c
	}
	return filepath.Join(os.Getenv("SystemRoot"), `System32\cmd.exe`)
}

// registerAtLogin adds the app to the programs Windows starts at login.
func registerAtLogin() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	var h syscall.Handle
	if r, _, _ := pRegCreateKeyExW.Call(uintptr(syscall.HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(u16(`Software\Microsoft\Windows\CurrentVersion\Run`))),
		0, 0, 0, syscall.KEY_SET_VALUE, 0, uintptr(unsafe.Pointer(&h)), 0); r != 0 {
		return false
	}
	defer syscall.RegCloseKey(h)
	value := wide(`"` + exe + `"`)
	r, _, _ := pRegSetValueExW.Call(uintptr(h), uintptr(unsafe.Pointer(u16("CC Token Manager"))), 0, syscall.REG_SZ,
		uintptr(unsafe.Pointer(&value[0])), uintptr(2*len(value)))
	return r == 0
}

// u16 is s as Windows takes a string.
func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// wide is s as Windows takes a string, null and all.
func wide(s string) []uint16 {
	if w, err := syscall.UTF16FromString(s); err == nil {
		return w
	}
	return []uint16{0}
}
