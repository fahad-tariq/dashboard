FROM golang:1.27.2-alpine@sha256:85dc1069ac644ea3c527b177303a406eb3358192816cd7f9e5848eb658851673 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o /dashboard ./cmd/dashboard

FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /dashboard /usr/local/bin/dashboard
# Unprivileged by default; compose may override with the data owner's UID.
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["dashboard"]
