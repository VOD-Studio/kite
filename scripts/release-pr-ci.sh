#!/usr/bin/env bash
# Only exact same-repository component release PRs can approve/dispatch ci.yml.
set -euo pipefail
PYTHONPATH="$(cd "$(dirname "$0")" && pwd)${PYTHONPATH:+:$PYTHONPATH}"
export PYTHONPATH
python3 - <<'PY'
import os
import sys
import time
import release_common as common


def ensure(component):
    branch = common.release_branch(component)
    prs = [pr for page in common.pages(f"pulls?state=open&base=main&head=VOD-Studio:{branch}&per_page=100") for pr in page]
    trusted = [pr for pr in prs if pr.get('state') == 'open' and common.trusted_release_pr(pr, component)]
    if not trusted:
        return
    if len(trusted) != 1:
        raise ValueError('multiple trusted release PRs; refusing ambiguous dispatch')
    pr = trusted[0]
    head, number = pr['head']['sha'], pr['number']
    if not common.SHA.fullmatch(head):
        raise ValueError('release PR head must be a full commit SHA')
    workflow = common.api('actions/workflows/ci.yml')
    if workflow.get('path') != '.github/workflows/ci.yml' or workflow.get('state') != 'active':
        raise ValueError('ci.yml is not the active trusted producer')

    def unchanged():
        current = common.api(f'pulls/{number}')
        if (current.get('state') != 'open' or not common.trusted_release_pr(current, component)
                or current['head']['sha'] != head or current['title'] != pr['title']):
            raise ValueError('release PR identity or head changed; retry workflow')

    tries = int(os.environ.get('POLL_TRIES', '6'))
    for attempt in range(tries):
        runs = [run for page in common.pages(f"actions/workflows/{workflow['id']}/runs?head_sha={head}&per_page=100") for run in page['workflow_runs']]
        matches = [run for run in runs if run.get('workflow_id') == workflow['id']
                   and run.get('path') == '.github/workflows/ci.yml'
                   and run.get('head_sha') == head and run.get('head_branch') == branch
                   and (run.get('head_repository') or {}).get('full_name') == common.REPO
                   and (run.get('event') == 'workflow_dispatch'
                        or (run.get('event') == 'pull_request' and any(p['number'] == number for p in run.get('pull_requests', []))))]
        if matches:
            for run in matches:
                if run['event'] == 'pull_request' and run.get('conclusion') == 'action_required':
                    unchanged()
                    common.command(os.environ.get('GH', 'gh'), 'api', f"repos/{common.REPO}/actions/runs/{run['id']}/approve", '--method', 'POST')
            return
        if attempt + 1 < tries:
            time.sleep(int(os.environ.get('POLL_SLEEP', '10')))
    unchanged()
    common.command(os.environ.get('GH', 'gh'), 'workflow', 'run', 'ci.yml', '--ref', branch)


try:
    common.require_main()
    if os.environ.get('GH_REPO') != common.REPO:
        raise ValueError('unexpected repository')
    for component in common.COMPONENTS:
        ensure(component)
except (ValueError, KeyError, common.subprocess.CalledProcessError) as error:
    sys.exit(f'release PR CI refused: {error}')
PY
