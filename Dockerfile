FROM golang:1.27-trixie AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/api ./cmd/api
COPY internal ./internal
COPY pkg ./pkg
RUN go build -trimpath -o /out/api ./cmd/api

FROM alpine:3.24
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/api ./api
COPY configs/config.yaml ./configs/config.yaml
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/api"]
CMD ["-config", "configs/config.yaml"]
