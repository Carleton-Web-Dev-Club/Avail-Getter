FROM golang:latest as builder
WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY cmd cmd
RUN go build -o avail ./...

FROM alpine
WORKDIR /app
RUN apk add gcompat
COPY --from=builder /app/avail avail
EXPOSE 8080
CMD ["avail"]