// cmd/ccm-helm-sync/main.go
// This tool automates the creation of a CCM Helm chart and changelog after a new CCM release.
// Usage: go run ./cmd/ccm-helm-sync --ccm-repo nutanix-cloud-native/cloud-provider-nutanix --helm-repo maheshnns/helm --token $GITHUB_TOKEN

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
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
	flag.StringVar(&releaseNotesFlag, "release-notes", "", "Provide release notes text to avoid requiring release-lookup (requires --ccm-tag)")
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
	if err := createPR(helmRepo, tag, dryRun, testMode, ccmRepo, releaseNotes, token); err != nil {
		log.Fatalf("Failed to create PR: %v", err)
	}
}

// normalizeCloneURL accepts either an owner/repo (e.g. "maheshnns/helm") or
// a full https URL (with or without .git) and returns a clone URL that embeds
// the token for HTTPS authentication when appropriate.
func normalizeCloneURL(repo, token string) string {
	// If repo looks like an HTTP(S) URL, inject token into it
	if strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "https://") {
		hasGit := strings.HasSuffix(repo, ".git")
		schemeSep := "://"
		parts := strings.SplitN(repo, schemeSep, 2)
		if len(parts) != 2 {
			return repo
		}
		hostAndPath := parts[1]
		// If the URL already contains credentials (user@), avoid injecting
		if strings.Contains(hostAndPath, "@") {
			return repo
		}
		if !hasGit {
			hostAndPath = hostAndPath + ".git"
		}
		return fmt.Sprintf("https://%s@%s", token, hostAndPath)
	}

	// If repo starts with github.com/, strip that prefix
	r := repo
	r = strings.TrimPrefix(r, "github.com/")

	// Now treat as owner/repo form
	if !strings.HasSuffix(r, ".git") {
		r = r + ".git"
	}
	return fmt.Sprintf("https://%s@github.com/%s", token, r)
}

func getReleaseNotesForTag(repo, tag, token string) (string, error) {
	owner, name := parseOwnerRepo(repo)
	if owner == "" || name == "" {
		return "", fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, name, tag)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "token "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return "", fmt.Errorf("release tag not found: %s", tag)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to fetch release: status %d body: %s", resp.StatusCode, string(b))
	}
	var data struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	return strings.TrimSpace(data.Body), nil
}

func getLatestReleaseTagAndNotes(repo, token string) (string, string, error) {
	owner, name := parseOwnerRepo(repo)
	if owner == "" || name == "" {
		return "", "", fmt.Errorf("invalid repo: %s", repo)
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, name)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "token "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("failed to fetch latest release: status %d body: %s", resp.StatusCode, string(b))
	}
	var data struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(data.TagName), strings.TrimSpace(data.Body), nil
}

func cloneHelmRepo(repo, token string) error {
	url := normalizeCloneURL(repo, token)
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

func createPR(repo, tag string, dryRun, testMode bool, sourceRepo, releaseNotes, token string) error {
	// Use a unique branch name per run to avoid non-fast-forward failures
	// Example: test/ccm-automation-20251209-150405-v0.6.0
	ts := time.Now().UTC().Format("20060102-150405")
	safeTag := strings.TrimPrefix(tag, "v")
	branch := fmt.Sprintf("test/ccm-automation-%s-%s", ts, safeTag)
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
		fmt.Printf("  curl -X POST -H 'Authorization: token <TOKEN>' -H 'Accept: application/vnd.github+json' https://api.github.com/repos/<OWNER>/<REPO>/pulls -d '{\"title\": \"%s\", \"head\": \"%s\", \"base\": \"main\", \"body\": \"%s\"}'\n", prTitle, branch, prBody)
		fmt.Println("\nTo remove any local test branch run:")
		fmt.Printf("  git -C helm-repo branch -D %s\n", branch)
		return nil
	}

	if err := GitPush(branch, "helm-repo"); err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	if err := CreatePR(prTitle, prBody, "helm-repo", token, repo, branch); err != nil {
		return fmt.Errorf("create PR via API: %w", err)
	}

	fmt.Println("\nPR created. To remove the test PR and branch after testing, run:")
	fmt.Printf("  gh pr close <PR_NUMBER> --repo %s\n", repo)
	fmt.Printf("  git push origin --delete %s\n", branch)
	return nil
}
