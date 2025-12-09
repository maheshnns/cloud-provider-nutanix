// cmd/ccm-helm-sync/main.go
// This tool automates the creation of a CCM Helm chart and changelog after a new CCM release.
// Usage: go run main.go --ccm-repo nutanix-cloud-native/cloud-provider-nutanix --helm-repo nutanix/helm --token $GITHUB_TOKEN

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

func main() {
	var ccmRepo, helmRepo, token, ccmTag string
	var dryRun bool
	var releaseNotesFlag string
	var testMode bool
	flag.StringVar(&ccmRepo, "ccm-repo", "nutanix-cloud-native/cloud-provider-nutanix", "CCM GitHub repo")
	flag.StringVar(&helmRepo, "helm-repo", "nutanix/helm", "Helm chart GitHub repo")
	flag.StringVar(&token, "token", os.Getenv("GITHUB_TOKEN"), "GitHub token")
	flag.StringVar(&ccmTag, "ccm-tag", "", "CCM release tag (optional, defaults to latest)")
	flag.BoolVar(&dryRun, "dry-run", false, "Run in dry-run mode: do not push branches or create PRs")
	flag.StringVar(&releaseNotesFlag, "release-notes", "", "Provide release notes text to avoid requiring gh (requires --ccm-tag)")
	flag.BoolVar(&testMode, "test", false, "Mark the run as a TEST; PRs and commits will be prefixed and include test metadata")
	flag.Parse()

	if token == "" {
		log.Fatal("GitHub token required via --token or GITHUB_TOKEN env var")
	}

	var tag, releaseNotes string
	var err error
	if releaseNotesFlag != "" {
		if ccmTag == "" {
			log.Fatal("When --release-notes is provided you must also pass --ccm-tag")
		}
		tag = ccmTag
		releaseNotes = releaseNotesFlag
	} else if ccmTag != "" {
		// Get release notes for the provided tag
		tag = ccmTag
		releaseNotes, err = getReleaseNotesForTag(ccmRepo, tag, token)
		if err != nil {
			log.Fatalf("Failed to get release notes for tag %s: %v", tag, err)
		}
	} else {
		// Get latest release
		tag, releaseNotes, err = getLatestReleaseTagAndNotes(ccmRepo, token)
		if err != nil {
			log.Fatalf("Failed to get latest CCM release: %v", err)
		}
	}
	fmt.Println("Using CCM release:", tag)

	// 2. Clone helm repo
	if err := cloneHelmRepo(helmRepo, token); err != nil {
		log.Fatalf("Failed to clone helm repo: %v", err)
	}

	// 3. Update chart and changelog
	if err := updateChartAndChangelog(tag, releaseNotes); err != nil {
		log.Fatalf("Failed to update chart/changelog: %v", err)
	}

	// 4. Commit, push, and create PR (or skip push/PR in dry-run)
	if err := createPR(helmRepo, tag, dryRun, testMode, ccmRepo, releaseNotes); err != nil {
		log.Fatalf("Failed to create PR: %v", err)
	}
}

func getReleaseNotesForTag(repo, tag, token string) (string, error) {
	cmd := exec.Command("gh", "release", "view", tag, "--repo", repo, "--json", "body", "--jq", ".body")
	cmd.Env = append(os.Environ(), "GITHUB_TOKEN="+token)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getLatestReleaseTagAndNotes(repo, token string) (string, string, error) {
	// Get tag
	tagCmd := exec.Command("gh", "release", "view", "--repo", repo, "--json", "tagName,body", "--jq", ".tagName + '\n' + .body")
	tagCmd.Env = append(os.Environ(), "GITHUB_TOKEN="+token)
	out, err := tagCmd.Output()
	if err != nil {
		return "", "", err
	}
	lines := strings.SplitN(string(out), "\n", 2)
	if len(lines) < 2 {
		return strings.TrimSpace(string(out)), "", nil
	}
	return strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1]), nil
}

func cloneHelmRepo(repo, token string) error {
	url := fmt.Sprintf("https://%s@github.com/%s.git", token, repo)
	cmd := exec.Command("git", "clone", url, "helm-repo")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func updateChartAndChangelog(tag, releaseNotes string) error {
	chartDir := "helm-repo/charts/cloud-provider-nutanix"
	chartYaml := chartDir + "/Chart.yaml"
	valuesYaml := chartDir + "/values.yaml"
	deploymentYaml := chartDir + "/templates/deployment.yaml"
	changelog := chartDir + "/CHANGELOG.md"

	if err := UpdateYamlVersion(chartYaml, tag); err != nil {
		return fmt.Errorf("update Chart.yaml: %w", err)
	}
	if err := UpdateImageTag(valuesYaml, tag); err != nil {
		return fmt.Errorf("update values.yaml: %w", err)
	}
	_ = UpdateImageTag(deploymentYaml, tag)
	if err := PrependChangelog(changelog, tag, releaseNotes); err != nil {
		return fmt.Errorf("update changelog: %w", err)
	}
	return nil
}

func createPR(repo, tag string, dryRun, testMode bool, sourceRepo, releaseNotes string) error {
	branch := "test/ccm-automation-dry-run-" + tag
	chartDir := "charts/cloud-provider-nutanix"
	if err := GitBranch(branch, "helm-repo"); err != nil {
		return fmt.Errorf("git checkout -b: %w", err)
	}
	if err := GitAdd(chartDir, "helm-repo"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Commit with optional TEST prefix
	commitPrefix := ""
	if testMode {
		commitPrefix = "[TEST] "
	}
	commitMsg := fmt.Sprintf("%scloud-provider-nutanix: update chart for CCM %s", commitPrefix, tag)
	if err := GitCommit(commitMsg, "helm-repo"); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	// Prepare PR title and body with test metadata when enabled
	prPrefix := ""
	if testMode {
		prPrefix = "[TEST] "
	}
	prTitle := fmt.Sprintf("%scloud-provider-nutanix: update chart for CCM %s", prPrefix, tag)
	prBody := fmt.Sprintf("%sAutomated update of CCM Helm chart and changelog.\n\nSource repo: %s\nCCM tag: %s\n\nRelease notes:\n%s\n\n/cc @nutanix-cloud-native/maintainers", prPrefix, sourceRepo, tag, releaseNotes)

	if dryRun {
		fmt.Println("Dry-run mode enabled: skipping push and PR creation.")
		fmt.Println("Would run:")
		fmt.Printf("  git -C helm-repo push origin %s\n", branch)
		fmt.Printf("  gh pr create --title \"%s\" --body \"%s\" --repo %s\n", prTitle, prBody, repo)
		fmt.Println("\nTo remove any local test branch run:")
		fmt.Printf("  git -C helm-repo branch -D %s\n", branch)
		return nil
	}

	if err := GitPush(branch, "helm-repo"); err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	if err := CreatePR(prTitle, prBody, "helm-repo"); err != nil {
		return fmt.Errorf("gh pr create: %w", err)
	}

	fmt.Println("\nPR created. To remove the test PR and branch after testing, run:")
	fmt.Printf("  gh pr close <PR_NUMBER> --repo %s\n", repo)
	fmt.Printf("  git push origin --delete %s\n", branch)
	return nil
}
