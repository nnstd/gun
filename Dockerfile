FROM golang:1.26-bookworm AS builder

ARG BASELINE=0

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN if [ "$BASELINE" = "1" ]; then \
      go build -tags baseline -ldflags "-X main.gunModuleRoot=/opt/gun" -o /gun .; \
    else \
      go build -ldflags "-X main.gunModuleRoot=/opt/gun" -o /gun .; \
    fi

FROM scratch
COPY --from=builder /gun /usr/local/bin/gun
ENTRYPOINT ["gun"]
