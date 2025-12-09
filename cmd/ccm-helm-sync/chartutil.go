package main

import (
	"fmt"
	"os"
	"strings"
)

func UpdateYamlVersion(path, version string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "version:") {
			lines[i] = "version: " + strings.TrimPrefix(version, "v")
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func UpdateImageTag(path, tag string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if strings.Contains(l, "image:") {
			parts := strings.SplitN(l, ":", 2)
			if len(parts) == 2 {
				before := parts[0] + ":"
				after := strings.TrimSpace(parts[1])
				if idx := strings.LastIndex(after, ":"); idx != -1 {
					after = after[:idx+1] + strings.TrimPrefix(tag, "v")
				} else {
					after = after + ":" + strings.TrimPrefix(tag, "v")
				}
				lines[i] = before + " " + after
			}
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func PrependChangelog(path, tag, notes string) error {
	entry := fmt.Sprintf("## %s\n\n%s\n\n", tag, notes)
	old, _ := os.ReadFile(path)
	return os.WriteFile(path, []byte(entry+string(old)), 0644)
}
