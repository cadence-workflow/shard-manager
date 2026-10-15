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
RUN rm -fr .bin .build idls

ENV SHARD_MANAGER_RELEASE_VERSION=$RELEASE_VERSION

# don't do anything fancy, just build.  must be run separately, before building things.
RUN make .just-build
RUN CGO_ENABLED=0 make shard-manager-server shard-manager-canary


# Shard Manager Server
#
# The binaries are static (CGO_ENABLED=0 above), so the runtime images carry no
# distro: distroless/static supplies only ca-certificates, tzdata and a nonroot
# user. There is no shell, so ENTRYPOINT/CMD must stay in exec form.
FROM gcr.io/distroless/static-debian12:nonroot AS shard-manager-server

# The server resolves config as <root>/<config>/{base,<env>}.yaml, which WORKDIR
# and the default --root/--config/--env flags make
# /etc/shard-manager/config/{base,development}.yaml.
COPY config/base.yaml /etc/shard-manager/config/base.yaml
COPY config/development.yaml /etc/shard-manager/config/development.yaml
COPY --from=builder /shard-manager/shard-manager-server /usr/local/bin/shard-manager-server

WORKDIR /etc/shard-manager

# development.yaml defaults to a loopback dev setup. These are the three values a
# container has to differ on; nop keeps the Cadence dynamicconfig file out of the image.
ENV BIND_ON_LOCALHOST=false \
    DYNAMIC_CONFIG_CLIENT=nop \
    ETCD_ENDPOINTS='["etcd:2379"]'

# grpc, pprof
EXPOSE 7943 7942

USER nonroot
ENTRYPOINT ["/usr/local/bin/shard-manager-server"]
# shard-distributor is the only service in this repo, and it is not in
# ListWithRing, so the --services default is empty and must be given here.
CMD ["start", "--services", "shard-distributor"]


# Shard Manager Canary
#
# Entirely flag-driven, so it needs no config files.
FROM gcr.io/distroless/static-debian12:nonroot AS shard-manager-canary

COPY --from=builder /shard-manager/shard-manager-canary /usr/local/bin/shard-manager-canary

# No EXPOSE: the canary binds its ping listener on 127.0.0.1 and advertises that
# same address to the shard distributor, so the port is not reachable from
# outside the container and publishing it would not help.

USER nonroot
ENTRYPOINT ["/usr/local/bin/shard-manager-canary"]
# buildCLI defines only a start subcommand and no app-level Action, so without
# this the container would print help and exit.
CMD ["start"]


# Final image
FROM shard-manager-${TARGET}
