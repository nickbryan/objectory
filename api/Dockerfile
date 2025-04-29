FROM golang:1.24

WORKDIR /opt/app/api

COPY ../go.mod .
COPY ../go.sum .
RUN go mod download

RUN curl -sSf https://atlasgo.sh | sh
RUN go install github.com/air-verse/air@latest
RUN go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

CMD ["air", "-c", "api/.air.toml"]
