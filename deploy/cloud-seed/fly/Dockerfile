# Starmesh cloud seed image for Fly.io (or any container host).
# Fly hands every app a free, dedicated IPv6 and a shared IPv4, which is
# enough to run a last-resort seed reachable over QUIC (UDP) and TLS (TCP).
FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/starmesh ./hub/cmd/starmesh

FROM debian:12-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/starmesh /usr/local/bin/starmesh
ENV STARMESH_HOME=/data
ENV SEED_NAME=seed-fly
VOLUME ["/data"]
EXPOSE 4433/udp
EXPOSE 4433/tcp
# A seed has no Starlink IPv6, so --force + --cloud-seed are required.
# SEED_NAME is overridable via `fly deploy --env` or fly.toml [env].
ENTRYPOINT ["/bin/sh", "-c", "exec /usr/local/bin/starmesh hub --cloud-seed --force --name \"$SEED_NAME\""]
