BINARIES   := sd-report-collector sd-report-certs sd-report-dashboard
BUILD_DIR  := bin

PREFIX                  ?= /usr
BINDIR                  ?= $(PREFIX)/bin
VENDOR_LIBDIR           ?= $(PREFIX)/lib/sd-report-collector
VENDOR_DASHBOARD_LIBDIR ?= $(PREFIX)/lib/sd-report-dashboard
UNITDIR                 ?= $(PREFIX)/lib/systemd/system

GO      ?= go
GOFLAGS ?=

.PHONY: all build test install tidy update fmt clean

all: build

build:
	for bin in $(BINARIES); do \
		$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$$bin ./cmd/$$bin; \
	done

test:
	$(GO) test $(GOFLAGS) ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

update: ## Get and update the dependencies
	$(GO) get -v -u ./...

install: build
	install -D -m 0755 $(BUILD_DIR)/sd-report-certs $(DESTDIR)$(BINDIR)/sd-report-certs
	install -D -m 0755 $(BUILD_DIR)/sd-report-collector $(DESTDIR)$(VENDOR_LIBDIR)/sd-report-collector
	install -D -m 0644 dist/sd-report-collector.conf $(DESTDIR)$(VENDOR_LIBDIR)/sd-report-collector.conf
	install -D -m 0644 dist/sd-report-collector.service $(DESTDIR)$(UNITDIR)/sd-report-collector.service
	install -D -m 0755 $(BUILD_DIR)/sd-report-dashboard $(DESTDIR)$(VENDOR_DASHBOARD_LIBDIR)/sd-report-dashboard
	install -D -m 0644 dist/sd-report-dashboard.conf $(DESTDIR)$(VENDOR_DASHBOARD_LIBDIR)/sd-report-dashboard.conf

clean:
	rm -rf $(BUILD_DIR)
