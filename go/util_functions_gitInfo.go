// Port of src/util/functions/gitInfo.ts.

package transformer

import (
	"os"
	"os/exec"
	"path/filepath"
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

// isLooseCommit is whether the commit's object is a loose file, not in a pack. rbxts-transform-debug read git with
// git-repo-info, which only reads loose objects: its ISODate is the author date for a loose commit, and the compile
// time for a packed one (a fresh clone, or after `git gc`). Games moving off it keep the value they had.
func isLooseCommit(cwd string, commit string, git gitRunner) bool {
	objectPath, ok := git(cwd, "rev-parse", "--git-path", "objects/"+commit[:2]+"/"+commit[2:])
	if !ok {
		return false
	}
	if !filepath.IsAbs(objectPath) {
		objectPath = filepath.Join(cwd, objectPath)
	}
	_, err := os.Stat(objectPath)
	return err == nil
}

// readGitInfo reads the repository `cwd` is in (four git calls; the caller caches the result per compile).
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
	if commit != "" && at(1) != "" && isLooseCommit(cwd, commit, git) {
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
