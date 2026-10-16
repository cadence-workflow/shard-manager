ARG TARGET=server

# Can be used in case a proxy is necessary
ARG GOPROXY

# Build Shard Manager binaries
FROM golang:1.23.4-alpine3.21 AS builder

ARG RELEASE_VERSION

RUN apk add --update --no-cache ca-certificates make git curl mercurial unzip bash

WORKDIR /shard-manager

# Making sure that dependency is not touched
ENV GOFLAGS="-mod=readonly"

# Copy go mod dependencies and try to share the module download cache
COPY go.* ./
COPY cmd/server/go.* ./cmd/server/
# go.work means this downloads everything, not just the top module
RUN go mod download

COPY . .
RUN rm -fr .bin .build

ENV SHARD_MANAGER_RELEASE_VERSION=$RELEASE_VERSION

# don't do anything fancy, just build.  must be run separately, before building things.
RUN make .just-build
RUN CGO_ENABLED=0 make shard-manager-server shard-manager-canary

FROM alpine:3.21 AS alpine

RUN apk add --update --no-cache ca-certificates


# Shard Manager Server
FROM alpine AS shard-manager-server

# WORKDIR plus the default --root/--config/--env flags resolve to these two files.
COPY config/base.yaml /etc/shard-manager/config/base.yaml
COPY config/development.yaml /etc/shard-manager/config/development.yaml
COPY --from=builder /shard-manager/shard-manager-server /usr/local/bin/shard-manager-server

WORKDIR /etc/shard-manager

# The three values a container differs on from development.yaml's loopback defaults.
ENV BIND_ON_LOCALHOST=false \
    DYNAMIC_CONFIG_CLIENT=nop \
    ETCD_ENDPOINTS=etcd:2379

# grpc, pprof
EXPOSE 7943 7942

ENTRYPOINT ["/usr/local/bin/shard-manager-server"]
# shard-distributor is not in ListWithRing, so the --services default is empty.
CMD ["start", "--services", "shard-distributor"]


# Shard Manager Canary: flag-driven, so it needs no config files.
FROM alpine AS shard-manager-canary

COPY --from=builder /shard-manager/shard-manager-canary /usr/local/bin/shard-manager-canary

# No EXPOSE: the ping listener binds 127.0.0.1 and advertises that same address.

ENTRYPOINT ["/usr/local/bin/shard-manager-canary"]
# buildCLI has no app-level Action, so without a subcommand this prints help and exits.
CMD ["start"]


# Final image
FROM shard-manager-${TARGET}
