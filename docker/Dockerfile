# The wand container image: the Target Chrome and the remote-launch manager on
# ubuntu:noble, one image per architecture built natively on its own runner
# (spec #33, section 14; ADR-0005; ticket #55).
#
#     docker build -t ghcr.io/headlesslab/wand -f docker/Dockerfile .
#
# or, with the :dev image and the checks the image Gate runs beside it:
#
#     go run ./internal/tools/docker
#
# Both base images are pinned by digest, so a rebuild takes the same bytes
# until Dependabot moves the pin (spec #33, section 13).

# The builder holds Go and the module cache; nothing of it reaches the image
# below except the manager binary and the browser it downloaded.
FROM golang:1.27-bookworm@sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b AS build

# Never download a newer toolchain: a dependency raising its Go floor above
# this image's must fail the build, not silently pull a toolchain.
ENV GOTOOLCHAIN=local

# The manager is built without cgo, so the binary this bookworm builder
# produces does not carry that distribution's glibc into noble below.
ENV CGO_ENABLED=0

# The two nested example modules of go.work take no part in the image, and
# resolving the workspace would download their dependencies as well.
ENV GOWORK=off

# The browser cache is a fixed path rather than one under $HOME, so the image
# serves whatever user the container ends up running as.
ENV WAND_BROWSER_CACHE=/opt/wand/browser

# A Go module proxy for a build behind a restricted network; empty leaves
# Go's own default.
ARG goproxy=""

RUN if [ -n "$goproxy" ]; then go env -w GOPROXY="$goproxy"; fi

WORKDIR /src

COPY . .

RUN go build -trimpath -o /out/wand-manager ./cmd/wand-manager

# The Target Chrome the pins package names, taken through Browser resolution's
# own download path and verified against the pinned hash before extraction, so
# the browser in the image is the archive the Tier 1 jobs ran on. The version
# is a pin and not a lookup, which is what makes the two architectures carry
# the same one. The path the command prints becomes /usr/bin/chrome below.
RUN go run ./cmd/wand-fetch-browser > /out/browser-path

FROM ubuntu:noble@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517

LABEL org.opencontainers.image.title="wand"
LABEL org.opencontainers.image.description="A Chrome DevTools Protocol driver for browser automation and web scraping in Go, with the Target Chrome inside."
LABEL org.opencontainers.image.source="https://github.com/headlesslab/wand"
LABEL org.opencontainers.image.licenses="MIT"

# An Ubuntu mirror for a build behind a restricted network; empty leaves the
# distribution's own hosts. noble keeps its sources in the deb822 file, and
# the two architectures start from different hosts: archive.ubuntu.com on
# amd64, ports.ubuntu.com on arm64.
ARG apt_mirror=""

RUN set -eux; \
    if [ -n "$apt_mirror" ]; then \
        sed -i "s|http://archive.ubuntu.com/ubuntu|$apt_mirror|g; s|http://ports.ubuntu.com/ubuntu-ports|$apt_mirror|g" \
            /etc/apt/sources.list.d/ubuntu.sources; \
    fi; \
    apt-get update > /dev/null; \
    apt-get install --no-install-recommends -y \
    # Chrome's runtime dependencies
    libnss3 \
    libxss1 \
    libasound2t64 \
    libxtst6 \
    libgtk-3-0 \
    libgbm1 \
    ca-certificates \
    # fonts: Liberation for the metric-compatible defaults, Noto for the
    # scripts fixtures/fonts.html prints, and the colour emoji
    fonts-liberation \
    fonts-noto-core \
    fonts-noto-color-emoji \
    fonts-noto-cjk \
    # timezone
    tzdata \
    # process reaper
    dumb-init \
    # a visible browser needs a display: $ xvfb-run wand-manager
    xvfb \
    > /dev/null; \
    rm -rf /var/lib/apt/lists/*

# utils.InContainer, which the launcher turns into --no-sandbox: Docker writes
# this file itself when it runs a container, and writing it here covers a
# runtime that does not.
RUN touch /.dockerenv

# Which Noto family fontconfig hands a page that asks for serif, sans-serif or
# monospace, so the PDF TestFonts prints renders the same on both
# architectures.
COPY docker/fonts-local.conf /etc/fonts/local.conf

COPY --from=build /out/wand-manager /usr/bin/wand-manager

# The browser cache the pre-fetch command filled, at the same fixed path, so
# that Browser resolution and the manager's binary-path whitelist both find
# it. /usr/bin/chrome is the same binary under the first name System browser
# discovery tries, for a container that runs as a user with a cache directory
# of its own; running it proves at build time that its runtime dependencies
# above are complete.
ENV WAND_BROWSER_CACHE=/opt/wand/browser

COPY --from=build /opt/wand/browser /opt/wand/browser

COPY --from=build /out/browser-path /tmp/browser-path

RUN ln -s "$(cat /tmp/browser-path)" /usr/bin/chrome && rm /tmp/browser-path && chrome --version

# The manager's default address.
EXPOSE 7317

# dumb-init is PID 1 and reaps the browsers a launch leaves behind.
ENTRYPOINT ["dumb-init", "--"]

CMD ["wand-manager"]
