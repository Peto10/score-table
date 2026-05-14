FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20

RUN adduser -D -H -u 10001 app && apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /out/server /app/server
COPY web /app/web

USER 10001

ENV ADDR=":8080"
ENV DB_PATH="/data/app.db"
ENV CONFIG_PATH="/config/teams.yaml"

EXPOSE 8080

ENTRYPOINT ["/app/server"]

