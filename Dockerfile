# Stage 1: build frontend static assets
FROM node:20-alpine AS frontend-build
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Stage 2: build backend binary
FROM golang:1.25-alpine AS backend-build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=0 go build -o /obsboard ./cmd/obsboard

# Stage 3: minimal runtime image
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
COPY --from=backend-build /obsboard /obsboard
COPY --from=frontend-build /app/dist /frontend
ENV FRONTEND_DIR=/frontend
EXPOSE 4000
ENTRYPOINT ["/obsboard"]
