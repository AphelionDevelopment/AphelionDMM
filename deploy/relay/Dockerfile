FROM golang:1.25.13-bookworm@sha256:e401dae1bf814e29204a8cb7915682e1780951e609ca0dd8865ee1937f510c48 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/apheliondmm-relay ./cmd/apheliondmm-relay
COPY cmd/apheliondmm-healthcheck ./cmd/apheliondmm-healthcheck
COPY internal ./internal
ARG APHELIONDMM_BUILD=container
ARG APHELIONDMM_REVISION=unknown
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.buildRevision=${APHELIONDMM_REVISION}" -o /out/apheliondmm-relay ./cmd/apheliondmm-relay
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/apheliondmm-healthcheck ./cmd/apheliondmm-healthcheck

FROM gcr.io/distroless/static-debian13:nonroot@sha256:1c2c046bc09ed40fad370b599a0b1ae7987f55b01e247cf27a7c27cd97e5bbc7

COPY --from=build --chown=nonroot:nonroot /out/apheliondmm-relay /apheliondmm-relay
COPY --from=build --chown=nonroot:nonroot /out/apheliondmm-healthcheck /apheliondmm-healthcheck
USER nonroot:nonroot
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD ["/apheliondmm-healthcheck"]
ENTRYPOINT ["/apheliondmm-relay"]
CMD ["-config", "/etc/apheliondmm/relay.yaml"]

