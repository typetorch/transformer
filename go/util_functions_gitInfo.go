// Port of src/util/functions/gitInfo.ts.

package transformer

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GitInfo is what `$git()` compiles in. Empty strings (and the current time) when the project is not a git
// repository.
type GitInfo struct {
	Branch    string
	Commit    string
	LatestTag string
	ISODate   string
	Timestamp float64
}

// gitRunner runs git in cwd and returns its trimmed output; ok is false when git failed.
type gitRunner func(cwd string, args ...string) (output string, ok bool)

func runGit(cwd string, args ...string) (string, bool) {
	command := exec.Command("git", args...)
	command.Dir = cwd
	output, err := command.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(output)), true
}

var commitRegex = regexp.MustCompile(`^[0-9a-f]{40}$`)
var digitsRegex = regexp.MustCompile(`^\d+$`)

// readGitInfo reads the repository `cwd` is in (three git calls; the caller caches the result per compile).
func readGitInfo(cwd string, git gitRunner, now time.Time) GitInfo {
	var head []string
	if output, ok := git(cwd, "log", "-1", "--format=%H%n%aI%n%at"); ok {
		head = strings.Split(output, "\n")
	}
	at := func(i int) string {
		if i < len(head) {
			return head[i]
		}
		return ""
	}
	commit := ""
	if commitRegex.MatchString(at(0)) {
		commit = at(0)
	}
	timestamp := float64(now.UnixMilli()) / 1000
	timestamp = float64(int64(timestamp + 0.5)) // Math.round
	if commit != "" && digitsRegex.MatchString(at(2)) {
		timestamp, _ = strconv.ParseFloat(at(2), 64)
	}
	isoDate := toISOString(now)
	if commit != "" && at(1) != "" {
		if date, err := time.Parse(time.RFC3339, at(1)); err == nil {
			isoDate = toISOString(date)
		}
	}
	// A detached HEAD prints "HEAD"; rbxts-transform-debug compiled "" in for it.
	branch, latestTag := "", ""
	if commit != "" {
		branch, _ = git(cwd, "rev-parse", "--abbrev-ref", "HEAD")
		latestTag, _ = git(cwd, "describe", "--tags", "--abbrev=0")
	}
	if branch == "HEAD" {
		branch = ""
	}
	return GitInfo{Branch: branch, Commit: commit, LatestTag: latestTag, ISODate: isoDate, Timestamp: timestamp}
}

// toISOString is JavaScript's Date.prototype.toISOString.
func toISOString(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}
