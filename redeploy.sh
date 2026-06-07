#!/usr/bin/env bash
# Build backend and frontend Docker images, push to Docker Hub, and restart
# the stack using the published images.
#
# Usage:
#   ./redeploy.sh              # build + push + restart (default)
#   ./redeploy.sh --push-only  # build + push, skip docker compose up
#
# Requires: docker login to hub.docker.com (docker login)

set -euo pipefail

DOCKER_USER="vpoluyaktov"
BACKEND_IMAGE="${DOCKER_USER}/promptevo-backend"
FRONTEND_IMAGE="${DOCKER_USER}/promptevo-frontend"

# Derive a version tag from git: short SHA, or "dev" if not in a git repo.
GIT_TAG=$(git rev-parse --short HEAD 2>/dev/null || echo "dev")

PUSH_ONLY=false
for arg in "$@"; do
  [[ "$arg" == "--push-only" ]] && PUSH_ONLY=true
done

echo "==> Building backend  (${BACKEND_IMAGE}:${GIT_TAG})"
docker build \
  -f Dockerfile.backend \
  -t "${BACKEND_IMAGE}:${GIT_TAG}" \
  -t "${BACKEND_IMAGE}:latest" \
  .

echo "==> Building frontend (${FRONTEND_IMAGE}:${GIT_TAG})"
docker build \
  -f frontend/Dockerfile \
  -t "${FRONTEND_IMAGE}:${GIT_TAG}" \
  -t "${FRONTEND_IMAGE}:latest" \
  ./frontend

echo "==> Pushing backend"
docker push "${BACKEND_IMAGE}:${GIT_TAG}"
docker push "${BACKEND_IMAGE}:latest"

echo "==> Pushing frontend"
docker push "${FRONTEND_IMAGE}:${GIT_TAG}"
docker push "${FRONTEND_IMAGE}:latest"

if [[ "$PUSH_ONLY" == "true" ]]; then
  echo "==> Push complete. Skipping docker compose up (--push-only)."
  exit 0
fi

echo "==> Pulling latest images and restarting stack"
BACKEND_IMAGE="${BACKEND_IMAGE}" FRONTEND_IMAGE="${FRONTEND_IMAGE}" \
  docker compose pull

BACKEND_IMAGE="${BACKEND_IMAGE}" FRONTEND_IMAGE="${FRONTEND_IMAGE}" \
  docker compose up -d

echo "==> Done. Stack is running."
