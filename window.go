package main

// What the window's buttons do, on every system.

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

// A theme is one of those /theme in the CLI offers: the value its settings
// take, and the name it goes by there, which the window shows too.
type theme struct{ Value, Name string }

// themes are the themes /theme offers, in its order.
var themes = []theme{
	{"auto", "Auto (match terminal)"},
	{"dark", "Dark mode"},
	{"light", "Light mode"},
	{"dark-daltonized", "Dark mode (colorblind-friendly)"},
	{"light-daltonized", "Light mode (colorblind-friendly)"},
	{"dark-ansi", "Dark mode (ANSI colors only)"},
	{"light-ansi", "Light mode (ANSI colors only)"},
}

// tokenVar is where the CLI looks for an OAuth token given in its environment.
const tokenVar = "CLAUDE_CODE_OAUTH_TOKEN"

// defaultTheme is the theme a profile starts with: the system's, as the
// terminal shows it. Left to itself the CLI would be dark.
const defaultTheme = "auto"

// tidy reads the profiles as they stand on screen. A name may be padded, and a
// pasted token arrives wrapped across lines.
func tidy(profiles []Profile) []Profile {
	for i := range profiles {
		p := &profiles[i]
		p.Name, p.Dir = strings.TrimSpace(p.Name), strings.TrimSpace(p.Dir)
		p.Token = strings.Join(strings.Fields(p.Token), "")
		// A guard as it is by default is not the profile's choice.
		for _, g := range guards {
			if v, ok := p.Guards[g.ID()]; ok && v == g.Ticked {
				delete(p.Guards, g.ID())
			}
		}
		if len(p.Guards) == 0 {
			p.Guards = nil
		}
	}
	return profiles
}

// saveProfiles writes the profiles on screen to the config file and switches
// nothing. The file remembers the applied profile, to apply it at the next
// launch.
func saveProfiles(profiles []Profile, appliedID string) (string, bool) {
	s := config
	s.Profiles, s.ActiveID = tidy(profiles), appliedID
	if err := saveConfig(s); err != nil {
		return "Not saved: " + err.Error(), false
	}
	config = s
	return "Saved.", true
}

// goingFolder is the folder, as saved, that goes with profile id when the
// window deletes it, or "" if none goes: a folder that cannot be a profile's
// stays, as do a whole disk and a folder another profile uses.
func goingFolder(id string) string {
	for _, p := range config.Profiles {
		if p.ID != id {
			continue
		}
		dir := p.Folder()
		if unfit(dir) || diskRoot(dir) || slices.ContainsFunc(config.Profiles, func(q Profile) bool {
			f := q.Folder()
			return q.ID != id && !unfit(f) && (within(f, dir) || within(dir, f))
		}) {
			return ""
		}
		return dir
	}
	return ""
}

// deleteNote is what the window says goes with a profile it is about to delete.
func deleteNote(id string) string {
	if dir := goingFolder(id); dir != "" {
		return "Its token is removed from this app, and its folder is deleted with everything in it:\n" + tilde(dir)
	}
	return "Its token is removed from this app."
}

// deleteFolder deletes the folder of a profile the window has just deleted,
// and says why not if it could not.
func deleteFolder(id string) string {
	if dir := goingFolder(id); dir != "" {
		if err := removeFolder(dir); err != nil {
			return tilde(dir) + " could not be deleted: " + err.Error() + "."
		}
	}
	return ""
}

// applyProfile switches Claude Code to the profile as it stands on screen and
// writes nothing down.
func applyProfile(profiles []Profile, id string) (string, bool) {
	return apply(Settings{Profiles: tidy(profiles), ActiveID: id})
}

// testToken asks Haiku for one word with the token on screen, saved or not, in a
// config folder of its own, thrown away after, so ~/.claude cannot interfere.
func testToken(token string) (string, bool) {
	token = strings.Join(strings.Fields(token), "")
	if token == "" {
		return "No OAuth token in this profile.", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd, why := claudeCommand(ctx, "-p", "Reply with one word: ready", "--model", "haiku")
	if cmd == nil {
		return why, false
	}
	dir, err := os.MkdirTemp("", "CCTokenManager-test-")
	if err != nil {
		return "Could not start claude: " + err.Error(), false
	}
	defer os.RemoveAll(dir)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	cmd.Env = append(cmd.Env,
		"CLAUDE_CONFIG_DIR="+dir,
		tokenVar+"="+token,
		// Each of these would outrank the token or send it elsewhere.
		"ANTHROPIC_API_KEY=", "ANTHROPIC_AUTH_TOKEN=", "ANTHROPIC_BASE_URL=")
	out, err := cmd.CombinedOutput()
	reply := strings.TrimSpace(string(out))
	if err != nil {
		if reply == "" {
			reply = err.Error()
		}
		return "Error: " + reply, false
	}
	return fmt.Sprintf("Connected. Model haiku replied: %q", reply), true
}
