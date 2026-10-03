#!/usr/bin/env bash
# ci-gate refuses a production release of a commit without a successful full CI
# run. A full run is a scheduled or dispatched run, or a pull request run into
# release; release lands that pull request by fast-forward, so the run's head
# commit is the released commit.
#
# Usage: GH_TOKEN=... ci-gate.sh <commit>
set -euo pipefail

commit="$1"
runs=$(gh api --paginate \
  "repos/${GITHUB_REPOSITORY}/actions/workflows/ci.yml/runs?head_sha=${commit}&status=success&per_page=100" \
  --jq '.workflow_runs[]
    | select(.event == "schedule" or .event == "workflow_dispatch"
      or (.event == "pull_request" and any(.pull_requests[]; .base.ref == "release")))
    | .html_url')
if [[ -z "${runs}" ]]; then
  echo "No successful full CI run for ${commit}. Open a pull request into release" >&2
  echo "and let its CI pass, or dispatch again with skip_ci_gate to deploy anyway." >&2
  exit 1
fi
echo "Full CI passed for ${commit}:"
echo "${runs}"
