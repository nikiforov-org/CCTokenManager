//go:build darwin || linux

package main

// What differs on macOS and Linux: ~/.claude, ~/.claude.json and the CLI's
// cache folder become symbolic links, and claude is looked for the way a
// terminal finds it.

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// unsafeInName is what a folder named after a profile cannot hold.
const unsafeInName = "/"

// homeShown stands for the home folder in messages.
const homeShown = "~"

// onlyInstance reports whether this is the one copy of the app running. The
// lock lasts as long as the process: nothing closes a bare descriptor.
func onlyInstance() bool {
	fd, err := syscall.Open(filepath.Join(profilesDir, ".lock"), syscall.O_CREAT|syscall.O_RDWR, 0o600)
	if err == nil && syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		log.Println("CC Token Manager is already running.")
		return false
	}
	return true
}

// watchSignals has a stop from a terminal take the profile back as a quit does.
func watchSignals() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-stop
		quit()
		os.Exit(0)
	}()
}

// cliInstalled is always true here: the app starts either way.
func cliInstalled() bool { return true }

// cacheDir is the CLI's cache folder, which holds the logs of MCP servers.
var cacheDir = func() string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Caches", "claude-cli-nodejs")
	}
	if d := os.Getenv("XDG_CACHE_HOME"); filepath.IsAbs(d) {
		return filepath.Join(d, "claude-cli-nodejs")
	}
	return filepath.Join(home, ".cache", "claude-cli-nodejs")
}()

// standIn makes ~/.claude a link to the profile's folder, ~/.claude.json a
// link to the .claude.json in it, which is where the CLI keeps that file for a
// folder of its own, and the CLI's cache folder a link to one in it.
func standIn(dir string) error {
	state := filepath.Join(dir, ".claude.json")
	if err := markOnboarded(state); err != nil {
		return err
	}
	cache := filepath.Join(dir, "claude-cli-nodejs")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o700); err != nil {
		return err
	}
	if err := link(claudeDir, dir); err != nil {
		return err
	}
	linked = dir
	if err := link(claudeJSON, state); err != nil {
		return err
	}
	return link(cacheDir, cache)
}

// standDown undoes standIn: the links go, and what was moved aside comes back.
func standDown() {
	for _, path := range []string{claudeDir, claudeJSON, cacheDir} {
		if ourLink(path) {
			os.Remove(path)
		}
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			os.Rename(path+".default", path)
		}
	}
}

// link makes path a link to target, after moving aside whatever stood there
// that the app did not put there.
func link(path, target string) error {
	if _, err := os.Lstat(path); err == nil && !ourLink(path) {
		if err := aside(path); err != nil {
			return err
		}
	}
	// Made beside it and renamed over it, the new link replaces the old one in
	// one step: a CLI starting meanwhile never finds the place empty.
	tmp := path + ".CCTokenManager"
	os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// diskRoot reports whether dir is the top of a disk: the root, or where one is
// mounted, such as a drive in /Volumes or /media.
func diskRoot(dir string) bool {
	fi, err := os.Stat(dir)
	up, err2 := os.Stat(filepath.Dir(dir))
	if err != nil || err2 != nil {
		return false
	}
	return filepath.Dir(dir) == dir ||
		fi.Sys().(*syscall.Stat_t).Dev != up.Sys().(*syscall.Stat_t).Dev
}

// loginShell is the user's shell: $SHELL, or, for an app started without it,
// the one the system keeps for the user (/etc/passwd, or dscl on macOS).
func loginShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	if u, err := user.Current(); err == nil {
		if data, err := os.ReadFile("/etc/passwd"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if f := strings.Split(line, ":"); len(f) == 7 && f[0] == u.Username && f[6] != "" {
					return f[6]
				}
			}
		}
	}
	if u, err := user.Current(); err == nil && runtime.GOOS == "darwin" {
		out, _ := exec.Command("/usr/bin/dscl", ".", "-read", "/Users/"+u.Username, "UserShell").Output()
		if s, ok := strings.CutPrefix(strings.TrimSpace(string(out)), "UserShell: "); ok {
			return s
		}
	}
	return "/bin/sh"
}

// claudeCommand is claude as a terminal runs it: found on the PATH the login
// shell sets up, and run with that PATH, since an app started from the desktop
// gets only the bare system one and an npm install needs its node.
func claudeCommand(ctx context.Context, args ...string) (*exec.Cmd, string) {
	sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	shell := exec.CommandContext(sctx, loginShell(), "-l", "-i", "-c", "/usr/bin/env")
	shell.WaitDelay = time.Second
	out, _ := shell.Output()
	path := os.Getenv("PATH")
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "PATH="); ok {
			path = p
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if !filepath.IsAbs(dir) {
			continue
		}
		if bin, err := exec.LookPath(filepath.Join(dir, "claude")); err == nil {
			cmd := exec.CommandContext(ctx, bin, args...)
			cmd.Env = append(os.Environ(), "PATH="+path)
			return cmd, ""
		}
	}
	return nil, "Claude Code is not on your login shell's PATH."
}
