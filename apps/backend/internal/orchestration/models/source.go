package models

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Trackers a task can be sourced from.
const (
	TrackerJira   = "jira"
	TrackerLinear = "linear"
)

// Task metadata keys written by the Jira and Linear issue watches. Branch
// naming and tracker write-back read the same keys.
const (
	MetaJiraIssueKey          = "jira_issue_key"
	MetaJiraIssueURL          = "jira_issue_url"
	MetaLinearIssueIdentifier = "linear_issue_identifier"
	MetaLinearIssueURL        = "linear_issue_url"
)

// Source issue states a coordinator may move an issue to.
const (
	SourceStateStarted = "started"
	SourceStateReview  = "review"
	SourceStateDone    = "done"
)

// SourceCommentMaxBytes bounds one tracker comment.
const SourceCommentMaxBytes = 4000

var issueKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-[0-9]+$`)

// SourceIssue identifies the tracker issue a task was created from.
type SourceIssue struct {
	Tracker string `json:"tracker"`
	Key     string `json:"key"`
	URL     string `json:"url,omitempty"`
}

// TaskPullRequest is the pull request most relevant to a task.
type TaskPullRequest struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	State  string `json:"state"`
}

// TaskSourceIssue reads a task's source issue from its metadata, or nil.
func TaskSourceIssue(metadata map[string]any) *SourceIssue {
	text := func(key string) string {
		value, _ := metadata[key].(string)
		return strings.TrimSpace(value)
	}
	if key := text(MetaJiraIssueKey); key != "" {
		return &SourceIssue{Tracker: TrackerJira, Key: key, URL: safeIssueURL(text(MetaJiraIssueURL))}
	}
	if key := text(MetaLinearIssueIdentifier); key != "" {
		return &SourceIssue{Tracker: TrackerLinear, Key: key, URL: safeIssueURL(text(MetaLinearIssueURL))}
	}
	return nil
}

// NormalizeSourceIssue validates a caller-supplied source issue.
func NormalizeSourceIssue(source SourceIssue) (SourceIssue, error) {
	source.Tracker = strings.ToLower(strings.TrimSpace(source.Tracker))
	source.Key = strings.ToUpper(strings.TrimSpace(source.Key))
	if source.Tracker != TrackerJira && source.Tracker != TrackerLinear {
		return SourceIssue{}, errors.New("source.tracker must be jira or linear")
	}
	if !issueKeyPattern.MatchString(source.Key) {
		return SourceIssue{}, fmt.Errorf("source.key must look like ABC-123")
	}
	raw := strings.TrimSpace(source.URL)
	source.URL = safeIssueURL(raw)
	if raw != "" && source.URL == "" {
		return SourceIssue{}, errors.New("source.url must be an https URL")
	}
	return source, nil
}

// MetadataKey is the task metadata key that holds the issue key.
func (s SourceIssue) MetadataKey() string {
	if s.Tracker == TrackerLinear {
		return MetaLinearIssueIdentifier
	}
	return MetaJiraIssueKey
}

// Metadata is the task metadata that records the issue.
func (s SourceIssue) Metadata() map[string]any {
	metadata := map[string]any{s.MetadataKey(): s.Key}
	if s.URL != "" {
		urlKey := MetaJiraIssueURL
		if s.Tracker == TrackerLinear {
			urlKey = MetaLinearIssueURL
		}
		metadata[urlKey] = s.URL
	}
	return metadata
}

// ExternalID is the task create-idempotency identity for the issue.
func (s SourceIssue) ExternalID() string {
	return s.Tracker + ":" + s.Key
}

func safeIssueURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}

// SourceIssueUpdate is a coordinator's write-back to a task's source issue.
type SourceIssueUpdate struct {
	Comment string `json:"comment"`
	State   string `json:"state"`
}

// SourceIssueResult reports what a write-back changed.
type SourceIssueResult struct {
	Source    SourceIssue `json:"source"`
	Commented bool        `json:"commented"`
	State     string      `json:"state,omitempty"`
	Unchanged bool        `json:"state_unchanged,omitempty"`
}

// ErrNoSourceIssue means the task records no tracker issue.
var ErrNoSourceIssue = errors.New("task has no source issue")

// SourceStateError means the tracker offers no way to reach the requested
// state from the issue's current one.
type SourceStateError struct {
	State     string
	Available []string
}

func (e *SourceStateError) Error() string {
	return fmt.Sprintf("no transition reaches %s; available: %s", e.State, strings.Join(e.Available, ", "))
}

// DuplicateTaskError means a task for the same source issue already exists.
type DuplicateTaskError struct {
	TaskID   string
	Archived bool
}

func (e *DuplicateTaskError) Error() string {
	return "a task for this source issue already exists: " + e.TaskID
}
