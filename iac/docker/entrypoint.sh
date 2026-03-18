#!/bin/sh
set -e

# -------------------------------------------------------
# PUID / PGID auto-permission entrypoint
#
# If PUID or PGID are set, the internal "dnsreconciler"
# user/group is re-created with the requested IDs.
# Ownership of /config and /state is fixed, then the
# application is launched under the target user via
# su-exec (no PID overhead, signal-transparent).
#
# When neither variable is set the defaults baked into
# the image (911:911) are used.
# -------------------------------------------------------

PUID="${PUID:-911}"
PGID="${PGID:-911}"

USERNAME="dnsreconciler"
GROUPNAME="dnsreconciler"

# ---- Reconcile user/group ----
# Delete user first (so the group has no members), then group,
# then recreate both with the requested IDs.
CURRENT_UID="$(id -u "${USERNAME}" 2>/dev/null || echo "")"
CURRENT_GID="$(id -g "${USERNAME}" 2>/dev/null || echo "")"

if [ "${CURRENT_UID}" != "${PUID}" ] || [ "${CURRENT_GID}" != "${PGID}" ]; then
    deluser "${USERNAME}" 2>/dev/null || true
    delgroup "${GROUPNAME}" 2>/dev/null || true
    addgroup -g "${PGID}" -S "${GROUPNAME}"
    adduser -u "${PUID}" -G "${GROUPNAME}" -S -H -D "${USERNAME}"
fi

echo "Running as uid=${PUID}(${USERNAME}) gid=${PGID}(${GROUPNAME})"

# ---- Fix ownership on mount points ----
chown -R "${PUID}:${PGID}" /config /state

# ---- Drop privileges and exec the application ----
exec su-exec "${USERNAME}" "$@"

