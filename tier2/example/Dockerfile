# syntax=docker/dockerfile:1
# Build context is the repo root (so the module go.mod is available).
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/scraper ./tier2/example

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/scraper /scraper
USER 65532:65532
ENTRYPOINT ["/scraper"]
