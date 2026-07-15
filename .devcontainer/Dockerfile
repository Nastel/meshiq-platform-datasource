# Contributor dev image for the meshIQ Platform datasource.
#
# Bakes the Go build tool (mage) and debugger (delve) into the image so they're present
# immediately — cached in the layer, offline-friendly, and no per-create `go install`.
#
# They're installed into /usr/local/bin (on everyone's PATH) using a THROWAWAY module cache
# (GOPATH=/tmp/gotools, removed after), so the shared /go module cache stays clean and
# vscode-owned for normal builds. The image ends as root so devcontainer features
# (node, docker-in-docker) install normally.
FROM mcr.microsoft.com/devcontainers/go:1.26-trixie

# Pinned for reproducibility. delve must support the Go toolchain (1.26) — bump these together
# with the `go` directive in go.mod / the base image tag.
RUN GOBIN=/usr/local/bin GOPATH=/tmp/gotools go install github.com/magefile/mage@v1.17.2 \
 && GOBIN=/usr/local/bin GOPATH=/tmp/gotools go install github.com/go-delve/delve/cmd/dlv@v1.27.0 \
 && rm -rf /tmp/gotools
