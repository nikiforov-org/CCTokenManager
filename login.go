//go:build !linux

package main

// The applied profile's token goes where the CLI keeps its own login, as one
// lasting ten years, so the CLI never tries to renew it. A login of the user's
// found there waits beside the profiles' tokens until the profile is taken back.

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	// cliService is what the CLI keeps its login under.
	cliService = "Claude Code-credentials"
	// stashID is where the user's own login waits, beside the profiles' tokens.
	stashID = "claude-code-login"
)

// given is the token the app last put where the CLI keeps its login.
var given string

// ours reports whether a login the CLI keeps is one the app put there.
func ours(login string) bool {
	var l struct {
		ClaudeAiOauth struct{ AccessToken string } `json:"claudeAiOauth"`
	}
	json.Unmarshal([]byte(login), &l)
	tok := l.ClaudeAiOauth.AccessToken
	if tok == "" {
		return false
	}
	for _, p := range last.Profiles {
		if tok == p.Token {
			return true
		}
	}
	return tok == given
}

// giveToken puts the token where the CLI keeps its login, after setting aside a
// login of the user's own found there.
func giveToken(token string) error {
	login, found, err := cliLogin()
	if err != nil {
		return err
	}
	if found && !ours(login) {
		if err := storeToken(stashID, login); err != nil {
			return fmt.Errorf("Claude Code's own login could not be set aside: %w", err)
		}
	}
	data, _ := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{
		"accessToken":  token,
		"refreshToken": nil,
		"expiresAt":    time.Now().AddDate(10, 0, 0).UnixMilli(),
		"scopes":       []string{"user:inference"},
	}})
	if err := putLogin(string(data)); err != nil {
		return err
	}
	given = token
	return nil
}

// takeToken takes the app's token from where the CLI keeps its login, and gives
// back the login set aside, if any. A login the user made meanwhile is left
// alone.
func takeToken() {
	defer func() { given = "" }()
	login, found, err := cliLogin()
	if err != nil || found && !ours(login) {
		return
	}
	if own, _ := loadToken(stashID); own != "" {
		if putLogin(own) == nil {
			storeToken(stashID, "")
		}
	} else if found {
		dropLogin()
	}
}
