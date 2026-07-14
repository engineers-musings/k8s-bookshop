# One image, three binaries. Each Deployment picks its binary with `command:`.
# Multi-stage, so the only thing you need installed is Docker.
FROM golang:1.24-alpine AS build
ARG VERSION=v1
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X github.com/engineers-musings/k8s-bookshop/internal/bookshop.Version=${VERSION}" \
      -o /out/catalog ./cmd/catalog && \
    CGO_ENABLED=0 go build -ldflags "-s -w -X github.com/engineers-musings/k8s-bookshop/internal/bookshop.Version=${VERSION}" \
      -o /out/orders ./cmd/orders && \
    CGO_ENABLED=0 go build -ldflags "-s -w -X github.com/engineers-musings/k8s-bookshop/internal/bookshop.Version=${VERSION}" \
      -o /out/web ./cmd/web

FROM alpine:3.21
# curl and wget are here on purpose: several chapters exec into a running pod to
# prove a Service or a probe works, and a distroless image would leave you with
# no shell to do it from. A production image would drop them.
RUN apk add --no-cache curl
COPY --from=build /out/catalog /out/orders /out/web /usr/local/bin/
RUN adduser -D -u 10001 shop
USER 10001
EXPOSE 8080
CMD ["/usr/local/bin/catalog"]
