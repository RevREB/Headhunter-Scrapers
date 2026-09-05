# syntax=docker/dockerfile:1
# This module's build definition. It is Go, so it uses the Go toolchain and runs
# go test in the build stage; a module in another language would ship a Dockerfile
# for that toolchain instead. Build context is the repo root so this module's
# local `replace => ../../scraperkit` resolves.
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY . .
WORKDIR /src/modules/builtin
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/scraper .
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/scraper /scraper
USER 65532:65532
ENTRYPOINT ["/scraper"]
