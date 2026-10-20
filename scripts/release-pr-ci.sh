#!/usr/bin/env bash
# Only the same-repository release-please bot PR may approve/dispatch ci.yml.
set -euo pipefail
: "${GH_REPO:?需要 GH_REPO}"
branch=release-please--branches--main
[[ "${BRANCH:-$branch}" == "$branch" ]] || { echo 'unexpected release branch' >&2; exit 1; }
tries=${POLL_TRIES:-6}
nap=${POLL_SLEEP:-10}
prs=$(gh api "repos/${GH_REPO}/pulls?state=open&base=main&head=${GH_REPO%%/*}:${branch}&per_page=100")
pr=$(jq -ce --arg repo "$GH_REPO" --arg branch "$branch" '[.[] | select(.user.login == "github-actions[bot]" and .user.type == "Bot" and .base.ref == "main" and .base.repo.full_name == $repo and .head.repo.full_name == $repo and .head.ref == $branch)] | if length == 1 then .[0] else empty end' <<<"$prs") || { echo 'No unique trusted release PR; no action'; exit 0; }
head=$(jq -r .head.sha <<<"$pr")
number=$(jq -r .number <<<"$pr")
workflow=$(gh api "repos/${GH_REPO}/actions/workflows/ci.yml" --jq .id)
for ((i=1; i<=tries; i++)); do
  # Query the producer, not names such as CI OK or unrelated AI Review runs.
  runs=$(gh api --paginate --slurp "repos/${GH_REPO}/actions/workflows/${workflow}/runs?head_sha=${head}&per_page=100")
  matches=$(jq -c --arg sha "$head" --arg branch "$branch" --arg repo "$GH_REPO" --argjson workflow "$workflow" --argjson number "$number" '[.[].workflow_runs[] | select(.workflow_id == $workflow and .head_sha == $sha and .head_branch == $branch and .head_repository.full_name == $repo and ((.event == "workflow_dispatch") or (.event == "pull_request" and any(.pull_requests[]; .number == $number))))]' <<<"$runs")
  if [[ $(jq length <<<"$matches") != 0 ]]; then
    while read -r id; do
      [[ -n "$id" ]] || continue
      # Re-read immediately before approving: a changed PR HEAD is not authorized.
      current=$(gh api "repos/${GH_REPO}/pulls/${number}" --jq .head.sha)
      [[ "$current" == "$head" ]] || { echo 'PR head changed; retry workflow' >&2; exit 1; }
      gh api -X POST "repos/${GH_REPO}/actions/runs/${id}/approve" >/dev/null
    done < <(jq -r '.[] | select(.event == "pull_request" and .conclusion == "action_required") | .id' <<<"$matches")
    exit 0
  fi
  if ((i < tries)); then sleep "$nap"; fi
done
current=$(gh api "repos/${GH_REPO}/pulls/${number}" --jq .head.sha)
[[ "$current" == "$head" ]] || { echo 'PR head changed; retry workflow' >&2; exit 1; }
gh workflow run ci.yml --ref "$branch"
