module github.com/cadence-workflow/shard-manager

go 1.26.0

toolchain go1.26.10

require (
	github.com/cactus/go-statsd-client/statsd v0.0.0-20191106001114-12b4e2b38748
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-farm v0.0.0-20240924180020-3414d57e47da
	github.com/fatih/color v1.13.0
	github.com/gogo/protobuf v1.3.2
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/golang/mock v1.6.0
	github.com/google/uuid v1.6.0
	github.com/jonboulle/clockwork v0.5.0
	github.com/m3db/prometheus_client_golang v0.8.1
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/stretchr/testify v1.11.1
	github.com/uber-go/tally v3.3.15+incompatible
	github.com/uber/cadence-idl v0.0.0-20260226231252-039e65827dda
	github.com/uber/tchannel-go v1.22.2 // indirect
	github.com/urfave/cli/v3 v3.8.0
	go.etcd.io/etcd/client/v3 v3.5.17
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/config v1.4.0
	go.uber.org/fx v1.23.0
	go.uber.org/multierr v1.10.0
	go.uber.org/thriftrw v1.29.2 // indirect
	go.uber.org/yarpc v1.70.3
	go.uber.org/zap v1.26.0
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sync v0.23.0
	golang.org/x/tools v0.49.0 // indirect
	google.golang.org/grpc v1.83.2
	gopkg.in/validator.v2 v2.0.0-20180514200540-135c24b11c19
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/google/gofuzz v1.0.0
	go.etcd.io/etcd/api/v3 v3.5.17
	go.uber.org/mock v0.5.0
)

require (
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.17 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/tools/go/expect v0.1.1-deprecated // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
)

require (
	github.com/BurntSushi/toml v1.3.2 // indirect
	github.com/apache/thrift v0.17.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/gogo/googleapis v1.3.2 // indirect
	github.com/gogo/status v1.1.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4
	github.com/m3db/prometheus_client_model v0.1.0 // indirect
	github.com/m3db/prometheus_common v0.1.0 // indirect
	github.com/m3db/prometheus_procfs v0.8.1 // indirect
	github.com/mattn/go-colorable v0.1.9 // indirect
	github.com/mattn/go-isatty v0.0.14
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/uber-go/mapdecode v1.0.0 // indirect
	go.uber.org/dig v1.18.0 // indirect
	go.uber.org/goleak v1.3.0
	go.uber.org/net/metrics v1.3.0 // indirect
	golang.org/x/exp/typeparams v0.0.0-20220218215828-6cf2b201936e // indirect
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	honnef.co/go/tools v0.3.2 // indirect
)

// DO NOT USE as it misses mysql/config store fix
retract v1.2.3
