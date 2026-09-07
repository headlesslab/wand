# The wand development image: the runtime image plus the Go and Node
# toolchains, so that wand's own suite runs inside a container, where
# utils.InContainer holds and the launcher passes --no-sandbox (spec #33,
# section 14; ticket #55).
#
#     docker build -t ghcr.io/headlesslab/wand -f docker/Dockerfile .
#     docker build -t ghcr.io/headlesslab/wand:dev -f docker/dev.Dockerfile \
#         --build-arg base=ghcr.io/headlesslab/wand .
#     docker run --rm -v "$PWD:/wand" ghcr.io/headlesslab/wand:dev \
#         go run ./internal/tools/ci-test -race -count=1 -run=^Test ./...
#
# or both images and the whole in-container run at once:
#
#     go run ./internal/tools/docker -suite

# The runtime image to build on, which has to be built first: no tag of it is
# published yet. The image build script passes the one it has just built, so
# the suite always runs on the Chrome of the image under test.
ARG base="ghcr.io/headlesslab/wand"

# The toolchains come from the official images rather than from tarball URLs,
# and each is a named stage rather than an inline COPY --from, so that
# Dependabot sees a FROM line for it and moves the digest. Both are the
# bookworm variant: its glibc is older than noble's, so what is copied below
# runs there.
FROM golang:1.27-bookworm@sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b AS golang

FROM node:24-bookworm@sha256:be23f54a88d34e8824c741b19b91064094f92c1c97b194144bfc8b50d67258e2 AS node

FROM $base

LABEL org.opencontainers.image.title="wand-dev"
LABEL org.opencontainers.image.description="The wand container image with the Go and Node toolchains, for running wand's own suite and generators inside a container."
LABEL org.opencontainers.image.source="https://github.com/headlesslab/wand"
LABEL org.opencontainers.image.licenses="MIT"

# The Ubuntu sources come with the base image, so a runtime image built with
# an apt_mirror hands its mirror down and this one needs no argument of its
# own.
RUN set -eux; \
    apt-get update > /dev/null; \
    apt-get install --no-install-recommends -y \
    # -race links against the C runtime, and the launcher's suite builds
    # fixture binaries of its own
    build-essential \
    # the pins generator asks the devtools-protocol remote for its tags
    git \
    > /dev/null; \
    rm -rf /var/lib/apt/lists/*

COPY --from=golang /usr/local/go /usr/local/go

COPY --from=node /usr/local/bin/node /usr/local/bin/node

COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules

# The two launchers the node image itself links, relative so that they follow
# /usr/local wherever it is mounted.
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

ENV PATH="/usr/local/go/bin:/root/go/bin:${PATH}"

# Never download a newer toolchain, as in the builder of the runtime image.
ENV GOTOOLCHAIN=local

# A checkout mounted from the host belongs to another user, which git refuses
# to touch until it is told the ownership is expected.
RUN git config --global --add safe.directory '*'

# Proof that the three toolchains run on this base, at build time rather than
# on the first docker run.
RUN go version && node --version && npm --version

# Where the image Gate mounts the checkout.
WORKDIR /wand

CMD ["bash"]
