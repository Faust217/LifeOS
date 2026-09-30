FROM golang:1.26.6-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/lifeos-api ./cmd/lifeos-api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/lifeos-migrate ./cmd/lifeos-migrate

FROM busybox:1.37.0-musl AS api

WORKDIR /app
COPY --from=build /out/lifeos-api /usr/local/bin/lifeos-api
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/lifeos-api"]

FROM scratch AS migrate

WORKDIR /app
COPY --from=build /out/lifeos-migrate /usr/local/bin/lifeos-migrate
COPY db/migrations /app/db/migrations
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/lifeos-migrate"]
