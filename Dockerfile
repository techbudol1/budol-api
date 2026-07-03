FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/budol-api ./cmd/api

FROM oven/bun:1.3.13

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile --production

COPY scripts/private-claim-note.ts ./scripts/private-claim-note.ts
COPY --from=build /out/budol-api /usr/local/bin/budol-api
COPY public/zk ./public/zk
RUN command -v bun \
    && command -v wget \
    && test -f /app/scripts/private-claim-note.ts \
    && test -s /app/public/zk/private-claim/private_winning_claim.wasm \
    && test -s /app/public/zk/private-claim/private_winning_claim_final.zkey \
    && test -s /app/public/zk/private-claim/verification_key.json

EXPOSE 8082
CMD ["budol-api"]
