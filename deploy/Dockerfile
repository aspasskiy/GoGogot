FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS builder
ARG TARGETOS TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN curl -sf https://openrouter.ai/api/v1/models -o llm/openrouter/openrouter_models.json
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /gogogot ./cmd

# -----------------------------------------------------------

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    # base
    bash curl wget git ca-certificates gnupg jq \
    # text & search
    ripgrep fd-find grep sed gawk less tree \
    # archives
    zip unzip tar gzip rsync \
    # network
    netcat-openbsd dnsutils iputils-ping traceroute openssh-client \
    # dev runtimes
    python3 python3-pip python3-venv nodejs npm sqlite3 \
    # db clients
    postgresql-client redis-tools \
    # system utils
    htop procps lsof file xxd bc \
    && rm -rf /var/lib/apt/lists/*

RUN pip3 install --no-cache-dir --break-system-packages \
    requests httpx beautifulsoup4 pandas

ARG TARGETARCH
# yq (YAML processor)
RUN curl -fsSL "https://github.com/mikefarah/yq/releases/latest/download/yq_linux_${TARGETARCH}" \
    -o /usr/local/bin/yq && chmod +x /usr/local/bin/yq

# fd-find installs as fdfind on Ubuntu — symlink to fd
RUN ln -sf /usr/bin/fdfind /usr/local/bin/fd

COPY --from=builder /gogogot /usr/local/bin/gogogot

ENV GOGOGOT_DATA_DIR=/data
RUN mkdir -p /data

WORKDIR /work

ENTRYPOINT ["gogogot"]
