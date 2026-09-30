package main

// Applying a profile: ~/.claude, ~/.claude.json and the CLI's cache folder stand
// for its folder, the state file and a cache folder in it, the theme, the place
// for the CLI's temporary files and the guards go into its settings.json and the
// token where the CLI keeps its login. What stood there waits as .default; a
// quit undoes it.

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// tmpEnv is the CLI's setting for where its temporary files go.
const tmpEnv = "CLAUDE_CODE_TMPDIR"

var (
	claudeDir  = filepath.Join(home, ".claude")
	claudeJSON = filepath.Join(home, ".claude.json")

	// mu keeps an apply, which the window runs on a thread of its own, apart
	// from the revert on quit.
	mu sync.Mutex
	// last is what the latest apply was given, and linked the folder ~/.claude
	// was made to stand for: through them a revert finds everything the app
	// handed out, saved or not.
	last   Settings
	linked string
)

// apply switches the CLI to the active profile and says how it went.
func apply(s Settings) (string, bool) {
	mu.Lock()
	defer mu.Unlock()
	last = s
	p := s.Active()
	name := "The profile"
	if p.Name != "" {
		name = "Profile " + p.Name
	}
	dir := p.Folder()
	var err error
	switch {
	case p.Token == "":
		err = errors.New("this profile has no OAuth token")
	case unfit(dir):
		err = fmt.Errorf("%s cannot be its folder", tilde(dir))
	default:
		takeBack(dir)
		// The CLI's temporary files go into the profile's folder too; the CLI
		// takes only a folder no one else can open.
		tmp := filepath.Join(dir, "tmp")
		if err = os.MkdirAll(tmp, 0o700); err == nil {
			err = os.Chmod(tmp, 0o700)
		}
		if err != nil {
			break
		}
		err = edit(filepath.Join(dir, "settings.json"), func(m map[string]any) bool {
			env, _ := m["env"].(map[string]any)
			if env == nil {
				env = map[string]any{}
			}
			// The token goes where the CLI keeps its login (see giveToken), and
			// loginEnv has the CLI look there.
			maps.Copy(env, loginEnv)
			env[tmpEnv] = tmp
			// The guards as they stand now, not on top of an earlier set.
			takeGuards(m, env)
			putGuards(m, env, p.Guards)
			m["env"] = env
			if len(env) == 0 {
				delete(m, "env")
			}
			m["theme"] = p.Theme
			return true
		})
		if err == nil {
			err = standIn(dir)
		}
		if err == nil {
			err = giveToken(p.Token)
		}
	}
	if err != nil {
		revert()
		return name + " is not applied: " + err.Error() + ".", false
	}
	return name + " is applied: Claude Code works with " + tilde(dir) + ". " +
		"It works only in Claude Code sessions started from now on.", true
}

// revert takes back every token the app handed out and puts the CLI's own
// ~/.claude and ~/.claude.json back in their places.
func revert() {
	takeToken()
	takeBack("")
	standDown()
	linked = ""
}

// quit reverts for good as the app goes: the lock is kept, so nothing is
// applied after it.
func quit() {
	mu.Lock()
	revert()
}

// unfit reports whether dir cannot be a profile's folder: ~/.claude or the
// cache folder would stand for itself or for a folder holding it, and
// ~/.CCTokenManager holds the profiles.
func unfit(dir string) bool {
	return !filepath.IsAbs(dir) || within(profilesDir, dir) || within(dir, claudeDir) ||
		within(dir, claudeDir+".default") || within(claudeDir, dir) ||
		within(dir, cacheDir) || within(dir, cacheDir+".default")
}

// removeProfile deletes dir, the folder of profile id, which is gone, unless
// dir is "". If Claude Code works with the profile, or ~/.claude stands for a
// folder in dir, it is taken back first, as a quit does, so that nothing is
// left handing out a token that is gone or pointing into a folder that is not
// there.
func removeProfile(id, dir string) error {
	mu.Lock()
	defer mu.Unlock()
	if linked != "" && (last.Active().ID == id || dir != "" && within(linked, dir)) {
		revert()
	}
	if dir == "" {
		return nil
	}
	return os.RemoveAll(dir)
}

// markOnboarded writes into a state file what the CLI writes once its
// first-launch setup is done. Until it finds it, the CLI runs the setup at
// every start.
func markOnboarded(state string) error {
	return edit(state, func(m map[string]any) bool {
		done := m["hasCompletedOnboarding"] == true
		m["hasCompletedOnboarding"] = true
		return !done
	})
}

// takeBack takes the theme and loginEnv out of the settings.json of the folder
// ~/.claude stands for, or stood for when the app was killed, unless it is keep.
func takeBack(keep string) {
	target, _ := os.Readlink(claudeDir)
	for _, dir := range folders() {
		if dir == keep || dir != linked && !(target != "" && within(target, dir) && within(dir, target)) {
			continue
		}
		edit(filepath.Join(dir, "settings.json"), func(m map[string]any) bool {
			env, _ := m["env"].(map[string]any)
			_, themed := m["theme"]
			delete(m, "theme")
			n := len(env)
			for k := range loginEnv {
				delete(env, k)
			}
			delete(env, tmpEnv)
			guarded := takeGuards(m, env)
			if env != nil && len(env) == 0 {
				delete(m, "env")
			}
			return themed || guarded || len(env) < n
		})
	}
}

// folders is every folder that may hold what the app handed out.
func folders() []string {
	var dirs []string
	if linked != "" {
		dirs = append(dirs, linked)
	}
	for _, p := range last.Profiles {
		dirs = append(dirs, p.Folder())
	}
	return dirs
}

// edit changes a JSON file in place and keeps whatever it does not touch. It
// writes only if change says it changed something, with mode 0600: the files
// are the user's own.
func edit(path string, change func(map[string]any) bool) error {
	m := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &m) != nil {
			return fmt.Errorf("%s is not valid JSON", tilde(path))
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if m == nil { // the file said null
		m = map[string]any{}
	}
	if !change(m) {
		return nil
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600) // WriteFile keeps the mode of a file already there
}

// ourLink tells a link the app made: one pointing into a folder it knows.
func ourLink(path string) bool {
	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	for _, d := range append(folders(), profilesDir) {
		if within(target, d) {
			return true
		}
	}
	return false
}

// aside moves what stood in path, which the app did not put there, to
// path.default, for standDown to bring back.
func aside(path string) error {
	if _, err := os.Lstat(path + ".default"); err == nil {
		return fmt.Errorf("%s and %s.default both exist, and one of them has to go",
			tilde(path), tilde(path))
	}
	return os.Rename(path, path+".default")
}

// within reports whether path is dir or lies inside it. Case does not count:
// neither system's disks tell names apart by it.
func within(path, dir string) bool {
	path, dir = strings.ToLower(filepath.Clean(path)), strings.ToLower(filepath.Clean(dir))
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// tilde shortens a path in the home folder the way the system shows it.
func tilde(path string) string {
	if within(path, home) {
		return homeShown + filepath.Clean(path)[len(filepath.Clean(home)):]
	}
	return path
}
