# To build the image:
#     docker build -t ghcr.io/headlesslab/wand -f docker/Dockerfile .
#

# build wand-manager
FROM golang as go

ARG goproxy="https://proxy.golang.org,direct"

COPY . /wand
WORKDIR /wand
RUN go env -w GOPROXY=$goproxy
RUN go build ./cmd/wand-manager
RUN go run ./cmd/wand-fetch-browser

FROM ubuntu:noble

COPY --from=go /root/.cache/rod /root/.cache/rod
RUN ln -s /root/.cache/rod/browser/$(ls /root/.cache/rod/browser)/chrome /usr/bin/chrome

RUN touch /.dockerenv

COPY --from=go /wand/wand-manager /usr/bin/

ARG apt_sources="http://archive.ubuntu.com"

RUN sed -i "s|http://archive.ubuntu.com|$apt_sources|g" /etc/apt/sources.list && \
    apt-get update > /dev/null && \
    apt-get install --no-install-recommends -y \
    # chromium dependencies
    libnss3 \
    libxss1 \
    libasound2t64 \
    libxtst6 \
    libgtk-3-0 \
    libgbm1 \
    ca-certificates \
    # fonts
    fonts-liberation fonts-noto-color-emoji fonts-noto-cjk \
    # timezone
    tzdata \
    # process reaper
    dumb-init \
    # headful mode support, for example: $ xvfb-run chromium-browser --remote-debugging-port=9222
    xvfb \
    > /dev/null && \
    # cleanup
    rm -rf /var/lib/apt/lists/*

# process reaper
ENTRYPOINT ["dumb-init", "--"]

CMD wand-manager
