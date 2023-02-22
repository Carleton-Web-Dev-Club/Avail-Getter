FROM golang:latest as builder
WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY cmd cmd
RUN CGO_ENABLED=0 go build -o avail ./...

FROM gcr.io/distroless/static-debian11
WORKDIR /
COPY --from=builder /app/avail avail
EXPOSE 8080
ENV GIN_MODE=release
ENTRYPOINT ["/avail"]
LABEL org.opencontainers.image.source https://github.com/Carleton-Web-Dev-Club/Avail-Getter
