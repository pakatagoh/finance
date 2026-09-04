# syntax=docker/dockerfile:1

# Build with the same Debian/glibc family as the production runtime. The
# generated templ output and CSS are produced from the checked-in sources here.
ARG GO_VERSION=1.27.1
FROM golang:${GO_VERSION}-bookworm AS build

ARG TEMPL_VERSION=v0.3.960
ARG TAILWIND_VERSION=v4.3.3
ARG TAILWIND_SHA256=dc61b3ac6b8c9ca874c0cc4c57b2409791a64c5540404ca5f5367360babc313a

RUN apt-get update \
    && apt-get install --no-install-recommends -y ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

RUN go install github.com/a-h/templ/cmd/templ@${TEMPL_VERSION} \
    && curl --fail --silent --show-error --location \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-x64" \
      --output /usr/local/bin/tailwindcss \
    && echo "${TAILWIND_SHA256}  /usr/local/bin/tailwindcss" | sha256sum --check \
    && chmod 0755 /usr/local/bin/tailwindcss

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN templ generate \
    && tailwindcss -i assets/css/input.css -o static/css/app.css --minify \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/finance ./cmd/finance

# Keep the runtime image Debian/glibc-based for parity with the build image and
# include only the executable and browser assets needed by the server.
FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install --no-install-recommends -y ca-certificates tzdata \
    && groupadd --gid 1000 finance \
    && useradd --uid 1000 --gid 1000 --create-home --home-dir /home/finance --shell /usr/sbin/nologin finance \
    && mkdir --parents /app \
    && chown 1000:1000 /app \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build --chown=1000:1000 /out/finance /app/finance
COPY --from=build --chown=1000:1000 /src/static /app/static

USER 1000:1000
EXPOSE 8080
ENTRYPOINT ["/app/finance"]
CMD ["serve"]
