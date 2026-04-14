#[=======================================================================[
# Description : Docker image containing the aura binary
#]=======================================================================]

ARG GO_VERSION=1.26.2
ARG DISTRO=trixie

#### ---- Build ---- ####
FROM golang:${GO_VERSION}-${DISTRO} AS build

LABEL maintainer=arash.idelchi

USER root

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    git \
    && rm -rf /var/lib/apt/lists/*

# Create User (Debian/Ubuntu)
ARG USER=user
ARG UID=1001
RUN groupadd -r -g ${UID} ${USER} && \
    useradd -r -u ${UID} -g ${UID} -m -c "${USER} account" -d /home/${USER} -s /bin/bash ${USER}

ARG CACHE_BUST=10
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b /usr/local/bin && \
    RTK_INSTALL_DIR=/usr/local/bin sh -c "$(curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh)"

RUN go install golang.org/x/tools/gopls@latest && \
    mv /go/bin/gopls /usr/local/bin/gopls

USER ${USER}
WORKDIR /tmp/go

ENV GOMODCACHE=/home/${USER}/.cache/.go-mod
ENV GOCACHE=/home/${USER}/.cache/.go

RUN mkdir -p /home/${USER}/.local/bin

COPY go.mod go.sum ./
COPY sdk/go.mod sdk/go.mod
RUN --mount=type=cache,target=${GOMODCACHE},uid=1001,gid=1001 \
    --mount=type=cache,target=${GOCACHE},uid=1001,gid=1001 \
    go mod download

ARG TARGETOS TARGETARCH

COPY . .
ARG AURA_VERSION="unofficial & built by unknown"
ARG BUILD_TAGS=""
RUN --mount=type=cache,target=${GOMODCACHE},uid=${UID},gid=${UID},id=gomod-${TARGETARCH} \
    --mount=type=cache,target=${GOCACHE},uid=${UID},gid=${UID},id=gocache-${TARGETARCH} \
    CGO_ENABLED=1 go build ${BUILD_TAGS} -ldflags="-s -w -X 'main.version=${AURA_VERSION}'" -o /home/${USER}/.local/bin/aura .

ENV PATH=$PATH:/home/${USER}/.local/bin
ENV PATH=$PATH:/root/.local/bin
ENV XDG_RUNTIME_DIR=/tmp/${UID}
ENV XDG_CONFIG_HOME=/home/${USER}/.config
ENV XDG_CACHE_HOME=/home/${USER}/.cache

WORKDIR /tmp
USER root
RUN rm -rf /tmp/go /go

USER ${USER}
WORKDIR /home/${USER}

# Timezone
ENV TZ=Europe/Zurich

FROM debian:trixie-slim AS final

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    jq \
    yq \
    nano \
    file \
    patch \
    ripgrep \
    xclip \
    poppler-utils \
    && rm -rf /var/lib/apt/lists/*

# Create User (Debian/Ubuntu)
ARG USER=user
ARG UID=1001
RUN groupadd -r -g ${UID} ${USER} && \
    useradd -r -u ${UID} -g ${UID} -m -c "${USER} account" -d /home/${USER} -s /bin/bash ${USER}

USER ${USER}
WORKDIR /home/${USER}

COPY --from=build --chown=${USER}:${USER} /home/${USER}/.local/bin/aura /home/${USER}/.local/bin/aura
COPY --from=build /usr/local/bin/golangci-lint /usr/local/bin/golangci-lint
COPY --from=build /usr/local/bin/rtk /usr/local/bin/rtk
COPY --from=build /usr/local/bin/gopls /usr/local/bin/gopls

ENV PATH=$PATH:/home/${USER}/.local/bin
ENV PATH=$PATH:/root/.local/bin
ENV XDG_RUNTIME_DIR=/tmp/${UID}
ENV XDG_CONFIG_HOME=/home/${USER}/.config
ENV XDG_CACHE_HOME=/home/${USER}/.cache

# Timezone
ENV TZ=Europe/Zurich

RUN mkdir -p /home/${USER}/.aura
RUN mkdir -p /home/${USER}/.cache
RUN mkdir -p  /home/${USER}/go

ENV GOPATH=/home/${USER}/go
ENV GOMODCACHE="/home/${USER}/go/pkg/mod"
ENV EDITOR=nano
