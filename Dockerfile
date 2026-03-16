FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /tracker ./cmd/tracker

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /tracker /usr/local/bin/tracker
EXPOSE 8080
ENTRYPOINT ["tracker"]
