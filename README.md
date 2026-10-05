# sd-report-collector

Server counterpart to `systemd-report upload`
(see [systemd-report](https://manpages.opensuse.org/systemd-report.1)).
Accepts report pushes over mTLS, stores each one zstd-compressed under a
per-hostname directory, and runs configured plugins against the saved file.

Report signature verification (the client's optional `--sign=`) is out of
scope for the core daemon; a plugin can implement it if needed.

## Build

Requires `libeconf` development headers (`libeconf-devel` / `libeconf-dev`)
and a C compiler, since configuration parsing uses cgo bindings to libeconf.

```
make
make test
```

## Configuration

See `dist/sd-report-collector.conf` for all keys and their defaults.

## Certificate management

`sd-report-certs` generates the CA, server certificate, and client
certificates needed for mTLS, storing everything below
`/etc/sd-report-collector`:

```
sd-report-certs init          # generates ca.pem/ca.key and server.pem/server.key
sd-report-certs client myhost # generates clients/myhost.pem and clients/myhost.key,
                              # signed by the CA from init
```

`init` must be run once (as root, since it writes under `/etc`) before any
`client` certificates can be issued. Both subcommands refuse to overwrite
existing files unless `--force` is given; forcing `init` invalidates every
certificate previously signed by that CA. `init` also writes
`sd-report-collector.conf.d/certgen.conf`, a config drop-in pointing
`ServerCertificateFile`, `ServerKeyFile`, and `ClientCAFile` at the generated
`server.pem`/`server.key`/`ca.pem`.
Each `clients/<name>.pem`/`clients/<name>.key` pair needs to be copied to
the matching client (`myhost` here matches the hostname systemd-report will
report as).

## Dashboard plugin

`sd-report-dashboard` is an sd-report-collector plugin that renders a saved
report as a static, self-contained HTML dashboard. Point a `[Plugins]` entry
at the built binary:

```
[Plugins]
dashboard=/usr/lib/sd-report-collector/sd-report-dashboard
```

It writes `<OutputDirectory>/<hostname>.html`, overwriting that host's
dashboard in place each time a new report arrives. The HTML template is
compiled into the binary, but can be overriden via `sd-report-dashboard.conf`:

```
[Dashboard]
OutputDirectory=/srv/www/html/dashboards
TemplateFile=/etc/sd-report-dashboard/dashboard_template.html
DescribeFile=/etc/sd-report-dashboard/report.describe
```

`TemplateFile` and `DescribeFile` are optional; leaving them unset uses the
built-in template and omits per-metric type/description annotations,
respectively. `DescribeFile` should point at the output of
`systemd-report describe`.

## Quickstart

Generate the CA, server cert, and a client cert under /etc/sd-report-collector and start the service:

```sh
sd-report-certs init
sd-report-certs client myhost.example.com

systemctl start sd-report-collector.service
```

On the client call `systemd-report`:
```sh
/usr/lib/systemd/systemd-report upload --url=https://<server>:8443/report --cert=/etc/sd-report-collector/clients/myhost.pem --key=/etc/sd-report-collector/clients/myhost.key --trust=/etc/sd-report-collector/ca.pem
```
