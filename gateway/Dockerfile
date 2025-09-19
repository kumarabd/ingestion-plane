ARG app_name
ARG app_version
ARG admin_token

FROM golang:1.21 as build-env
WORKDIR /github.com/kumarabd/ingestion-plane/gateway
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/main.go cmd/main.go
COPY internal/ internal/
COPY pkg/ pkg/

RUN GO111MODULE=on go build -ldflags "-w -s -X github.com/kumarabd/ingestion-plane/gateway/internal/config.ApplicationName=$app_name -X github.com/kumarabd/ingestion-plane/gateway/internal/config.ApplicationVersion=$app_version" -a -o service cmd/main.go

FROM gcr.io/distroless/base:nonroot-amd64
ENV DISTRO="debian"
ENV GOARCH="amd64"
WORKDIR /app
COPY --from=build-env /github.com/kumarabd/ingestion-plane/gateway/service .
ENTRYPOINT ["./service"]
