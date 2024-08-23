# Build the manager binary
FROM ghcr.io/cybozu/golang:1.22-jammy AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/egress-controller/main.go cmd/egress-controller/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o egress-controller cmd/egress-controller/main.go

FROM ghcr.io/cybozu/ubuntu:22.04
WORKDIR /
COPY --from=builder /workspace/egress-controller .
USER 65532:65532

ENTRYPOINT ["/egress-controller"]
