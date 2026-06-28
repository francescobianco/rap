#!/usr/bin/env bash

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
    ok "bwrap trovato: $(command -v bwrap)"
    bwrap --version || true
else
    err "bubblewrap NON installato"
fi

echo
echo "-----------------------------"
echo "Codex"
echo "-----------------------------"

if command -v codex >/dev/null; then
    ok "codex trovato"
    codex --version || true
else
    err "codex non trovato nel PATH"
fi

echo
echo "-----------------------------"
echo "User namespaces"
echo "-----------------------------"

if [ -f /proc/sys/kernel/unprivileged_userns_clone ]; then
    VALUE=$(cat /proc/sys/kernel/unprivileged_userns_clone)
    echo "kernel.unprivileged_userns_clone = $VALUE"

    if [ "$VALUE" = "1" ]; then
        ok "user namespaces abilitate"
    else
        warn "user namespaces DISABILITATE"
    fi
fi

if [ -f /proc/sys/kernel/apparmor_restrict_unprivileged_userns ]; then
    VALUE=$(cat /proc/sys/kernel/apparmor_restrict_unprivileged_userns)
    echo "kernel.apparmor_restrict_unprivileged_userns = $VALUE"

    if [ "$VALUE" = "1" ]; then
        warn "AppArmor restringe gli user namespace"
    else
        ok "AppArmor non li restringe"
    fi
fi

echo
echo "-----------------------------"
echo "AppArmor"
echo "-----------------------------"

if systemctl is-active --quiet apparmor; then
    ok "servizio AppArmor attivo"
else
    warn "servizio AppArmor NON attivo"
fi

if [ -f /etc/apparmor.d/bwrap-userns-restrict ]; then
    ok "profilo bwrap presente"
else
    warn "profilo /etc/apparmor.d/bwrap-userns-restrict assente"
fi

echo
echo "-----------------------------"
echo "Test bubblewrap"
echo "-----------------------------"

if command -v bwrap >/dev/null; then
    if bwrap \
        --ro-bind /usr /usr \
        --ro-bind /bin /bin \
        --proc /proc \
        --dev /dev \
        /bin/sh -c 'echo sandbox-ok' >/tmp/bwrap-test.out 2>/tmp/bwrap-test.err
    then
        ok "Bubblewrap funziona"
        cat /tmp/bwrap-test.out
    else
        err "Bubblewrap NON funziona"
        echo
        cat /tmp/bwrap-test.err
    fi
fi

echo
echo "======================================"
echo " Diagnostica completata"
echo "======================================"