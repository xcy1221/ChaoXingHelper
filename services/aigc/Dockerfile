FROM golang:1.27.2-bookworm AS build
WORKDIR /src
ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY web ./web
COPY platform ./platform
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /aigc .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /aigc /aigc
USER 65532:65532
ENV AIGC_LISTEN=0.0.0.0:8080 AIGC_DATA_DIR=/data
EXPOSE 8080
ENTRYPOINT ["/aigc"]
