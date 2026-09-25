package main

// On Windows the token goes into the credential Claude Code-credentials/
// claude-code-user, where the CLI looks when loginEnv is in the settings.json;
// otherwise it keeps its login in plain text, in .credentials.json.

import "fmt"

// cliAccount is the name the CLI keeps its login under.
const cliAccount = "claude-code-user"

// loginEnv is what the env block of the applied profile's settings.json holds
// for the CLI to find the login the app gives it: the CLI's own switch to
// Credential Manager, which is no secret.
var loginEnv = map[string]any{"CLAUDE_CODE_FORCE_WINDOWS_CREDMAN": "1"}

// cliLogin is what the CLI's credential holds, and whether there is one.
func cliLogin() (string, bool, error) {
	login, err := loadSecret(cliService, cliAccount)
	if err != nil {
		return "", false, fmt.Errorf("Credential Manager did not give Claude Code's login: %w", err)
	}
	return login, login != "", nil
}

// putLogin writes a login into the CLI's credential.
func putLogin(login string) error {
	if err := storeSecret(cliService, cliAccount, login); err != nil {
		return fmt.Errorf("Credential Manager did not take the login for Claude Code: %w", err)
	}
	return nil
}

// dropLogin removes the CLI's credential.
func dropLogin() { storeSecret(cliService, cliAccount, "") }
