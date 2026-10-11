FROM golang:1.23-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends libsqlite3-dev pkg-config && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY . .
RUN CGO_ENABLED=1 go test ./... && CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/artifactvault ./cmd/artifactvault

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libsqlite3-0 && rm -rf /var/lib/apt/lists/* && useradd --system --uid 10001 --home /nonexistent --shell /usr/sbin/nologin artifactvault
COPY --from=build /out/artifactvault /usr/local/bin/artifactvault
RUN mkdir -p /data && chown 10001:10001 /data
USER 10001:10001
VOLUME ["/data"]
EXPOSE 8080
ENV ARTIFACTVAULT_DB_PATH=/data/artifactvault.db ARTIFACTVAULT_STORAGE_ROOT=/data/storage ARTIFACTVAULT_BIND=0.0.0.0:8080
ENTRYPOINT ["artifactvault"]
CMD ["serve"]
