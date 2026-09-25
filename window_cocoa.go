//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Cocoa -framework ServiceManagement
#include <stdlib.h>
void cp_run_settings(const char *profilesJSON, const char *activeID,
                     const char *appliedID, const char *profilesDir,
                     const char *themesJSON, const char *defaultTheme,
                     const char *initialStatus, const void *iconData, int iconLen,
                     int asked);
*/
import "C"

import (
	_ "embed"
	"encoding/json"
	"runtime"
	"unsafe"
)

// Monochrome template image for the menu bar item.
//
//go:embed icon/menubar@2x.png
var menuBarIcon []byte

// Cocoa runs on the main thread.
func init() { runtime.LockOSThread() }

// runWindow opens the settings window on the active profile, with the tick on
// it if applied is true, and says status first if there is anything to say. It
// never returns: the app exits from inside it.
func runWindow(s Settings, applied bool, status string) {
	list, _ := json.Marshal(s.Profiles)
	choices, _ := json.Marshal(themes)
	active, tick := s.Active().ID, ""
	if applied {
		tick = active
	}
	asked := C.int(0)
	if s.LaunchAtLogin {
		asked = 1
	}
	C.cp_run_settings(C.CString(string(list)), C.CString(active), C.CString(tick),
		C.CString(profilesDir), C.CString(string(choices)), C.CString(defaultTheme),
		C.CString(status), unsafe.Pointer(&menuBarIcon[0]), C.int(len(menuBarIcon)), asked)
}

// fromWindow reads the profiles the window sends as JSON.
func fromWindow(profilesJSON *C.char) ([]Profile, error) {
	var profiles []Profile
	err := json.Unmarshal([]byte(C.GoString(profilesJSON)), &profiles)
	return profiles, err
}

// result is the window's answer: a message, and whether it is good news.
func result(text string, ok bool) *C.char {
	b, _ := json.Marshal(struct {
		OK   bool   `json:"ok"`
		Text string `json:"text"`
	}{ok, text})
	return C.CString(string(b))
}

//export cpSaveAll
func cpSaveAll(profilesJSON, activeID *C.char) *C.char {
	profiles, err := fromWindow(profilesJSON)
	if err != nil {
		return result("Not saved: "+err.Error(), false)
	}
	return result(saveProfiles(profiles, C.GoString(activeID)))
}

//export cpApply
func cpApply(profilesJSON, activeID *C.char) *C.char {
	profiles, err := fromWindow(profilesJSON)
	if err != nil {
		return result("Not applied: "+err.Error(), false)
	}
	return result(applyProfile(profiles, C.GoString(activeID)))
}

//export cpTest
func cpTest(token *C.char) *C.char { return result(testToken(C.GoString(token))) }

//export cpRevert
func cpRevert() { quit() }

// cpSetLaunchAtLogin records that the window has registered the app to open at
// login, so that it never does again.
//
//export cpSetLaunchAtLogin
func cpSetLaunchAtLogin() {
	config.LaunchAtLogin = true
	saveConfig(config)
}
