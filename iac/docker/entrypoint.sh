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

# ---- Docker socket access ----
# If the Docker socket is mounted, detect its GID and add the user to
# that group so container discovery works without running as root.
DOCKER_SOCK="/var/run/docker.sock"
if [ -S "${DOCKER_SOCK}" ]; then
    SOCK_GID="$(stat -c '%g' "${DOCKER_SOCK}")"
    # Create a group with the socket's GID (if it doesn't already exist)
    DOCKER_GROUP="dockersock"
    if ! getent group "${SOCK_GID}" >/dev/null 2>&1; then
        addgroup -g "${SOCK_GID}" -S "${DOCKER_GROUP}"
    else
        DOCKER_GROUP="$(getent group "${SOCK_GID}" | cut -d: -f1)"
    fi
    addgroup "${USERNAME}" "${DOCKER_GROUP}" 2>/dev/null || true
fi

# ---- Fix ownership on mount points ----
chown -R "${PUID}:${PGID}" /config /state

# ---- Drop privileges and exec the application ----
exec su-exec "${USERNAME}" "$@"

