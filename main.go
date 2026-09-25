// CC Token Manager keeps Claude Code OAuth tokens as profiles, each with a config
// folder of its own, and while it runs switches Claude Code to the
// applied one. A tray app for macOS, Windows and Linux.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A Profile is one subscription: its OAuth token, the folder Claude Code keeps
// its config in while the profile is applied, and the theme it shows meanwhile.
type Profile struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token,omitempty"` // kept in the system's store, never in the config file
	Dir   string `json:"dir,omitempty"`   // chosen in the window; empty means the default
	Theme string `json:"theme"`           // one of themes
}

type Settings struct {
	Profiles []Profile `json:"profiles"`
	ActiveID string    `json:"active_profile"` // the profile applied at launch
	// LaunchAtLogin is true once the app has registered to open at login. It
	// does so only once, so a removal in the system's settings stays.
	LaunchAtLogin bool `json:"launch_at_login,omitempty"`
}

var (
	home, _ = os.UserHomeDir()
	// profilesDir holds the config file and the profiles' default folders.
	profilesDir = filepath.Join(home, ".CCTokenManager")
	configPath  = filepath.Join(profilesDir, "config.json")
	// config is the config file as last read or saved. Only the thread that
	// runs the window touches it: the startup, then the window.
	config Settings
)

// Folder is the profile's Claude Code config folder: the one chosen for it, or
// one in profilesDir named after it, with what a file name cannot hold made a
// dash. The window works out the default the same way.
func (p Profile) Folder() string {
	if d := p.Dir; d == "~" || strings.HasPrefix(d, "~/") || strings.HasPrefix(d, "~"+string(filepath.Separator)) {
		return filepath.Join(home, d[1:])
	} else if d != "" {
		return d
	}
	n := strings.TrimSpace(strings.Map(func(r rune) rune {
		if strings.ContainsRune(unsafeInName, r) {
			return '-'
		}
		return r
	}, p.Name))
	if n == "" || n == "." || n == ".." {
		n = p.ID
	}
	return filepath.Join(profilesDir, n)
}

// Active is the profile to apply: the one the settings name, or else the first.
// There is always a first: the app starts with one, and the window keeps one.
func (s Settings) Active() Profile {
	for _, p := range s.Profiles {
		if p.ID == s.ActiveID {
			return p
		}
	}
	return s.Profiles[0]
}

// saveConfig puts the tokens in the system's store and the rest in the config
// file, which is written only once every token is stored. A profile gone since
// the last save takes its token out of the store with it.
func saveConfig(s Settings) error {
	for _, p := range s.Profiles {
		if err := storeToken(p.ID, p.Token); err != nil {
			return fmt.Errorf("%s did not take the token of %q: %w", storeName, p.Name, err)
		}
	}
	for _, p := range config.Profiles {
		if !slices.ContainsFunc(s.Profiles, func(q Profile) bool { return q.ID == p.ID }) {
			storeToken(p.ID, "")
		}
	}
	file := s
	file.Profiles = slices.Clone(s.Profiles)
	for i := range file.Profiles {
		file.Profiles[i].Token = ""
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err == nil {
		err = os.WriteFile(configPath, data, 0o600)
	}
	return err
}

func main() {
	// A second copy quitting would take the profile away from the first.
	os.MkdirAll(profilesDir, 0o700)
	if !onlyInstance() {
		return
	}

	// The config file, or one empty profile. A file that is not valid JSON is
	// moved aside rather than written over.
	var status string
	data, err := os.ReadFile(configPath)
	switch {
	case err != nil && !os.IsNotExist(err):
		status = err.Error() + "."
	case err == nil && json.Unmarshal(data, &config) != nil:
		config = Settings{}
		status = tilde(configPath) + " is not valid JSON and was moved to config.json.broken."
		if err := os.Rename(configPath, configPath+".broken"); err != nil {
			status = tilde(configPath) + " is not valid JSON and could not be moved aside: " + err.Error() + "."
		}
	}
	if len(config.Profiles) == 0 {
		config.Profiles = []Profile{{ID: "default", Name: "Default"}}
	}
	// A profile with a theme the window does not offer has the default.
	for i, p := range config.Profiles {
		if !slices.ContainsFunc(themes, func(t theme) bool { return t.Value == p.Theme }) {
			config.Profiles[i].Theme = defaultTheme
		}
	}
	// The tokens come from the system's store.
	for i, p := range config.Profiles {
		var err error
		if config.Profiles[i].Token, err = loadToken(p.ID); err != nil {
			status = strings.TrimSpace(fmt.Sprintf("%s %s did not give the token of %q: %v.", status, storeName, p.Name, err))
		}
	}

	// Without Claude Code there is nothing for the app to switch.
	if !cliInstalled() {
		return
	}
	watchSignals()

	// A launch that applied the profile has nothing more to say; one that could
	// not says why in the window, or nobody would know.
	msg, ok := apply(config)
	if !ok {
		status = strings.TrimSpace(status + " " + msg)
	}
	runWindow(config, ok, status)
}
