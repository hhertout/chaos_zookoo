FROM golang:1.26.2-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o bin/chaos_zookoo ./cmd/chaos_zookoo

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/bin/chaos_zookoo /chaos_zookoo

ENTRYPOINT ["/chaos_zookoo"]
