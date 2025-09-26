FROM --platform=${BUILDPLATFORM} golang:1.24-alpine AS build

WORKDIR /fn/postgresql

ENV CGO_ENABLED=0

RUN --mount=target=. --mount=type=cache,target=/go/pkg/mod go mod download

ARG TARGETOS
ARG TARGETARCH

RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /function .

FROM gcr.io/distroless/static-debian12:nonroot AS image
WORKDIR /
COPY --from=build /function /function
EXPOSE 9443
ENTRYPOINT ["/function"]
