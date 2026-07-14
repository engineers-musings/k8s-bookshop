# The official registry.k8s.io/kubectl image is DISTROLESS — it has no shell, so a
# shell-loop controller cannot run in it. This adds kubectl to a base that has one.
FROM alpine:3.21
ARG KUBECTL_VERSION=v1.36.1
RUN apk add --no-cache curl && \
    curl -sSLo /usr/local/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" && \
    chmod +x /usr/local/bin/kubectl
ENTRYPOINT ["/bin/sh"]
