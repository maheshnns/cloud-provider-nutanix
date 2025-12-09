package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// RunCmd runs a shell command in a given directory.
func RunCmd(cmd string, args []string, dir string) error {
	c := exec.Command(cmd, args...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// GitBranch creates a new branch.
func GitBranch(branch, dir string) error {
	return RunCmd("git", []string{"checkout", "-b", branch}, dir)
}

// GitAdd stages files.
func GitAdd(path, dir string) error {
	return RunCmd("git", []string{"add", path}, dir)
}

// GitCommit commits changes.
func GitCommit(msg, dir string) error {
	return RunCmd("git", []string{"commit", "-m", msg}, dir)
}

// GitPush pushes a branch.
func GitPush(branch, dir string) error {
	return RunCmd("git", []string{"push", "origin", branch}, dir)
}

// parseOwnerRepo accepts either an owner/repo or a full URL and returns owner and repo name.
func parseOwnerRepo(repo string) (string, string) {
	// Trim any trailing .git
	r := strings.TrimSuffix(repo, ".git")
	// If it's a URL, strip scheme and host
	if strings.HasPrefix(r, "http://") || strings.HasPrefix(r, "https://") {
		parts := strings.SplitN(r, "://", 2)
		if len(parts) == 2 {
			r = parts[1]
		}
		// strip possible leading github.com/
		r = strings.TrimPrefix(r, "github.com/")
	}
	// If it contains github.com/owner/repo style, ensure we only have owner/repo
	if strings.Contains(r, "github.com/") {
		r = strings.SplitN(r, "github.com/", 2)[1]
	}
	// Now split on /
	parts := strings.SplitN(r, "/", 2)
	if len(parts) < 2 {
		return "", ""
	}
	owner := parts[0]
	name := parts[1]
	return owner, name
}

// CreatePR creates a pull request using the GitHub REST API (avoids requiring gh CLI).
// repo may be owner/repo or a full HTTPS URL. branch is the branch name pushed to the repo.
func CreatePR(title, body, dir, token, repo, branch string) error {
	owner, name := parseOwnerRepo(repo)
	if owner == "" || name == "" {
		return fmt.Errorf("invalid repo format: %s", repo)
	}

	payload := map[string]string{
		"title": title,
		"head":  branch,
		"base":  "main",
		"body":  body,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", owner, name)
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("create PR failed: status %d response: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
