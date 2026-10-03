#!/usr/bin/env bash
# ci-gate refuses a production release of a commit unless both the CI and the
# E2E workflow have a successful full run on it. A full run is a dispatched
# run, or a pull request run into release; release lands that pull request by
# fast-forward, so the run's head commit is the released commit.
#
# Usage: GH_TOKEN=... ci-gate.sh <commit>
set -euo pipefail

commit="$1"
for workflow in ci.yml e2e.yml; do
  runs=$(gh api --paginate \
    "repos/${GITHUB_REPOSITORY}/actions/workflows/${workflow}/runs?head_sha=${commit}&status=success&per_page=100" \
    --jq '.workflow_runs[]
      | select(.event == "workflow_dispatch"
        or (.event == "pull_request" and any(.pull_requests[]; .base.ref == "release")))
      | .html_url')
  if [[ -z "${runs}" ]]; then
    echo "No successful full ${workflow} run for ${commit}. Open a pull request" >&2
    echo "into release and let its checks pass, or dispatch again with" >&2
    echo "skip_ci_gate to deploy anyway." >&2
    exit 1
  fi
  echo "Full ${workflow} passed for ${commit}:"
  echo "${runs}"
done
