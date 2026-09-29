FROM golang:1.26.2

WORKDIR /app
COPY . .
RUN go mod download
ENTRYPOINT [ "go", "test", "./..." ]
