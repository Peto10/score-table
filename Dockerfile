FROM golang:1.23-alpine@sha256:383395b794dffa5b53012a212365d40c8e37109a626ca30d6151c8348d380b5f AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20@sha256:a4f4213abb84c497377b8544c81b3564f313746700372ec4fe84653e4fb03805

RUN adduser -D -H -u 10001 app && apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /out/server /app/server
COPY web /app/web

USER 10001

ENV ADDR=":8080"
ENV DB_PATH="/data/app.db"

EXPOSE 8080

ENTRYPOINT ["/app/server"]

