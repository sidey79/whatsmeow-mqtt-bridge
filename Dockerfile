FROM --platform=$BUILDPLATFORM golang:1.27-bookworm@sha256:8d48e12ec56735e9358640898b9d9b9fcca110612ed8a5567438c0a1baa24e66 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/bridge ./cmd/bridge
RUN mkdir -p /out/data && chown 65532:65532 /out/data

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/bridge /bridge
COPY --from=build --chown=nonroot:nonroot /out/data /data
VOLUME ["/data"]
EXPOSE 3000
USER nonroot:nonroot
ENTRYPOINT ["/bridge"]
