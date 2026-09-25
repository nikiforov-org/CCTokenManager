package main

// CC Token Manager Setup for Linux: one program per architecture, carrying the
// app built for it. Run with --uninstall, as the menu entry's Uninstall action
// does, it takes everything back.

/*
#cgo pkg-config: gtk+-3.0 libsecret-1
#include <gtk/gtk.h>
#include <libsecret/secret.h>
#include <stdlib.h>

int cp_gui;

// cp_dialog shows a message with OK, or asks yes or no; it says whether Yes was
// pressed.
static int cp_dialog(const char *head, const char *detail, int question) {
	GtkWidget *d = gtk_message_dialog_new(NULL, GTK_DIALOG_MODAL,
		question ? GTK_MESSAGE_QUESTION : GTK_MESSAGE_WARNING,
		question ? GTK_BUTTONS_YES_NO : GTK_BUTTONS_OK, "%s", head);
	gtk_window_set_title(GTK_WINDOW(d), "CC Token Manager");
	gtk_message_dialog_format_secondary_text(GTK_MESSAGE_DIALOG(d), "%s", detail);
	int yes = gtk_dialog_run(GTK_DIALOG(d)) == GTK_RESPONSE_YES;
	gtk_widget_destroy(d);
	while (gtk_events_pending()) gtk_main_iteration();
	return yes;
}

// cp_clear_tokens takes every secret of the app's schema out of the keyring.
static void cp_clear_tokens(void) {
	static const SecretSchema schema = {
		"org.nikiforov.CCTokenManager", SECRET_SCHEMA_NONE, {{"id", SECRET_SCHEMA_ATTRIBUTE_STRING}, {NULL, 0}},
	};
	secret_password_clear_sync(&schema, NULL, NULL, NULL);
}
*/
import "C"

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// The payload is staged here by the build.sh beside this file before this
// package is built.
//
//go:embed payload/CCTokenManager
var payload []byte

//go:embed payload/app.png
var icon []byte

// version is set from the build.sh beside this file with -ldflags -X.
var version = "dev"

const (
	appName = "CC Token Manager"
	binName = "CCTokenManager"
	entryID = "org.nikiforov.CCTokenManager"
)

func main() {
	silent, doUninstall := false, false
	for _, a := range os.Args[1:] {
		switch a {
		case "-s", "--silent":
			silent = true
		case "--uninstall":
			doUninstall = true
		}
	}
	if !silent && C.gtk_init_check(nil, nil) != 0 {
		C.cp_gui = 1
	}
	if doUninstall {
		uninstall(silent)
	} else {
		install(silent)
	}
}

func dataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(d) {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

// install puts CCTokenManager where it belongs, with an entry in the menu of
// applications, and offers to launch it.
func install(silent bool) {
	notify := func(head, detail string) {
		if !silent {
			alert(head, detail)
		}
	}
	if running() {
		notify("CC Token Manager is running.", "Quit it from its tray icon (Quit in its menu), then run Setup again.")
		os.Exit(1)
	}
	dir := filepath.Join(dataDir(), binName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		notify("Could not install CC Token Manager.", err.Error())
		os.Exit(1)
	}

	// Written beside the old one and renamed over it: a copy starting meanwhile
	// finds one or the other.
	target := filepath.Join(dir, binName)
	if err := os.WriteFile(target+".new", payload, 0o755); err != nil || os.Rename(target+".new", target) != nil {
		notify("Could not install CC Token Manager.", fmt.Sprint(err))
		os.Exit(1)
	}
	uninstaller := filepath.Join(dir, "uninstall")
	if exe, err := os.Executable(); err == nil {
		if self, err := os.ReadFile(exe); err == nil {
			os.WriteFile(uninstaller+".new", self, 0o755)
			os.Rename(uninstaller+".new", uninstaller)
		}
	}
	iconDir := filepath.Join(dataDir(), "icons", "hicolor", "256x256", "apps")
	os.MkdirAll(iconDir, 0o755)
	os.WriteFile(filepath.Join(iconDir, binName+".png"), icon, 0o644)
	apps := filepath.Join(dataDir(), "applications")
	os.MkdirAll(apps, 0o755)
	os.WriteFile(filepath.Join(apps, entryID+".desktop"), []byte(fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Comment=Profiles for Claude Code
Exec=%s
Icon=%s
Categories=Development;
Actions=Uninstall;
X-CC-Token-Manager-Version=%s

[Desktop Action Uninstall]
Name=Uninstall
Exec=%s --uninstall
`, appName, quote(target), binName, version, quote(uninstaller))), 0o644)
	refreshMenus()

	if silent {
		return
	}
	if ask("CC Token Manager is installed.", "Installed at "+dir+". Launch it now?") {
		cmd := exec.Command(target)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		cmd.Start()
	}
}

// uninstall takes back everything install put down, after asking whether the
// saved profiles and tokens should go too.
func uninstall(silent bool) {
	if running() {
		if !silent {
			alert("CC Token Manager is running.", "Quit it from its tray icon (Quit in its menu), then run Uninstall again.")
		}
		os.Exit(1)
	}
	deleteProfiles := false
	if !silent {
		deleteProfiles = ask("Remove CC Token Manager.",
			"Delete the saved profiles in ~/.CCTokenManager, and their tokens in the keyring, too, or keep them? Yes deletes them.")
	}
	os.Remove(filepath.Join(dataDir(), "applications", entryID+".desktop"))
	os.Remove(filepath.Join(dataDir(), "icons", "hicolor", "256x256", "apps", binName+".png"))
	if cfg, err := os.UserConfigDir(); err == nil {
		os.Remove(filepath.Join(cfg, "autostart", entryID+".desktop"))
	}
	refreshMenus()
	if deleteProfiles {
		if home, err := os.UserHomeDir(); err == nil {
			os.RemoveAll(filepath.Join(home, ".CCTokenManager"))
		}
		C.cp_clear_tokens()
	}
	os.RemoveAll(filepath.Join(dataDir(), binName))
}

// running reports whether CC Token Manager is running, the way the app itself
// tells: it holds a lock on ~/.CCTokenManager/.lock as long as it runs.
func running() bool {
	home, _ := os.UserHomeDir()
	fd, err := syscall.Open(filepath.Join(home, ".CCTokenManager", ".lock"), syscall.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer syscall.Close(fd)
	return syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil
}

// quote is path as a desktop entry's Exec key takes it.
func quote(path string) string {
	r := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", `$`, `\\$`)
	return `"` + r.Replace(path) + `"`
}

// refreshMenus has the desktop pick up the entry at once where it caches them.
func refreshMenus() {
	exec.Command("update-desktop-database", filepath.Join(dataDir(), "applications")).Run()
	exec.Command("gtk-update-icon-cache", "-q", "-t", filepath.Join(dataDir(), "icons", "hicolor")).Run()
}

// alert and ask use dialogs where there is a display, and the terminal where
// there is not.
func alert(head, detail string) {
	if C.cp_gui != 0 {
		dialog(head, detail, false)
		return
	}
	fmt.Fprintf(os.Stderr, "%s %s\n", head, detail)
}

func ask(head, detail string) bool {
	if C.cp_gui != 0 {
		return dialog(head, detail, true)
	}
	fmt.Fprintf(os.Stderr, "%s %s [y/N] ", head, detail)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y")
}

func dialog(head, detail string, question bool) bool {
	h, d := C.CString(head), C.CString(detail)
	defer C.free(unsafe.Pointer(h))
	defer C.free(unsafe.Pointer(d))
	q := C.int(0)
	if question {
		q = 1
	}
	return C.cp_dialog(h, d, q) != 0
}
