#!/usr/bin/env bash
# Publishes the wiki/ folder to the repository's GitHub wiki.
#
#   ./scripts/publish-wiki.sh Fred78290/cakeagent
#
# Authentication: GH_TOKEN or GITHUB_TOKEN (a token that can push to the wiki), or USE_SSH=1.
# The wiki must have been initialized once in the GitHub UI (a first page created), otherwise it can't be cloned.
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WIKI_DIR="${PROJECT_ROOT}/wiki"
GITHUB_REPOSITORY="${1:-${GITHUB_REPOSITORY:-}}"

if [[ ! -d "${WIKI_DIR}" ]]; then
  echo "Error: wiki directory not found: ${WIKI_DIR}" >&2
  exit 1
fi

if [[ -z "${GITHUB_REPOSITORY}" ]]; then
  echo "Usage: $0 <owner/repo>" >&2
  echo "Example: $0 Fred78290/cakeagent" >&2
  exit 1
fi

TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
WIKI_REMOTE_DISPLAY="https://github.com/${GITHUB_REPOSITORY}.wiki.git"

if [[ "${USE_SSH:-0}" == "1" ]]; then
  WIKI_REMOTE="git@github.com:${GITHUB_REPOSITORY}.wiki.git"
elif [[ -n "${TOKEN}" ]]; then
  WIKI_REMOTE="https://x-access-token:${TOKEN}@github.com/${GITHUB_REPOSITORY}.wiki.git"
else
  WIKI_REMOTE="${WIKI_REMOTE_DISPLAY}"
fi

if ! git ls-remote "${WIKI_REMOTE}" >/dev/null 2>&1; then
  echo "Unable to access remote wiki: ${WIKI_REMOTE_DISPLAY}" >&2
  echo "Check the token (GH_TOKEN/GITHUB_TOKEN or USE_SSH=1), and that 'Wiki' is enabled in the repository settings with a first page created." >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

echo "Cloning remote wiki: ${WIKI_REMOTE_DISPLAY}"
git clone --quiet "${WIKI_REMOTE}" "${TMP_DIR}/wiki-repo"

cd "${TMP_DIR}/wiki-repo"
WIKI_BRANCH="$(git symbolic-ref --short HEAD 2>/dev/null || echo master)"

find . -mindepth 1 -maxdepth 1 ! -name '.git' -exec rm -rf {} +
cp -R "${WIKI_DIR}/." .

# GitHub wants _Sidebar.md (capital S) for the sidebar to be recognised.
if [[ -f "_sidebar.md" && ! -f "_Sidebar.md" ]]; then
  mv "_sidebar.md" "_Sidebar.md"
fi

git add .

if git diff --cached --quiet; then
  echo "No changes to publish."
  exit 0
fi

if [[ -z "$(git config user.name || true)" ]]; then
  git config user.name "${GITHUB_REPOSITORY%%/*}"
  git config user.email "${GITHUB_REPOSITORY%%/*}@users.noreply.github.com"
fi

git commit --quiet -m "${COMMIT_MSG:-Update wiki from repository}"
git push origin "${WIKI_BRANCH}"

echo "Wiki successfully published to ${GITHUB_REPOSITORY}."
