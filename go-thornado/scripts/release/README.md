# THORNode Release Scripts

Interactive automation for the THORNode release process. These scripts replace
the manual steps documented in [`docs/release.md`](../../docs/release.md) with
guided, confirmation-gated workflows.

## Contents

| File         | Description                                                                             |
| ------------ | --------------------------------------------------------------------------------------- |
| `release.sh` | Main release driver with subcommands for stagenet, mainnet, and bifrost-patch releases. |
| `prlog.py`   | Generates a changelog from merged MRs in a GitLab milestone.                            |

## Prerequisites

| Tool        | Required By             | Purpose                                                                                         |
| ----------- | ----------------------- | ----------------------------------------------------------------------------------------------- |
| `git`       | all                     | Branch management, tagging, cherry-picks                                                        |
| `glab`      | all                     | GitLab CLI — creating releases, fetching release notes                                          |
| `uv`        | stagenet, bifrost-patch | Runs `prlog.py` with auto-installed dependencies ([PEP 723](https://peps.python.org/pep-0723/)) |
| `jq`        | all                     | JSON parsing (version API, etc.)                                                                |
| `goimports` | bifrost-patch           | Go source formatting after editing `app/upgrades.go`                                            |
| `curl`      | bifrost-patch           | Fetching the current consensus version from the mainnet API                                     |

`glab` must be authenticated before running (`glab auth login`).

## Usage

All commands are run from the repository root.

### Stagenet Release

```bash
./scripts/release/release.sh stagenet
```

Walks you through:

1. Reading the current version from the `version` file.
2. Auto-detecting the next RC number from existing `v<version>-rc*` tags.
3. Prompting for the RC number, milestone name, and tag target commit.
4. Generating a changelog via `prlog.py` for the given milestone.
5. Creating a GitLab release with the changelog.
6. Checking out the release tag and force-pushing the `stagenet` branch.
7. Printing manual follow-up steps (upgrade proposal, Discord announcement, testing).

### Mainnet Release

```bash
./scripts/release/release.sh mainnet
```

Walks you through:

1. Reading the current version and verifying the chosen RC tag exists locally.
2. Prompting for the RC tag, upgrade block height, and upgrade date.
3. Fetching release notes from the RC release (falls back to `prlog.py` if unavailable).
4. Composing mainnet release notes with block height, date, and changelog.
5. Checking out the RC tag and force-pushing the `mainnet` branch.
6. Creating a GitLab release on the `mainnet` branch.
7. Printing manual follow-up steps (upgrade proposal, Discord/exchange announcements, node-launcher PR).

### Bifrost-Only Patch Release

```bash
./scripts/release/release.sh bifrost-patch
```

Walks you through:

1. Reading the current version and fetching the consensus version from the mainnet API.
2. Computing the next patch version (bumps the PATCH segment by 1).
3. Syncing the local `mainnet` branch with `origin/mainnet`.
4. Cherry-picking one or more commits from `develop`.
5. Bumping the `version` file and running `make generate`.
6. Inserting a `standard.NewUpgrade("<consensus_version>")` entry into `app/upgrades.go` and formatting with `goimports`.
7. Committing all changes as `Release <version>` and pushing the `mainnet` branch.
8. Generating a changelog (from a milestone or auto-generated from cherry-picked commit subjects).
9. Creating a GitLab release on the `mainnet` branch.
10. Printing manual follow-up steps (node-launcher PR, Discord announcement).

## Changelog Generator (`prlog.py`)

Standalone script that lists all merged MRs for a given GitLab milestone.

```bash
# Requires uv (auto-installs python-gitlab via PEP 723 inline metadata)
uv run ./scripts/release/prlog.py "Release-3.17.0"
```

Output format:

```text
1) MR title PR: https://gitlab.com/thorchain/thornode/-/merge_requests/1234
2) Another MR title PR: https://gitlab.com/thorchain/thornode/-/merge_requests/1235
```

The script uses the public GitLab API with no authentication token, so only
public project data is accessible. The THORNode project ID (`13422983`) is
hardcoded.

## Safety Features

- **Clean worktree check** — refuses to run if there are uncommitted changes.
- **Confirmation prompts** — every destructive or visible action (force-push, release creation, changelog acceptance) requires explicit `y` confirmation.
- **Duplicate release detection** — warns and prompts for an alternative tag/version if a GitLab release already exists.
- **Prerequisite validation** — checks that all required CLI tools are installed and `glab` is authenticated before proceeding.

## Relationship to `docs/release.md`

`docs/release.md` is the canonical reference for the full release process,
including manual steps like Discord announcements, exchange coordination, and
node-launcher PRs. These scripts automate the git and GitLab portions of that
process. After each subcommand completes, it prints the remaining manual
follow-up steps that cannot be automated.
