FROM golang:1.25-bookworm@sha256:6359592445455f2dbe2412bed411336035bc019a50017720d77454ffdd6d0f82

ARG USERNAME=vscode
ARG USER_UID=1000
ARG USER_GID=1000

RUN groupadd --gid ${USER_GID} ${USERNAME} \
    && useradd --uid ${USER_UID} --gid ${USER_GID} -m ${USERNAME} \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git mosquitto-clients \
    && rm -rf /var/lib/apt/lists/* \
    && chown -R ${USERNAME}:${USERNAME} /go

WORKDIR /workspace
USER ${USERNAME}
