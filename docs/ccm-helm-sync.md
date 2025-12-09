# CCM Helm Chart Sync Automation

This repository now includes automation to create/update the CCM Helm chart and changelog in the nutanix/helm repo after a new CCM release.

## How it works
- A Go script (`cmd/ccm-helm-sync/main.go`) fetches the latest CCM release, updates the Helm chart and changelog, and creates a PR in the nutanix/helm repo.
- A GitHub Actions workflow (`.github/workflows/ccm-helm-sync.yml`) runs this script on demand (manual trigger) or after a new release is published.

## Usage

### Manual Trigger
Go to the Actions tab, select "CCM Helm Chart Sync", and click "Run workflow".

### On Release
The workflow also runs automatically when a new release is published in this repo.

## Required Secrets
- `CCM_HELM_SYNC_TOKEN`: A GitHub PAT with `repo` scope and permission to push branches and create PRs in the nutanix/helm repo.

## Implementation Notes
- The Go script uses the GitHub CLI (`gh`) and `git`.
- The script is scaffolded; you must implement the logic to update the Helm chart and changelog as needed.

## Extending
- Edit `cmd/ccm-helm-sync/main.go` to customize chart/changelog logic.
- The workflow can be extended to support more triggers or notifications.
