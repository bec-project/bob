#!/bin/sh
set -eu

if [ "$#" -eq 0 ]; then
    if [ "${OPENCODE_CONFIG:-/etc/opencode/opencode.json}" = /etc/opencode/opencode.json ]; then
        : "${LLM_BASE_URL:?Set LLM_BASE_URL to the self-hosted API endpoint, including its API path}"
        : "${LLM_MODEL_ID:?Set LLM_MODEL_ID to the model ID served by the provider}"
    fi
    set -- serve --hostname 0.0.0.0 --port "${OPENCODE_SERVER_PORT:-4096}"
fi

# No agent code runs until both address families have an outbound deny policy.
python3 -I /usr/local/lib/bob/network.py

# Preserve runtime-supplied supplementary groups, but remove all capabilities
# permanently before running OpenCode or anything it launches.
case "${BOB_GROUPS:-1000}" in
    keep-groups) set -- --keep-groups opencode "$@" ;;
    *) set -- --groups="${BOB_GROUPS:-1000}" opencode "$@" ;;
esac
exec setpriv --reuid="${BOB_UID:-1000}" --regid="${BOB_GID:-1000}" \
    --bounding-set=-all --inh-caps=-all --ambient-caps=-all \
    --no-new-privs "$@"
