#!/usr/bin/env bash
set -euo pipefail
trap 'echo "ERROR: Script failed at line $LINENO (exit code $?)" >&2' ERR

# ---------------------------------------------------------------------------
# THORNode Release Automation
#
# Usage:
#   ./scripts/release/release.sh stagenet
#   ./scripts/release/release.sh mainnet
#   ./scripts/release/release.sh bifrost-patch
#
# Automates the release process documented in docs/release.md.
# ---------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
VERSION_FILE="$REPO_ROOT/version"
UPGRADES_FILE="$REPO_ROOT/app/upgrades.go"
PRLOG_SCRIPT="$SCRIPT_DIR/prlog.py"
THORNODE_API="https://gateway.liquify.com/chain/thorchain_api/thorchain"

# ---------------------------------------------------------------------------
# Utility functions
# ---------------------------------------------------------------------------

die() {
  echo "ERROR: $*" >&2
  exit 1
}

info() {
  echo "==> $*"
}

warn() {
  echo "WARNING: $*" >&2
}

confirm() {
  local prompt="${1:-Continue?}"
  local reply
  read -rp "$prompt [y/N] " reply
  [[ $reply =~ ^[Yy]$ ]] || die "Aborted by user."
}

require_cmd() {
  for cmd in "$@"; do
    command -v "$cmd" &>/dev/null || die "'$cmd' is required but not installed."
  done
}

check_glab_auth() {
  glab auth status &>/dev/null || die "glab is not authenticated. Run: glab auth login"
}

check_clean_worktree() {
  if [[ -n "$(git -C "$REPO_ROOT" status --porcelain)" ]]; then
    die "Working tree is not clean. Please commit or stash changes first."
  fi
}

read_version() {
  [[ -f $VERSION_FILE ]] || die "Version file not found: $VERSION_FILE"
  VERSION="$(tr -d '[:space:]' <"$VERSION_FILE")"
  [[ -n $VERSION ]] || die "Version file is empty."
}

bump_patch() {
  local ver="$1"
  local major minor patch
  IFS='.' read -r major minor patch <<<"$ver"
  echo "${major}.${minor}.$((patch + 1))"
}

generate_changelog() {
  local milestone="$1"
  info "Generating changelog for milestone: $milestone"
  uv run "$PRLOG_SCRIPT" "$milestone"
}

check_release_exists() {
  local tag="$1"
  if glab release view "$tag" &>/dev/null; then
    echo ""
    warn "A GitLab release already exists for tag '$tag'."
    return 0
  fi
  return 1
}

# ---------------------------------------------------------------------------
# Stagenet release
# ---------------------------------------------------------------------------

