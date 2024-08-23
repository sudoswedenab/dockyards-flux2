FROM docker.io/library/golang:1.23.0 AS builder
COPY . /src
WORKDIR /src
ENV CGO_ENABLED=0
RUN go build -o dockyards-flux2 -ldflags="-s -w"

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /src/dockyards-flux2 /usr/bin/dockyards-flux2
ENTRYPOINT ["/usr/bin/dockyards-flux2"]
