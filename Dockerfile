FROM golang:1.26.6-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/midnight-council ./cmd/server

FROM alpine:3.23

RUN addgroup -S midnight && adduser -S -G midnight midnight

COPY --from=build /out/midnight-council /usr/local/bin/midnight-council

USER midnight:midnight
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/midnight-council"]