cmd_stagenet() {
  info "Starting STAGENET release..."
  echo ""

  # Prerequisites
  require_cmd glab uv jq git
  check_glab_auth
  check_clean_worktree
  read_version

  echo "Current version: $VERSION"
  echo ""

  # Predict next RC number by finding the highest existing RC tag
  git -C "$REPO_ROOT" fetch --tags --force --quiet
  local latest_rc
  latest_rc="$(git -C "$REPO_ROOT" tag -l "v${VERSION}-rc*" |
    sed 's/.*-rc//' |
    sort -n |
    tail -1)"
  local default_rc
  if [[ -n $latest_rc ]]; then
    default_rc=$((latest_rc + 1))
    info "Found existing RC tags up to rc${latest_rc}."
  else
    default_rc=1
  fi

  # Prompt for RC number
  local rc_num
  read -rp "RC number [${default_rc}]: " rc_num
  rc_num="${rc_num:-$default_rc}"

  local rc_tag="v${VERSION}-rc${rc_num}"

  # Check if this RC release already exists
  if check_release_exists "$rc_tag"; then
    local custom_tag
    read -rp "Enter a different tag (or Ctrl-C to abort): " custom_tag
    [[ -n $custom_tag ]] || die "No tag provided."
    rc_tag="$custom_tag"
  fi

  # Prompt for milestone
  local milestone
  read -rp "Milestone name [Release-${VERSION}]: " milestone
  milestone="${milestone:-Release-${VERSION}}"

  # Prompt for tag target
  local tag_ref
  read -rp "Tag target commit [HEAD]: " tag_ref
  tag_ref="${tag_ref:-HEAD}"
  tag_ref="$(git -C "$REPO_ROOT" rev-parse "$tag_ref")"

  echo ""
  info "Release details:"
  echo "  Tag:        $rc_tag"
  echo "  Milestone:  $milestone"
  echo "  Commit:     $tag_ref"
  echo ""

  # Generate changelog
  info "Generating changelog..."
  local changelog
  changelog="$(generate_changelog "$milestone")" || die "Failed to generate changelog."
  echo ""
  echo "--- Changelog Preview ---"
  echo "$changelog"
  echo "--- End Changelog ---"
  echo ""
  confirm "Does the changelog look correct?"

  # Create release
  info "Creating GitLab release: $rc_tag"
  glab release create "$rc_tag" \
    --ref "$tag_ref" \
    --notes "$changelog"

  info "Release $rc_tag created successfully."
  echo ""

  # Push stagenet branch
  confirm "Checkout $rc_tag and force-push the 'stagenet' branch?"

  git -C "$REPO_ROOT" fetch --tags --force origin
  local original_branch
  original_branch="$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)"
  [[ $original_branch != "HEAD" ]] || original_branch="$(git -C "$REPO_ROOT" rev-parse HEAD)"
  git -C "$REPO_ROOT" checkout "$rc_tag"
  git -C "$REPO_ROOT" branch -D stagenet 2>/dev/null || true
  git -C "$REPO_ROOT" checkout -b stagenet
  git -C "$REPO_ROOT" push -f origin stagenet
  git -C "$REPO_ROOT" checkout "$original_branch" --quiet

  info "Branch 'stagenet' force-pushed to origin."
  echo ""

  echo "========================================="
  echo " Manual follow-up steps:"
  echo "========================================="
  echo "1. Wait for the 'build-thornode' CI job to complete on the stagenet branch."
  echo "2. Send upgrade proposal from an active validator:"
  echo "     thornode tx thorchain propose-upgrade ${VERSION} <height> \\"
  # shellcheck disable=SC1003
  echo '       --node http://localhost:27147 --chain-id thorchain-stagenet-2 \'
  echo "       --keyring-backend file --from thorchain"
  echo "3. Approve the upgrade from all other nodes via 'make upgrade-vote' in node-launcher."
  echo "4. Announce in Discord #stagenet tagging @here once the upgrade succeeds."
  echo "5. Coordinate testing per the release test plan."
  echo "========================================="
}

# ---------------------------------------------------------------------------
# Mainnet release
# ---------------------------------------------------------------------------

