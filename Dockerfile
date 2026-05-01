FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o world-fog ./cmd/server

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/world-fog /world-fog
COPY --from=builder /app/configs /configs

EXPOSE 8443

ENTRYPOINT ["/world-fog", "--config", "/configs/config.yaml"]
