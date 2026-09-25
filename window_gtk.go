//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0
#include <stdlib.h>
void cp_run_settings(const char *profiles, const char *activeID, const char *appliedID,
                     const char *themes, const char *defaultTheme, const char *initialStatus,
                     const void *iconData, int iconLen, int asked);
*/
import "C"

import (
	_ "embed"
	"runtime"
	"strings"
	"unsafe"
)

// The app's icon, for the window and the tray.
//
//go:embed icon/app.png
var appIcon []byte

// GTK runs on the main thread.
func init() { runtime.LockOSThread() }

// Profiles cross to the window as records split by RS, their fields by US:
// id, name, token, dir, theme. Neither character can be typed into a field.
const rs, us = "\x1e", "\x1f"

func encode(profiles []Profile) string {
	var recs []string
	for _, p := range profiles {
		recs = append(recs, strings.Join([]string{p.ID, p.Name, p.Token, p.Dir, p.Theme}, us))
	}
	return strings.Join(recs, rs)
}

func decode(s *C.char) []Profile {
	var profiles []Profile
	for _, rec := range strings.Split(C.GoString(s), rs) {
		if f := strings.Split(rec, us); len(f) == 5 {
			profiles = append(profiles, Profile{ID: f[0], Name: f[1], Token: f[2], Dir: f[3], Theme: f[4]})
		}
	}
	return profiles
}

// runWindow opens the settings window on the active profile, with the tick on
// it if applied is true, and says status first if there is anything to say. It
// never returns: the app exits from inside it.
func runWindow(s Settings, applied bool, status string) {
	var choices []string
	for _, t := range themes {
		choices = append(choices, t.Value+us+t.Name)
	}
	active, tick := s.Active().ID, ""
	if applied {
		tick = active
	}
	asked := C.int(0)
	if s.LaunchAtLogin {
		asked = 1
	}
	C.cp_run_settings(C.CString(encode(s.Profiles)), C.CString(active), C.CString(tick),
		C.CString(strings.Join(choices, rs)), C.CString(defaultTheme), C.CString(status),
		unsafe.Pointer(&appIcon[0]), C.int(len(appIcon)), asked)
}

// result is the window's answer: "1" for good news or "0", then the message.
func result(text string, ok bool) *C.char {
	if ok {
		return C.CString("1" + text)
	}
	return C.CString("0" + text)
}

//export cpSaveAll
func cpSaveAll(profiles, appliedID *C.char) *C.char {
	return result(saveProfiles(decode(profiles), C.GoString(appliedID)))
}

//export cpApply
func cpApply(profiles, id *C.char) *C.char {
	return result(applyProfile(decode(profiles), C.GoString(id)))
}

//export cpTest
func cpTest(token *C.char) *C.char { return result(testToken(C.GoString(token))) }

//export cpRevert
func cpRevert() { quit() }

// cpDefaultDir is the folder a profile gets when none is chosen, as shown.
//
//export cpDefaultDir
func cpDefaultDir(name, id *C.char) *C.char {
	return C.CString(tilde(Profile{ID: C.GoString(id), Name: C.GoString(name)}.Folder()))
}

// cpSetLaunchAtLogin records that the window has registered the app to open at
// login, so that it never does again.
//
//export cpSetLaunchAtLogin
func cpSetLaunchAtLogin() {
	config.LaunchAtLogin = true
	saveConfig(config)
}
