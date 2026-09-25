package main

// On macOS the CLI's login is the keychain item Claude Code-credentials, read and
// written with the security tool as the CLI does, so the CLI reads it without
// asking. What is written goes in on the tool's standard input, unseen by ps.

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
)

// loginEnv is what the env block of the applied profile's settings.json holds
// for the CLI to find the login the app gives it: nothing, on macOS.
var loginEnv map[string]any

// cliAccount is the account the CLI files its login under: the user's name,
// or a stand-in when the name has characters it will not use.
func cliAccount() string {
	name := os.Getenv("USER")
	if u, err := user.Current(); name == "" && err == nil {
		name = u.Username
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9._-]+$`).MatchString(name) {
		return "claude-code-user"
	}
	return name
}

// cliLogin is what the CLI's item holds, and whether there is one.
func cliLogin() (string, bool, error) {
	out, err := exec.Command("/usr/bin/security", "find-generic-password",
		"-a", cliAccount(), "-s", cliService, "-w").Output()
	if e, ok := err.(*exec.ExitError); ok && e.ExitCode() == 44 { // errSecItemNotFound
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("the keychain did not give Claude Code's login: %w", err)
	}
	login := strings.TrimSuffix(string(out), "\n")
	if b, err := hex.DecodeString(login); err == nil { // what is not text comes out as hex
		login = string(b)
	}
	return login, true, nil
}

// putLogin writes a login into the CLI's item.
func putLogin(login string) error {
	if err := security(fmt.Sprintf(`add-generic-password -U -a "%s" -s "%s" -X "%s"`,
		cliAccount(), cliService, hex.EncodeToString([]byte(login)))); err != nil {
		return fmt.Errorf("the keychain did not take the login for Claude Code: %w", err)
	}
	return nil
}

// dropLogin removes the CLI's item.
func dropLogin() {
	security(fmt.Sprintf(`delete-generic-password -a "%s" -s "%s"`, cliAccount(), cliService))
}

// security runs one command of the security tool, given on its standard input.
func security(command string) error {
	cmd := exec.Command("/usr/bin/security", "-i")
	cmd.Stdin = strings.NewReader(command + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(strings.Split(string(out), "\n")[0]))
	}
	return nil
}
