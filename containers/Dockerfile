# Build with Docker or Podman using this directory as the build context.
# Override with a release tag or digest for reproducible deployments.
ARG OPENCODE_IMAGE=ghcr.io/anomalyco/opencode@sha256:ae90ec960c871b0ea6df9abc1c2ec1e6b36464e8eca9b0e71e1ed3b553f47aa2
FROM ${OPENCODE_IMAGE}

# Tools for working on mounted repositories and launching Node/Python MCPs.
RUN apk add --no-cache ca-certificates git nodejs npm python3 py3-pip iptables ip6tables setpriv \
    && addgroup -g 1000 opencode \
    && adduser -D -u 1000 -G opencode opencode

ARG SERVER_PORT=4096
ARG MCP_PORT=19000
ENV HOME=/home/opencode \
    OPENCODE_SERVER_PORT=${SERVER_PORT} \
    LLM_MODEL_ID=Qwen3.8-27B \
    OPENCODE_CONFIG=/etc/opencode/opencode.json

# Metadata only: publish the required ports explicitly when launching.
EXPOSE ${SERVER_PORT}/tcp
EXPOSE ${MCP_PORT}/tcp

COPY entrypoint.sh /usr/local/bin/opencode-entrypoint
COPY network.py /usr/local/lib/bob/network.py
COPY opencode.json /etc/opencode/opencode.json
RUN chmod 755 /usr/local/bin/opencode-entrypoint

USER root
WORKDIR /home/opencode
ENTRYPOINT ["/usr/local/bin/opencode-entrypoint"]
CMD []
