FROM golang:1.26

WORKDIR /opt/app/api

COPY ../go.mod .
COPY ../go.sum .
RUN go mod download

CMD ["go", "tool", "air", "-c", "api/.air.toml"]
