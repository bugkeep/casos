FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY dist/casos /app/casos
COPY web/build /app/web/build

RUN mkdir -p /app/tmp /var/lib/casos

EXPOSE 9000 6443 9443

ENTRYPOINT ["/app/casos"]
