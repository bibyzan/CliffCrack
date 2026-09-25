# The online coordinator (lobby and WebRTC signalling), for fly.io. The game
# itself isn't built here: it needs cgo and the Vulkan renderer.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY coordinator ./coordinator
COPY cmd/coordinator ./cmd/coordinator
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /coordinator ./cmd/coordinator

FROM gcr.io/distroless/static-debian12
COPY --from=build /coordinator /coordinator
EXPOSE 8080
ENTRYPOINT ["/coordinator"]
