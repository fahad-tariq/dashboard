FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /dashboard ./cmd/dashboard

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata git
COPY --from=build /dashboard /usr/local/bin/dashboard
EXPOSE 8080
ENTRYPOINT ["dashboard"]
