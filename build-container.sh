#!/bin/bash

set -e

VERSION=$(grep -oP "^\s*version\s*:\s*'\K[^']+" meson.build)
VER=( ${VERSION//./ } )

podman pull registry.opensuse.org/opensuse/busybox:latest
podman build --rm --no-cache --layers=false --build-arg VERSION="${VERSION}" --build-arg BUILDTIME=$(date +%Y-%m-%dT%TZ) -t sd-report-collector .
podman tag localhost/sd-report-collector sd-report-collector:"${VERSION}"
podman tag localhost/sd-report-collector sd-report-collector:latest
podman tag localhost/sd-report-collector sd-report-collector:"${VER[0]}"
podman tag localhost/sd-report-collector sd-report-collector:"${VER[0]}.${VER[1]}"
