#!/bin/bash
# run-e2e-container.sh
# Runs the fastVIP e2e test suite inside a Docker container.
#
# Supports:
#   - macOS with Docker Desktop (Linux VM kernel has IPVS built-in; modprobe
#     runs inside the container via the entrypoint script)
#   - Linux hosts with Docker (same container-internal modprobe approach)
#
# Requires: Docker (no root on the host required for macOS Docker Desktop).
# Reference: docs/deploy/start-container.sh

set -euo pipefail

IMAGE_NAME="fastVIP-e2e-test"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

echo "==> Building e2e test image..."
docker build -f "${PROJECT_ROOT}/Dockerfile.e2e" -t "${IMAGE_NAME}" "${PROJECT_ROOT}"

echo "==> Running e2e tests in container..."
# --privileged grants the container full access to the host kernel, which is
# required for modprobe (loading IPVS modules) and IPVS netlink operations.
# On macOS with Colima/Docker Desktop the "host kernel" is the Linux VM managed
# by the container runtime, so --privileged is both safe and necessary.
#
# -v /lib/modules:/lib/modules:ro mounts the host VM's kernel module directory
# into the container so that modprobe can locate and load the ip_vs modules.
# This is required because the container image does not ship kernel modules.
docker run --rm \
  --privileged \
  --network host \
  -v /lib/modules:/lib/modules:ro \
  "${IMAGE_NAME}"
