// Package credentiallock clears the account lock a Claude process leaves
// beside its config directory when it is killed while refreshing the login.
package credentiallock

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// StaleAge is how old an empty lock must be before it is removed. Claude
// Code takes its OAuth refresh lock with proper-lockfile options
// {stale: 60000, update: 5000}: a live holder touches the lock every five
// seconds and Claude itself treats a lock untouched for a minute as stale.
// Matching that minute never removes a lock a live refresh still holds.
const StaleAge = time.Minute

// ConfigDirEnv names the account's config directory in an agent profile.
const ConfigDirEnv = "CLAUDE_CONFIG_DIR"

// loginFailureMarkers identify a session that stopped because its provider
// could not refresh or use the account login.
var loginFailureMarkers = []string{
	"failed to refresh oauth token",
	"oauth token has expired",
	"authentication_error",
}

// IsLoginFailure reports whether an agent error is a provider login failure
// that clearing a stale lock and resuming can repair.
func IsLoginFailure(message string) bool {
	message = strings.ToLower(message)
	for _, marker := range loginFailureMarkers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// ProfileReader loads the agent profile that selects the account.
type ProfileReader interface {
	GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error)
}

// ConfigDir returns the account config directory a profile runs with: its
// absolute CLAUDE_CONFIG_DIR, otherwise the default ~/.claude.
func ConfigDir(profile *settingsmodels.AgentProfile) string {
	if profile == nil {
		return ""
	}
	for _, env := range profile.EnvVars {
		if env.Key == ConfigDirEnv && filepath.IsAbs(env.Value) {
			return env.Value
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// ClearStale removes configDir + ".lock" only when it is an empty directory
// older than StaleAge. It reports whether a lock was removed; a missing,
// fresh or non-empty lock is left alone and is not an error.
func ClearStale(configDir string) (bool, error) {
	if configDir == "" {
		return false, nil
	}
	lock := filepath.Clean(configDir) + ".lock"
	info, err := os.Stat(lock)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil || !info.IsDir() || time.Since(info.ModTime()) < StaleAge {
		return false, err
	}
	entries, err := os.ReadDir(lock)
	if err != nil || len(entries) != 0 {
		return false, err
	}
	// Remove refuses a directory that gained an entry since ReadDir.
	if err := os.Remove(lock); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ClearForProfile clears a stale lock for the account the profile uses.
func ClearForProfile(ctx context.Context, profiles ProfileReader, profileID string) (bool, error) {
	if profiles == nil || profileID == "" {
		return false, nil
	}
	profile, err := profiles.GetAgentProfile(ctx, profileID)
	if err != nil || profile == nil {
		return false, err
	}
	return ClearStale(ConfigDir(profile))
}
