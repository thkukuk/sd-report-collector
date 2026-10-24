FROM registry.opensuse.org/opensuse/tumbleweed:latest AS build-stage
RUN zypper install --no-recommends --auto-agree-with-product-licenses -y git go meson libeconf-devel
COPY . sd-report-collector
RUN cd sd-report-collector && rm -rf container-build && meson setup container-build && meson compile -C container-build && meson test -C container-build
RUN cd sd-report-collector && meson install -C container-build --no-rebuild --destdir /sd-report-collector-install

FROM registry.opensuse.org/opensuse/busybox:latest
LABEL maintainer="Thorsten Kukuk <kukuk@thkukuk.de>"

ARG BUILDTIME=
ARG VERSION=unreleased
LABEL org.opencontainers.image.title="Systemd Report Collector"
LABEL org.opencontainers.image.description="This service accepts systemd reports pushed over mTLS, stores each one zstd-compressed under a per-hostname directory, and runs configured plugins against the saved file."
LABEL org.opencontainers.image.created=$BUILDTIME
LABEL org.opencontainers.image.version=$VERSION

# libeconf.so.0 is a runtime dependency of the binaries below; busybox doesn't ship it.
COPY --from=build-stage /usr/lib64/libeconf.so.0* /usr/lib64/

COPY --from=build-stage /sd-report-collector-install/ /

EXPOSE 8443/tcp

#HEALTHCHECK --interval=60s --timeout=15s \
#            CMD curl --fail http://localhost:8080/healthz || exit 1

CMD ["/usr/lib/sd-report-collector/sd-report-collector"]
