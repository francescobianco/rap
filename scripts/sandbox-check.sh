#!/usr/bin/env bash
# sandbox-check.sh - environment diagnostic for agent sandboxes.
#
# Reports whether bubblewrap, user namespaces and AppArmor are configured so
# that sandboxed coding agents can run on this machine. Not required by RAP
# itself; kept here as a troubleshooting aid.

set -euo pipefail

echo "======================================"
echo " Codex CLI Sandbox Diagnostic"
echo "======================================"
echo

GREEN="\e[32m"
RED="\e[31m"
YELLOW="\e[33m"
NC="\e[0m"

ok()   { echo -e "${GREEN}✔${NC} $1"; }
warn() { echo -e "${YELLOW}⚠${NC} $1"; }
err()  { echo -e "${RED}✘${NC} $1"; }

echo "OS:"
cat /etc/os-release | grep PRETTY_NAME || true
echo

echo "Kernel:"
uname -r
echo

echo "-----------------------------"
echo "Bubblewrap"
echo "-----------------------------"

if command -v bwrap >/dev/null; then
    ok "bwrap found: $(command -v bwrap)"
    bwrap --version || true
else
    err "bubblewrap NOT installed"
fi

echo
echo "-----------------------------"
echo "Codex"
echo "-----------------------------"

if command -v codex >/dev/null; then
    ok "codex found"
    codex --version || true
else
    err "codex not found in PATH"
fi

echo
echo "-----------------------------"
echo "User namespaces"
echo "-----------------------------"

if [ -f /proc/sys/kernel/unprivileged_userns_clone ]; then
    VALUE=$(cat /proc/sys/kernel/unprivileged_userns_clone)
    echo "kernel.unprivileged_userns_clone = $VALUE"

    if [ "$VALUE" = "1" ]; then
        ok "user namespaces enabled"
    else
        warn "user namespaces DISABLED"
    fi
fi

if [ -f /proc/sys/kernel/apparmor_restrict_unprivileged_userns ]; then
    VALUE=$(cat /proc/sys/kernel/apparmor_restrict_unprivileged_userns)
    echo "kernel.apparmor_restrict_unprivileged_userns = $VALUE"

    if [ "$VALUE" = "1" ]; then
        warn "AppArmor restricts user namespaces"
    else
        ok "AppArmor does not restrict them"
    fi
fi

echo
echo "-----------------------------"
echo "AppArmor"
echo "-----------------------------"

if systemctl is-active --quiet apparmor; then
    ok "AppArmor service active"
else
    warn "AppArmor service NOT active"
fi

if [ -f /etc/apparmor.d/bwrap-userns-restrict ]; then
    ok "bwrap profile present"
else
    warn "profile /etc/apparmor.d/bwrap-userns-restrict missing"
fi

echo
echo "-----------------------------"
echo "Bubblewrap test"
echo "-----------------------------"

if command -v bwrap >/dev/null; then
    if bwrap \
        --ro-bind /usr /usr \
        --ro-bind /bin /bin \
        --proc /proc \
        --dev /dev \
        /bin/sh -c 'echo sandbox-ok' >/tmp/bwrap-test.out 2>/tmp/bwrap-test.err
    then
        ok "Bubblewrap works"
        cat /tmp/bwrap-test.out
    else
        err "Bubblewrap does NOT work"
        echo
        cat /tmp/bwrap-test.err
    fi
fi

echo
echo "======================================"
echo " Diagnostics complete"
echo "======================================"