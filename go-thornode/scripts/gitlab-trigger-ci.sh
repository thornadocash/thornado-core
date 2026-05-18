#!/usr/bin/env bash
set -euo pipefail

# Check if glab is installed
if ! command -v glab &>/dev/null; then
  echo "Error: glab (GitLab CLI) is not installed."
  echo "Please install it from: https://gitlab.com/gitlab-org/cli"
  exit 1
fi

# Check if glab is authenticated
if ! glab auth status &>/dev/null; then
  echo "Error: glab is not authenticated."
  echo "Please run: glab auth login"
  exit 1
fi

# Use MR_ID env var if provided, otherwise prompt
if [ -n "${MR_ID-}" ]; then
  MR="$MR_ID"
  echo "Using MR_ID from environment: $MR"
else
  # prompt for gitlab merge request id
  read -rp "Enter Gitlab Merge Request ID: " MR
fi

# Get MR details using glab
echo "Fetching merge request details..."
MR_INFO=$(glab mr view "$MR" --output json 2>/dev/null || true)

if [ -z "$MR_INFO" ]; then
  echo "Error: Unable to fetch merge request #$MR"
  exit 1
fi

# Debug: Show the JSON structure
if [ "${DEBUG:-0}" = "1" ]; then
  echo "=== DEBUG: Full MR JSON ==="
  echo "$MR_INFO" | jq '.'
  echo "=== END DEBUG ==="
fi

# Extract source branch - this is always available
SOURCE_BRANCH=$(echo "$MR_INFO" | jq -r '.source_branch // empty')

# Check if this is a fork MR by comparing project IDs
SOURCE_PROJECT_ID=$(echo "$MR_INFO" | jq -r '.source_project_id // empty')
TARGET_PROJECT_ID=$(echo "$MR_INFO" | jq -r '.target_project_id // empty')

# For fork MRs, we need to fetch the source project details separately
if [ "$SOURCE_PROJECT_ID" != "$TARGET_PROJECT_ID" ] && [ -n "$SOURCE_PROJECT_ID" ]; then
  # This is a fork MR - fetch source project details
  echo "Fetching fork project details..."
  SOURCE_PROJECT_INFO=$(glab api "projects/$SOURCE_PROJECT_ID" 2>/dev/null || true)

  if [ -n "$SOURCE_PROJECT_INFO" ]; then
    SOURCE_PROJECT=$(echo "$SOURCE_PROJECT_INFO" | jq -r '.path_with_namespace // empty')
    SOURCE_REMOTE=$(echo "$SOURCE_PROJECT_INFO" | jq -r '.ssh_url_to_repo // empty')
  fi
else
  # Same project MR
  SOURCE_PROJECT="thorchain/thornode"
  SOURCE_REMOTE=""
fi

# Try to extract from the standard structure if available
if [ -z "$SOURCE_PROJECT" ] || [ "$SOURCE_PROJECT" = "null" ]; then
  # Extract source branch and project path from MR
  # The glab CLI returns different JSON structure than the GitLab API
  # Check if this is using the newer structure
  if echo "$MR_INFO" | jq -e '.source_branch' >/dev/null 2>&1; then
    # Older structure
    SOURCE_BRANCH=$(echo "$MR_INFO" | jq -r '.source_branch')
    SOURCE_PROJECT=$(echo "$MR_INFO" | jq -r '.source_project.path_with_namespace')
    SOURCE_REMOTE=$(echo "$MR_INFO" | jq -r '.source_project.ssh_url_to_repo')
  fi
fi

# Validate extracted values
if [ -z "$SOURCE_BRANCH" ] || [ "$SOURCE_BRANCH" = "null" ]; then
  echo "Error: Unable to extract source branch from MR #$MR"
  echo "Try running with DEBUG=1 to see the JSON structure"
  exit 1
fi

if [ -z "$SOURCE_PROJECT" ] || [ "$SOURCE_PROJECT" = "null" ]; then
  echo "Error: Unable to extract source project from MR #$MR"
  echo "Try running with DEBUG=1 to see the JSON structure"
  exit 1
fi

CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)

echo "MR #$MR: $SOURCE_BRANCH from $SOURCE_PROJECT"

