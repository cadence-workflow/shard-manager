module github.com/cadence-workflow/shard-manager

go 1.23.0

toolchain go1.23.4

require (
	github.com/aws/aws-sdk-go v1.54.12
	github.com/cactus/go-statsd-client/statsd v0.0.0-20191106001114-12b4e2b38748
	github.com/dave/dst v0.26.2
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-farm v0.0.0-20240924180020-3414d57e47da
	github.com/fatih/color v1.13.0
	github.com/gogo/protobuf v1.3.2
	github.com/golang-jwt/jwt/v5 v5.2.0
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.6.0
	github.com/google/uuid v1.6.0
	github.com/hashicorp/go-version v1.2.0
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jonboulle/clockwork v0.5.0
	github.com/m3db/prometheus_client_golang v0.8.1
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709
	github.com/stretchr/testify v1.10.0
	github.com/uber-go/tally v3.3.15+incompatible
	github.com/uber/cadence-idl v0.0.0-20260226231252-039e65827dda
	github.com/uber/ringpop-go v0.8.5
	github.com/uber/tchannel-go v1.22.2 // indirect
	github.com/urfave/cli/v2 v2.27.4
	go.etcd.io/etcd/client/v3 v3.5.5
	go.uber.org/atomic v1.10.0
	go.uber.org/cadence v0.19.0
	go.uber.org/config v1.4.0
	go.uber.org/fx v1.23.0
	go.uber.org/multierr v1.10.0
	go.uber.org/thriftrw v1.29.2
	go.uber.org/yarpc v1.70.3
	go.uber.org/zap v1.26.0
	golang.org/x/exp v0.0.0-20231226003508-02704c960a9b
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/sync v0.14.0
	golang.org/x/time v0.5.0
	golang.org/x/tools v0.22.0
	google.golang.org/grpc v1.59.0
	gopkg.in/validator.v2 v2.0.0-20180514200540-135c24b11c19
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/Masterminds/semver/v3 v3.2.1
	github.com/google/gofuzz v1.0.0
	github.com/mark3labs/mcp-go v0.18.0
	github.com/robfig/cron/v3 v3.0.1
	go.etcd.io/etcd/api/v3 v3.5.5
	go.uber.org/mock v0.5.0
)

require (
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.5 // indirect
	google.golang.org/genproto v0.0.0-20231016165738-49dd2c1f3d0b // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20231012201019-e917dd12ba7a // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

require (
	github.com/BurntSushi/toml v1.3.2 // indirect
	github.com/apache/thrift v0.17.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.4 // indirect
	github.com/cristalhq/jwt/v3 v3.1.0 // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/gogo/googleapis v1.3.2 // indirect
	github.com/gogo/status v1.1.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4
	github.com/m3db/prometheus_client_model v0.1.0 // indirect
	github.com/m3db/prometheus_common v0.1.0 // indirect
	github.com/m3db/prometheus_procfs v0.8.1 // indirect
	github.com/mattn/go-colorable v0.1.9 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.11.1 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.26.0 // indirect
	github.com/prometheus/procfs v0.6.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/uber-go/mapdecode v1.0.0 // indirect
	go.uber.org/dig v1.18.0 // indirect
	go.uber.org/goleak v1.2.0
	go.uber.org/net/metrics v1.3.0 // indirect
	golang.org/x/exp/typeparams v0.0.0-20220218215828-6cf2b201936e // indirect
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/mod v0.18.0
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.25.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231030173426-d783a09b4405 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	honnef.co/go/tools v0.3.2 // indirect
)

// ringpop-go and tchannel-go depends on older version of thrift, yarpc brings up newer version
replace github.com/apache/thrift => github.com/apache/thrift v0.0.0-20161221203622-b2a4d4ae21c7

// DO NOT USE as it misses mysql/config store fix
retract v1.2.3
