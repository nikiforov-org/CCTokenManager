# What the Linux build needs: GTK 3, libsecret and Go, on Ubuntu 22.04, whose
# older glibc lets the app run on the distributions of the last years.
FROM ubuntu:22.04
ARG GO_VERSION
RUN export DEBIAN_FRONTEND=noninteractive && apt-get update -qq && apt-get install -y -qq --no-install-recommends \
      build-essential pkg-config ca-certificates curl libgtk-3-dev libsecret-1-dev >/dev/null \
 && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-$(dpkg --print-architecture).tar.gz" | tar -C /usr/local -xz
ENV PATH=/usr/local/go/bin:$PATH GOFLAGS=-buildvcs=false
