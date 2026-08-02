FROM golang:1.24-alpine AS build

WORKDIR /src
RUN apk add --no-cache gcc musl-dev
COPY go.mod ./
RUN go mod download
COPY . ./

# The official TigerBeetle Go client uses its native client library through
# CGO. Alpine's musl toolchain keeps this build reproducible and the runtime
# image already provides the required libc implementation.
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/server ./cmd/server \
  && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/seed ./cmd/seed

FROM alpine:3.21

RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/server /app/server
COPY --from=build /out/seed /app/seed
USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
