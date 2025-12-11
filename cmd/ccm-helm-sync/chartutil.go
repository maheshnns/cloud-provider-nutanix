package main

import (
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