# Use a unique branch name based on MR number and source branch
LOCAL_BRANCH="trigger-ci-mr-$MR-$SOURCE_BRANCH"

# Clean up any existing local branch
git branch -D "${LOCAL_BRANCH}" 2>/dev/null || true

# Add fork remote if needed
if [ "$SOURCE_PROJECT" != "thorchain/thornode" ]; then
  FORK_REMOTE="fork-mr-$MR"
  git remote remove "$FORK_REMOTE" 2>/dev/null || true
  git remote add "$FORK_REMOTE" "$SOURCE_REMOTE"
  echo "Added remote '$FORK_REMOTE' for fork: $SOURCE_REMOTE"

  # Fetch from fork
  git fetch "$FORK_REMOTE" "$SOURCE_BRANCH":"$LOCAL_BRANCH"

  # Clean up fork remote
  git remote remove "$FORK_REMOTE"
else
  # For MRs from the same repo, use glab to checkout
  glab mr checkout "$MR" -b "$LOCAL_BRANCH"
fi

# Push to origin to trigger CI
echo "Pushing branch to trigger CI..."
git push --set-upstream origin "${LOCAL_BRANCH}" -f --no-verify

# Return to original branch
git checkout "$CURRENT_BRANCH"

# Trigger pipeline using glab
echo
echo "Triggering pipeline on branch: $LOCAL_BRANCH"
PIPELINE_OUTPUT=$(glab ci run --branch "$LOCAL_BRANCH")
echo "$PIPELINE_OUTPUT"

# Extract pipeline ID from the output
PIPELINE_ID=$(echo "$PIPELINE_OUTPUT" | grep -oE 'id: [0-9]+' | head -1 | cut -d' ' -f2)

# Show initial pipeline status
echo
echo "Pipeline triggered. View status:"
echo "----------------------------------------"
glab ci status --branch "$LOCAL_BRANCH" 2>&1 | grep -E '^\(' | while IFS= read -r line; do
  echo "$line"
done
echo "----------------------------------------"

# Interactive menu loop
while true; do
  echo
  echo "========================================="
  echo "Choose an action:"
  echo "1) View current status"
  echo "2) View pipeline logs"
  echo "3) Retry failed jobs"
  echo "4) Open pipeline in browser"
  echo "5) Exit"
  echo "========================================="

  read -rp "Enter your choice (1-5): " choice

  case $choice in
  1)
    echo
    echo "Current pipeline status:"
    echo "----------------------------------------"
    glab ci status --branch "$LOCAL_BRANCH" 2>&1 | grep -E '^\(' | while IFS= read -r line; do
      echo "$line"
    done
    echo "----------------------------------------"
    ;;
  2)
    echo
    echo "Fetching pipeline logs..."
    if [ -n "$PIPELINE_ID" ]; then
      # Show job list first
      echo "Available jobs:"
      glab api "projects/:id/pipelines/$PIPELINE_ID/jobs" | jq -r '.[] | "\(.id) - \(.name) (\(.status))"'

      read -rp "Enter job ID to view logs (or press Enter to skip): " JOB_ID
      if [ -n "$JOB_ID" ]; then
        echo
        echo "Logs for job $JOB_ID:"
        glab api "projects/:id/jobs/$JOB_ID/trace" | less
      fi
    else
      echo "Unable to determine pipeline ID"
    fi
    ;;
  3)
    echo
    echo "Retrying failed jobs..."
    if [ -n "$PIPELINE_ID" ]; then
      glab ci retry "$PIPELINE_ID"
      echo "Retry initiated. Checking new status..."
      sleep 2
      glab ci status --branch "$LOCAL_BRANCH"
    else
      echo "Unable to determine pipeline ID"
    fi
    ;;
  4)
    echo
    echo "Opening pipeline in browser..."
    if [ -n "$PIPELINE_ID" ]; then
      glab ci view "$PIPELINE_ID" --web
    else
      # Fallback to branch-based view
      glab ci view --branch "$LOCAL_BRANCH" --web
    fi
    ;;
  5)
    echo
    echo "Exiting..."
    break
    ;;
  *)
    echo
    echo "Invalid choice. Please enter a number between 1 and 5."
    ;;
  esac
done

# Ensure clean exit
exit 0
