FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /obsboard ./cmd/obsboard

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl
ARG TRIVY_VERSION=0.62.1
RUN curl -sfL \
    https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-64bit.tar.gz \
    | tar -xz -C /usr/local/bin trivy
COPY --from=build /obsboard /obsboard
ENTRYPOINT ["/obsboard"]
