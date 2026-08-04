#!/bin/bash
# docker-entrypoint.sh
# Loads IPVS kernel modules inside the container before running the test command.
#
# This runs inside the container (requires --privileged on the docker run side).
# On macOS Docker Desktop, the underlying kernel is the Linux VM managed by
# Docker Desktop, which ships with IPVS modules built-in, so modprobe succeeds.
# On a Linux host the same approach works as long as the host kernel has IPVS.

set -euo pipefail

echo "==> Loading IPVS kernel modules..."
modprobe ip_vs
modprobe ip_vs_rr
modprobe ip_vs_wrr
modprobe ip_vs_lc
modprobe ip_vs_wlc
modprobe ip_vs_sh
modprobe ip_vs_dh
modprobe nf_conntrack
echo "==> IPVS modules loaded."

exec "$@"
