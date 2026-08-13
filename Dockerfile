# ---- Build stage ----
FROM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/jira-ai-creator .

# ---- Runtime stage ----
# Distroless static image: no shell, no package manager, no libc — minimal attack surface.
# Includes ca-certificates and tzdata. Runs as nonroot (uid 65534) by default.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:1b7b9f0f0e0a1d2155f531db587cc48ec26aaf97ab64364225f5bf18a054e66a

COPY --from=builder /bin/jira-ai-creator /usr/local/bin/jira-ai-creator

ENTRYPOINT ["jira-ai-creator"]
