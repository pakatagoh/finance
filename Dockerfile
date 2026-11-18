# syntax=docker/dockerfile:1

# Use the shared Mise tool pins while retaining Bookworm's existing CA policy.
# The scratch image supplies only the verified static Mise binary, not its OS.
FROM debian:bookworm-slim AS build
ARG TARGETARCH
RUN test "$TARGETARCH" = amd64 \
    || { echo "Only linux/amd64 is locked and validated for this image" >&2; exit 1; }
COPY --from=ghcr.io/jdx/mise:2026.10.4@sha256:1e0a39bc5a3b089b9b9305b6cd53e5d32ed5327e6241db5ec412fc2eabd44b02 /usr/local/bin/mise /usr/local/bin/mise

COPY build/check-ca-certificates.sh /usr/local/bin/check-ca-certificates
RUN rm -rf /var/lib/apt/lists/* \
    && apt-get update \
    && apt-get install --no-install-recommends -y ca-certificates curl git \
    && sh /usr/local/bin/check-ca-certificates \
    && rm -rf /var/lib/apt/lists/*

ENV MISE_ENV=build \
    MISE_DATA_DIR=/mise \
    MISE_CONFIG_DIR=/mise/config \
    MISE_CACHE_DIR=/mise/cache \
    CGO_ENABLED=0 \
    GOTOOLCHAIN=local

# Keep config discovery outside /src; never load development or personal config.
# Tool installation is cached separately from application/module changes.
WORKDIR /toolchain
COPY mise.toml mise.lock mise.build.toml mise.build.lock ./
RUN mise trust && mise install --locked

WORKDIR /src
COPY go.mod go.sum ./
RUN cd /toolchain \
    && mise exec -- sh -ec 'cd /src; go mod download'
COPY . .
RUN cd /toolchain \
    && mise exec -- sh -ec '\
        cd /src; \
        templ generate; \
        tailwindcss -i assets/css/input.css -o static/css/app.css --minify; \
        mkdir -p /out; \
        GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/finance ./cmd/finance; \
        cp "$(command -v supercronic)" /out/supercronic \
    '

# Shared web/categorization runtime; no Mise, Go, scheduler or browser assets yet.
FROM debian:bookworm-slim AS finance-base
COPY build/check-ca-certificates.sh /usr/local/bin/check-ca-certificates
RUN rm -rf /var/lib/apt/lists/* \
    && apt-get update \
    && apt-get install --no-install-recommends -y ca-certificates tzdata \
    && sh /usr/local/bin/check-ca-certificates \
    && groupadd --gid 1000 finance \
    && useradd --uid 1000 --gid 1000 --create-home --home-dir /home/finance --shell /usr/sbin/nologin finance \
    && mkdir --parents /app \
    && chown 1000:1000 /app \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build --chown=1000:1000 /out/finance /app/finance

# Supply a read-only crontab at runtime; this image does not create schedules.
FROM finance-base AS categorizer
COPY --from=build /out/supercronic /usr/local/bin/supercronic
USER 1000:1000
ENTRYPOINT ["/usr/local/bin/supercronic"]
CMD ["/etc/finance/categorize.crontab"]

# Match the database's PostgreSQL major/client release; never start its server.
# The upstream image declares /var/lib/postgresql as a VOLUME. Mount it as tmpfs
# in worker deployments/tests to avoid accumulating unused anonymous DB volumes.
FROM postgres:18.6-bookworm AS backup
COPY build/check-ca-certificates.sh /usr/local/bin/check-ca-certificates
RUN rm -rf /var/lib/apt/lists/* \
    && apt-get update \
    && apt-get install --no-install-recommends -y ca-certificates tzdata \
    && sh /usr/local/bin/check-ca-certificates \
    && install -d -o postgres -g postgres /backups \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /backups
COPY --from=build /out/supercronic /usr/local/bin/supercronic
USER postgres
ENTRYPOINT ["/usr/local/bin/supercronic"]
CMD ["/etc/finance/backup.crontab"]

# Keep this last: plain `docker build .` must still produce the existing web app.
FROM finance-base AS runtime
COPY --from=build --chown=1000:1000 /src/static /app/static
USER 1000:1000
EXPOSE 8080
ENTRYPOINT ["/app/finance"]
CMD ["serve"]
