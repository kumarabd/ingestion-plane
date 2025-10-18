ARG app_name
ARG app_version
ARG admin_token
ARG GO_VERSION=1.25

FROM golang:${GO_VERSION} as build-env
WORKDIR /github.com/kumarabd/ingestion-plane/gateway
COPY contracts/ contracts/
COPY gateway/go.mod gateway/go.sum ./
RUN cd gateway && go mod download

COPY gateway/cmd/main.go cmd/main.go
COPY gateway/internal/ internal/
COPY gateway/pkg/ pkg/

RUN GO111MODULE=on go build -ldflags "-w -s -X github.com/kumarabd/ingestion-plane/gateway/internal/config.ApplicationName=$app_name -X github.com/kumarabd/ingestion-plane/gateway/internal/config.ApplicationVersion=$app_version" -a -o service cmd/main.go

FROM gcr.io/distroless/base:nonroot-amd64
ENV DISTRO="debian"
ENV GOARCH="amd64"
WORKDIR /app
COPY --from=build-env /github.com/kumarabd/ingestion-plane/gateway/service .
ENTRYPOINT ["./service"]
