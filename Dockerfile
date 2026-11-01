# SluiceOps container image.
#
# The final stage is based on the official HAProxy image so that the same
# haproxy binary is available for "haproxy -c" validation. Build one image per
# HAProxy version you run, e.g.:
#
#   docker build --build-arg HAPROXY_VERSION=2.9 -t sluiceops:haproxy2.9 .
#
ARG HAPROXY_VERSION=3.2

# --- web UI -------------------------------------------------------------------
# Build stages run natively on the build machine (no emulation); only the
# final stage is per target platform.
FROM --platform=$BUILDPLATFORM node:22-alpine AS ui
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# --- Go binary ----------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /src/web/dist ./web/dist
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/sluiceops ./cmd/sluiceops

# --- runtime --------------------------------------------------------------------
FROM haproxy:${HAPROXY_VERSION}-alpine
USER root
COPY --from=build /out/sluiceops /usr/local/bin/sluiceops
RUN mkdir -p /var/lib/sluiceops && chown haproxy:haproxy /var/lib/sluiceops
# Runs as the same user as HAProxy (uid 99) so both can use the master socket.
USER haproxy
ENV SLUICEOPS_HAPROXY_BIN=/usr/local/sbin/haproxy \
    SLUICEOPS_CONFIG=/usr/local/etc/haproxy/haproxy.cfg \
    SLUICEOPS_DATA_DIR=/var/lib/sluiceops \
    SLUICEOPS_LISTEN=:8484
EXPOSE 8484
VOLUME /var/lib/sluiceops
HEALTHCHECK --interval=30s --timeout=5s CMD wget -q -O /dev/null http://127.0.0.1:8484/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/sluiceops"]
