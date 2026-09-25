package main

// On Linux the CLI keeps its login only in .credentials.json in its folder. The
// app keeps the login in a file in memory, and puts there a link to it through
// /proc: each reader opens its own copy, and nothing of it is on disk.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// loginEnv is what the env block of the applied profile's settings.json holds
// for the CLI to find the login the app gives it: nothing, on Linux.
var loginEnv map[string]any

// stashPrefix names where a login of the user's own, found in the folder, waits
// in the keyring until the profile is taken back.
const stashPrefix = "claude-code-login:"

// served is where the login is linked, the memory file behind the link, and the
// watch on the folder.
var served struct {
	sync.Mutex
	path, token string
	mem, watch  *os.File
}

// memfd is a file that lives in memory alone, gone with the last descriptor.
func memfd() (*os.File, error) {
	nr := uintptr(319) // memfd_create on amd64
	if runtime.GOARCH == "arm64" {
		nr = 279
	}
	name, _ := syscall.BytePtrFromString("claude-code-login")
	fd, _, errno := syscall.Syscall(nr, uintptr(unsafe.Pointer(name)), 1, 0) // MFD_CLOEXEC
	if errno != 0 {
		return nil, errno
	}
	return os.NewFile(fd, "claude-code-login"), nil
}

// ours reports whether path is a link the app made, to a memory file through
// /proc: its own now, or one from a run that was killed.
func ours(path string) bool {
	t, err := os.Readlink(path)
	return err == nil && strings.HasPrefix(t, "/proc/") && strings.Contains(t, "/fd/")
}

// giveToken links the login where the CLI reads it in the applied folder, after
// setting aside a login of the user's own found there.
func giveToken(token string) error {
	takeToken()
	path := filepath.Join(linked, ".credentials.json")
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode().IsRegular() {
			own, err := os.ReadFile(path)
			if err == nil {
				err = storeToken(stashPrefix+path, string(own))
			}
			if err != nil {
				return fmt.Errorf("Claude Code's own login could not be set aside: %w", err)
			}
		}
		os.Remove(path)
	}
	data, _ := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{
		"accessToken":  token,
		"refreshToken": nil,
		"expiresAt":    time.Now().AddDate(10, 0, 0).UnixMilli(),
		"scopes":       []string{"user:inference"},
	}})
	mem, err := memfd()
	if err == nil {
		_, err = mem.Write(data)
	}
	if err == nil {
		err = os.Symlink(fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), mem.Fd()), path)
	}
	if err != nil {
		if mem != nil {
			mem.Close()
		}
		return fmt.Errorf("the login for Claude Code could not be put in place: %w", err)
	}
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err == nil {
		if _, err = syscall.InotifyAddWatch(fd, linked, syscall.IN_MOVED_TO|syscall.IN_CLOSE_WRITE); err != nil {
			syscall.Close(fd)
		}
	}
	if err != nil {
		os.Remove(path)
		mem.Close()
		return err
	}
	watch := os.NewFile(uintptr(fd), "inotify")
	served.Lock()
	served.path, served.token, served.mem, served.watch = path, token, mem, watch
	served.Unlock()
	go follow(path, watch)
	return nil
}

// follow catches the CLI writing its login, which it does by renaming a file over
// the link. A login it keeps with the app's token goes into memory, and the link
// comes back; one the user made with /login stays.
func follow(path string, watch *os.File) {
	buf := make([]byte, 4096)
	for {
		n, err := watch.Read(buf)
		if err != nil {
			return
		}
		for i := 0; i+syscall.SizeofInotifyEvent <= n; {
			ev := (*syscall.InotifyEvent)(unsafe.Pointer(&buf[i]))
			name := string(bytes.TrimRight(buf[i+syscall.SizeofInotifyEvent:i+syscall.SizeofInotifyEvent+int(ev.Len)], "\x00"))
			i += syscall.SizeofInotifyEvent + int(ev.Len)
			if name == filepath.Base(path) {
				reclaim(path)
			}
		}
	}
}

// reclaim takes a login the CLI wrote at path into memory, if it is the app's,
// and puts the link back in its place.
func reclaim(path string) {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var l struct {
		ClaudeAiOauth struct{ AccessToken string } `json:"claudeAiOauth"`
	}
	json.Unmarshal(data, &l)
	served.Lock()
	defer served.Unlock()
	if served.path != path || l.ClaudeAiOauth.AccessToken != served.token {
		return
	}
	if served.mem.Truncate(0) != nil {
		return
	}
	if _, err := served.mem.WriteAt(data, 0); err != nil {
		return
	}
	tmp := path + ".CCTokenManager"
	os.Remove(tmp)
	if os.Symlink(fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), served.mem.Fd()), tmp) == nil {
		os.Rename(tmp, path)
	}
}

// takeToken takes the link away and gives back the login set aside, if any. A
// login the user made meanwhile is left alone; a link a killed run left goes.
func takeToken() {
	served.Lock()
	path, mem, watch := served.path, served.mem, served.watch
	served.path, served.token, served.mem, served.watch = "", "", nil, nil
	served.Unlock()
	if watch != nil {
		watch.Close()
	}
	dirs := folders()
	if path != "" {
		dirs = append(dirs, filepath.Dir(path))
	}
	for _, dir := range dirs {
		p := filepath.Join(dir, ".credentials.json")
		if _, err := os.Lstat(p); err == nil && !ours(p) {
			continue // the user's own login, made meanwhile
		}
		os.Remove(p)
		if own, _ := loadToken(stashPrefix + p); own != "" {
			if os.WriteFile(p, []byte(own), 0o600) == nil {
				storeToken(stashPrefix+p, "")
			}
		}
	}
	if mem != nil {
		mem.Close()
	}
}
