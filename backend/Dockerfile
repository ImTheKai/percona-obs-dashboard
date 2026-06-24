FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /obsboard ./cmd/obsboard

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl
ARG TRIVY_VERSION=0.62.1
RUN ARCH=$(uname -m) && \
    case "$ARCH" in \
      x86_64)  TRIVY_ARCH="64bit" ;; \
      aarch64) TRIVY_ARCH="ARM64" ;; \
      *) echo "Unsupported arch: $ARCH" && exit 1 ;; \
    esac && \
    curl -sfL \
      "https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-${TRIVY_ARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin trivy
COPY --from=build /obsboard /obsboard
ENTRYPOINT ["/obsboard"]