cmd_mainnet() {
  info "Starting MAINNET release..."
  echo ""

  # Prerequisites
  require_cmd glab jq git
  check_glab_auth
  check_clean_worktree
  read_version

  local release_tag="v${VERSION}"

  echo "Current version: $VERSION"
  echo ""

  # Check if mainnet release already exists
  if check_release_exists "$release_tag"; then
    local custom_ver
    read -rp "Enter a different version string (or Ctrl-C to abort): " custom_ver
    [[ -n $custom_ver ]] || die "No version provided."
    VERSION="$custom_ver"
    release_tag="v${VERSION}"
  fi

  # Prompt for RC tag
  local rc_tag
  read -rp "RC tag to base release on [v${VERSION}-rc1]: " rc_tag
  rc_tag="${rc_tag:-v${VERSION}-rc1}"

  # Verify RC tag exists
  git -C "$REPO_ROOT" rev-parse "$rc_tag" &>/dev/null ||
    die "Tag '$rc_tag' does not exist locally. Run: git fetch --tags"

  # Prompt for upgrade details
  local block_height
  read -rp "Upgrade block height: " block_height
  [[ $block_height =~ ^[0-9]+$ ]] || die "Block height must be a positive integer."

  local upgrade_date
  read -rp "Upgrade date string (e.g. '22-May-2025 @ ~1:00pm EDT'): " upgrade_date
  [[ -n $upgrade_date ]] || die "Upgrade date is required."

  echo ""

  # Fetch RC release notes
  info "Fetching release notes from $rc_tag..."
  local rc_notes
  rc_notes="$(glab release view "$rc_tag" 2>/dev/null | sed '1,/^---$/d')" || true

  if [[ -z $rc_notes ]]; then
    warn "Could not fetch notes from $rc_tag release. Falling back to prlog."
    require_cmd uv
    local milestone
    read -rp "Milestone name for changelog [Release-${VERSION}]: " milestone
    milestone="${milestone:-Release-${VERSION}}"
    rc_notes="$(generate_changelog "$milestone")" || die "Failed to generate changelog."
  fi

  # Compose mainnet release body
  local release_notes
  release_notes="$(
    cat <<EOF
- **Proposed Block**: \`${block_height}\`
- **Date**: ${upgrade_date} - https://runescan.io/block/${block_height}
- **Note**: Block time is an estimate and may fluctuate.

**Changelog**
${rc_notes}
EOF
  )"

  echo ""
  echo "--- Release Notes Preview ---"
  echo "$release_notes"
  echo "--- End Preview ---"
  echo ""
  confirm "Do the release notes look correct?"

  # Push mainnet branch
  confirm "Checkout $rc_tag and force-push the 'mainnet' branch?"

  git -C "$REPO_ROOT" fetch origin
  local original_branch
  original_branch="$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)"
  [[ $original_branch != "HEAD" ]] || original_branch="$(git -C "$REPO_ROOT" rev-parse HEAD)"
  git -C "$REPO_ROOT" checkout "$rc_tag"
  git -C "$REPO_ROOT" branch -D mainnet 2>/dev/null || true
  git -C "$REPO_ROOT" checkout -b mainnet
  git -C "$REPO_ROOT" push -f origin mainnet
  git -C "$REPO_ROOT" checkout "$original_branch" --quiet

  info "Branch 'mainnet' force-pushed to origin."
  echo ""

  # Create release
  info "Creating GitLab release: $release_tag"
  glab release create "$release_tag" \
    --ref mainnet \
    --notes "$release_notes"

  info "Release $release_tag created successfully."
  echo ""

  echo "========================================="
  echo " Manual follow-up steps:"
  echo "========================================="
  echo "1. Wait for the 'build-thornode' CI job to complete on the mainnet branch."
  echo "2. Send upgrade proposal from an active validator:"
  echo "     thornode tx thorchain propose-upgrade ${VERSION} ${block_height} \\"
  # shellcheck disable=SC1003
  echo '       --node http://localhost:27147 --chain-id thorchain-1 \'
  echo "       --keyring-backend file --from thorchain"
  echo "3. Announce upgrade proposal in Discord #thornode-mainnet."
  echo "4. Relay to exchange chats."
  echo "5. PR in node-launcher to extend thornode.versions with:"
  echo "     height: ${block_height}, image: ${release_tag}"
  echo "6. Once upgrade proposal passes, announce in Discord for nodes to apply."
  echo "========================================="
}

# ---------------------------------------------------------------------------
# Bifrost-only patch release
# ---------------------------------------------------------------------------

cmd_bifrost_patch() {
  info "Starting BIFROST-ONLY PATCH release..."
  echo ""

  # Prerequisites
  require_cmd glab uv jq git goimports
  check_glab_auth
  check_clean_worktree

  # Sync local mainnet branch first so we read the correct version
  info "Syncing local 'mainnet' branch with origin..."
  git -C "$REPO_ROOT" fetch origin mainnet
  git -C "$REPO_ROOT" checkout origin/mainnet
  git -C "$REPO_ROOT" branch -D mainnet 2>/dev/null || true
  git -C "$REPO_ROOT" checkout -b mainnet

  info "Local 'mainnet' branch is up to date."
  echo ""

  read_version

  echo "Current version: $VERSION"
  echo ""

  # Fetch consensus version from mainnet API
  local consensus_version
  info "Fetching current consensus version from mainnet API..."
  consensus_version="$(curl -sf "${THORNODE_API}/version" | jq -r '.current // empty' 2>/dev/null)" || true

  if [[ -z $consensus_version ]]; then
    warn "Could not fetch consensus version from API."
    read -rp "Enter current consensus version manually (e.g. 3.16.0): " consensus_version
    [[ -n $consensus_version ]] || die "Consensus version is required."
  else
    info "Current consensus version: $consensus_version"
  fi

  # Compute new patch version
  local new_version
  new_version="$(bump_patch "$VERSION")"

  # Check if release already exists for the new version
  if check_release_exists "v${new_version}"; then
    read -rp "Enter a different version [${new_version}]: " override_version
    new_version="${override_version:-$new_version}"
  fi

  echo ""
  echo "  Current version:    $VERSION"
  echo "  Consensus version:  $consensus_version"
  echo "  New patch version:  $new_version"
  echo ""
  confirm "Proceed with version $new_version?"

  # Cherry-pick commits
  local commits_input
  read -rp "Enter commit hashes to cherry-pick (space-separated): " commits_input
  [[ -n $commits_input ]] || die "No commits provided."

  local -a commits
  read -ra commits <<<"$commits_input"

  for commit in "${commits[@]}"; do
    info "Cherry-picking $commit..."
    if ! git -C "$REPO_ROOT" cherry-pick "$commit"; then
      echo ""
      git -C "$REPO_ROOT" cherry-pick --abort || true
      die "Conflict while cherry-picking $commit. Cherry-pick was aborted. Resolve the commit first, then re-run the script."
    fi
  done

  info "All commits cherry-picked successfully."
  echo ""

  # Bump version file
  info "Bumping version file to $new_version..."
  echo "$new_version" >"$VERSION_FILE"

  # Run make generate
  info "Running 'make generate'..."
  make -C "$REPO_ROOT" generate

  # Edit app/upgrades.go
  info "Updating app/upgrades.go with consensus version entry..."

  # Find the closing } of the var Upgrades slice by locating its opening line
  # and then finding the matching closing brace.
  local start_line
  start_line="$(grep -n '^var Upgrades' "$UPGRADES_FILE" | head -1 | cut -d: -f1)"
  [[ -n $start_line ]] || die "Could not find 'var Upgrades' in $UPGRADES_FILE"

  local closing_line
  closing_line="$(tail -n +"$start_line" "$UPGRADES_FILE" | grep -n '^}' | head -1 | cut -d: -f1)"
  [[ -n $closing_line ]] || die "Could not find closing brace of Upgrades slice in $UPGRADES_FILE"
  closing_line=$((start_line + closing_line - 1))

  # Insert the standard.NewUpgrade entry before the closing brace
  local entry
  entry="	standard.NewUpgrade(\"${consensus_version}\"),"
  local tmpfile
  tmpfile="$(mktemp)"
  awk -v line="$closing_line" -v text="$entry" 'NR==line{print text} {print}' "$UPGRADES_FILE" >"$tmpfile"
  mv "$tmpfile" "$UPGRADES_FILE"

  # Format
  goimports -w "$UPGRADES_FILE"

  echo ""
  echo "--- upgrades.go diff ---"
  git -C "$REPO_ROOT" diff "$UPGRADES_FILE"
  echo "--- end diff ---"
  echo ""
  confirm "Does the upgrades.go change look correct?"

  # Stage and commit
  info "Creating release commit..."
  git -C "$REPO_ROOT" add -A
  git -C "$REPO_ROOT" commit -m "Release ${new_version}"

  echo ""
  confirm "Push 'mainnet' branch to origin?"
  git -C "$REPO_ROOT" push origin mainnet

  info "Branch 'mainnet' pushed to origin."
  echo ""

  # Generate changelog
  local changelog
  echo "How would you like to generate the changelog?"
  echo "  1) From a milestone name"
  echo "  2) Auto-generate from cherry-picked commits"
  local changelog_choice
  read -rp "Choice [1]: " changelog_choice
  changelog_choice="${changelog_choice:-1}"

  case "$changelog_choice" in
  1)
    local milestone
    read -rp "Milestone name [Release-${new_version}]: " milestone
    milestone="${milestone:-Release-${new_version}}"
    changelog="$(generate_changelog "$milestone")" || die "Failed to generate changelog."
    ;;
  2)
    changelog=""
    for commit in "${commits[@]}"; do
      local subject
      subject="$(git -C "$REPO_ROOT" log -1 --format='%s' "$commit")"
      changelog="${changelog}- ${subject} (${commit:0:8})"$'\n'
    done
    ;;
  *)
    die "Invalid choice."
    ;;
  esac

  echo ""
  echo "--- Changelog Preview ---"
  echo "$changelog"
  echo "--- End Changelog ---"
  echo ""
  confirm "Does the changelog look correct?"

  # Create release
  local release_tag="v${new_version}"
  info "Creating GitLab release: $release_tag"
  glab release create "$release_tag" \
    --ref mainnet \
    --notes "$changelog"

  info "Release $release_tag created successfully."
  echo ""

  echo "========================================="
  echo " Manual follow-up steps:"
  echo "========================================="
  echo "1. Wait for the 'build-thornode' CI job to complete on the mainnet branch."
  echo "2. PR in node-launcher to update thornode.versions image to $release_tag."
  echo "3. Announce in Discord #thornode-mainnet for nodes to apply the patch."
  echo "========================================="
}

# ---------------------------------------------------------------------------
# Main dispatch
# ---------------------------------------------------------------------------

usage() {
  echo "Usage: $0 {stagenet|mainnet|bifrost-patch}"
  echo ""
  echo "Automates the THORNode release process (see docs/release.md)."
  echo ""
  echo "Subcommands:"
  echo "  stagenet       Create an RC release and push the stagenet branch"
  echo "  mainnet        Create a mainnet release from an existing RC"
  echo "  bifrost-patch  Create a bifrost-only patch release on mainnet"
  exit 1
}

[[ $# -ge 1 ]] || usage

case "$1" in
stagenet)
  cmd_stagenet
  ;;
mainnet)
  cmd_mainnet
  ;;
bifrost-patch)
  cmd_bifrost_patch
  ;;
*)
  usage
  ;;
esac
