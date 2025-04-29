FROM golang:1.24

WORKDIR /opt/app/web

COPY ../go.mod .
COPY ../go.sum .
RUN go mod download

RUN go install github.com/air-verse/air@latest

CMD ["air", "-c", "web/.air.toml"]
