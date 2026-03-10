FROM --platform=${BUILDPLATFORM} golang:1.25-alpine AS build

WORKDIR /fn

COPY common common
COPY database database

WORKDIR /fn/database

ENV CGO_ENABLED=0

RUN --mount=type=cache,target=/go/pkg/mod go mod download

ARG TARGETOS
ARG TARGETARCH

RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o /function .

FROM gcr.io/distroless/static-debian12:nonroot AS image
WORKDIR /
COPY --from=build /function /function
EXPOSE 9443
ENTRYPOINT ["/function"]
