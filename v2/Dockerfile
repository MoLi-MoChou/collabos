FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /collab ./cmd/collab

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /collab /usr/local/bin/collab
EXPOSE 8080
CMD ["/usr/local/bin/collab", "server", "--addr", ":8080"]
