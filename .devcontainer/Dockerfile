FROM golang:1.27-bookworm@sha256:484ef6066fa69acb059fdfeda7ba2b8f7391f2ef6abc6f9b8411e669ebd56466

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
