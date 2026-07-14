# ---- Build stage ----
FROM golang:1.26-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648 AS builder

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
FROM gcr.io/distroless/static-debian12:nonroot@sha256:aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b

COPY --from=builder /bin/jira-ai-creator /usr/local/bin/jira-ai-creator

ENTRYPOINT ["jira-ai-creator"]
