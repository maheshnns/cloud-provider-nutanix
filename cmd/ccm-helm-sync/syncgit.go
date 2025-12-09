package main

import (
	"os"
	"os/exec"
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

// CreatePR opens a pull request using gh CLI.
func CreatePR(title, body, dir string) error {
	return RunCmd("gh", []string{"pr", "create", "--title", title, "--body", body, "--base", "main"}, dir)
}

// GitPush pushes a branch.
