FROM golang:1.27-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195

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
