FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/server ./cmd/server \
  && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/seed ./cmd/seed

FROM alpine:3.21

RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/server /app/server
COPY --from=build /out/seed /app/seed
USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
