# Shard Manager
[![Build Status](https://github.com/cadence-workflow/shard-manager/actions/workflows/ci-checks.yml/badge.svg)](https://github.com/cadence-workflow/shard-manager/actions/workflows/ci-checks.yml)
[![Slack Status](https://img.shields.io/badge/slack-join_chat-white.svg?logo=slack&style=social)](https://communityinviter.com/apps/cloud-native/cncf)
[![License](https://img.shields.io/github/license/cadence-workflow/shard-manager.svg)](http://www.apache.org/licenses/LICENSE-2.0)

Shard Manager is a service that assigns shards to the hosts of a sharded application and keeps that
assignment balanced and up to date as hosts come and go. Applications hand over ownership decisions.

The shard manager go service is backed by etcd, additionally we provide Go client libraries that
applications embed:

* **Executor client** — a host that *runs* shards. It heartbeats to the distributor, receives its
  shard assignments, and starts/stops a `ShardProcessor` per assigned shard.
* **Spectator client** — a host that only needs to *route*. It watches the assignment and answers
  `GetShardOwner(shardKey)`, and can be plugged in as a YARPC peer chooser to route requests
  straight to the owning host.

Namespaces are the unit of configuration. A namespace is either:

* `fixed` — a static set of shards (`shardNum`), distributed across the executors of that namespace.
* `ephemeral` — shards are created on demand, the first time someone asks for a shard key.

Rebalancing is pluggable per namespace via the `naive` or `greedy` load balancer (see
`service/sharddistributor/loadbalancer`), tunable through dynamic config.

## Getting Started

The distributor needs an etcd cluster. The quickest way to get one locally:

```
docker compose -f docker/github_actions/docker-compose.yml up -d etcd
```

Build the binaries and start the server:

```
make bins
./shard-manager-server start --services shard-distributor
```

It reads `config/development.yaml` by default, which points at `localhost:2379` and defines a
handful of development namespaces. The gRPC API is served on port `7943`.

To see it doing something, run the canary — it spins up executors and a pinger against the
`shard-distributor-canary` (fixed) and `shard-distributor-canary-ephemeral` namespaces:

```
make start-shard-manager-canary
```

### CLI

`smctl` is the command-line client for inspecting and operating a running distributor:

```
make smctl
./smctl --address localhost:7943 namespace list
./smctl --address localhost:7943 --namespace shard-distributor-canary executor list
./smctl --address localhost:7943 --namespace shard-distributor-canary shard inspect --shard-key 7
```

The root flags (`--namespace`, `--address`, `--transport`, `--tls-cert-path`, `--context-timeout`)
also read from `SMCTL_*` environment variables. Use `--help` on any subcommand to explore.

### Using it from your application

Wire in one of the clients under `service/sharddistributor/client` with fx:

* `executorclient` — implement `ShardProcessor` (start/stop work for one shard) and
  `ShardProcessorFactory`, and the executor takes care of heartbeating, assignment updates and
  draining.
* `spectatorclient` — for callers that only need to find the owner of a shard key, including a
  ready-made YARPC peer chooser.

`service/sharddistributor/canary` is a small, complete example of both.

## Repository Layout

| Path | What lives there |
| --- | --- |
| `service/sharddistributor` | The distributor service: handler, leader election, stores, load balancers, clients, canary |
| `service/sharddistributor/store/etcd` | etcd-backed shard/executor/leader state |
| `cmd/server` | The server binary (`shard-manager-server`) |
| `cmd/smctl`, `tools/smctl` | The `smctl` CLI |
| `cmd/sharddistributor-canary` | The canary binary (`shard-manager-canary`) |
| `common` | Shared libraries inherited from Cadence: config, log, metrics, rpc, dynamic config, types |
| `common/types` | Internal RPC types and mappers — service logic uses these, never generated IDL types |
| `proto`, `idls` | API definitions |

## Development

```
make bins    # build all binaries
make test    # run unit tests
make lint    # run the linter
make pr      # codegen + lint + fmt + tidy, run this before opening a PR
```

Tests that need etcd are guarded by `testflags.RequireEtcd` and run via `make integration_tests_etcd`
with the etcd compose file above running.


