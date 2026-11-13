module sd-report-collector

go 1.27.1

require (
	github.com/influxdata/influxdb-client-go/v2 v2.14.0
	github.com/klauspost/compress v1.20.3
	github.com/openSUSE/libeconf/go/econf v0.0.0-20261111102138-301ef79a89d9
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/influxdata/line-protocol v0.0.0-20210922203350-b1ad95c89adf // indirect
	github.com/oapi-codegen/runtime v1.8.0 // indirect
	golang.org/x/net v0.61.0 // indirect
)

replace github.com/openSUSE/libeconf/go/econf => github.com/openSUSE/libeconf/go/native/econf v0.0.0-20261111102138-301ef79a89d9
