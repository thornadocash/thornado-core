FROM rust:1-bookworm AS builder

WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends pkg-config libssl-dev \
    && rm -rf /var/lib/apt/lists/*

COPY Cargo.toml Cargo.lock ./
COPY crates ./crates
COPY circuits ./circuits
COPY scripts ./scripts

RUN cargo build --release -p thornado-node

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/target/release/thornado-node /usr/local/bin/thornado-node
COPY --from=builder /app/scripts/live_regtest_smoke.sh /usr/local/bin/live_regtest_smoke.sh

EXPOSE 3030
ENTRYPOINT ["thornado-node"]
