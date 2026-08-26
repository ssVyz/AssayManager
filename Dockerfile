# syntax=docker/dockerfile:1

# AssayManager — minimal two-stage image.
#
#   builder : compiles the Go server into a static binary and writes /app/.env
#             from the required NCBI_EMAIL build arg.
#   runtime : distroless/static (no shell, ~2 MB) holding the server, the
#             prebuilt static inclusivity_check_blast binary, and .env.
#
# The inclusivity binary in assets/ is a static linux/amd64 ELF, so the image is
# pinned to linux/amd64 and needs no shared libraries at runtime — distroless
# supplies the CA certificates the tool needs for HTTPS to NCBI.
#
# The SQLite database and log are written to the working directory (/app), just
# like running the program locally. They live in the container's writable layer,
# so they PERSIST across `docker stop` / `docker start` of the same container,
# but are lost if the container is removed/recreated. Note that `docker run`
# always makes a NEW container — use `docker start <name>` to resume the old one.
# To copy the DB out of a running/stopped container:
#   docker cp assaymanager:/app/assaymanager.db ./assaymanager.db
#
# Build (NCBI contact email is REQUIRED):
#   docker build --build-arg NCBI_EMAIL=you@org.example -t assaymanager .
#
# Run once, then stop/resume the same container (DB intact):
#   docker run -d -p 8080:8080 --name assaymanager assaymanager
#   docker stop assaymanager
#   docker start assaymanager

# ---------------------------------------------------------------------------
# Stage 1 — build the server, generate .env
# ---------------------------------------------------------------------------
FROM --platform=linux/amd64 golang:1.26-alpine AS builder

WORKDIR /src

# Module cache layer: only invalidated when go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Static build (modernc SQLite is pure Go, so no cgo is required).
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/assaymanager .

# Generate .env from the required build arg; fail the build if it is missing.
ARG NCBI_EMAIL
RUN test -n "$NCBI_EMAIL" || { \
        echo "ERROR: build requires --build-arg NCBI_EMAIL=<contact-email>" >&2; \
        exit 1; }; \
    printf 'AM_NCBI_EMAIL=%s\n' "$NCBI_EMAIL" > /out/.env

# ---------------------------------------------------------------------------
# Stage 2 — minimal runtime (runs as root so the working dir is writable)
# ---------------------------------------------------------------------------
FROM --platform=linux/amd64 gcr.io/distroless/static-debian12

WORKDIR /app

# Server binary + generated .env (read from the working dir at startup).
COPY --from=builder /out/assaymanager /app/assaymanager
COPY --from=builder /out/.env         /app/.env

# Prebuilt static analysis binary from the build context (renamed, +x).
COPY --chmod=0755 assets/inclusivity_check_blast_lin /app/inclusivity_check_blast

# Only the analysis binary needs pointing; AM_DB / AM_LOG keep their defaults, so
# the DB and log land in the working dir as assaymanager.db / assaymanager.log.
ENV AM_INCLUSIVITY_BIN=/app/inclusivity_check_blast \
    AM_ADDR=:8080

EXPOSE 8080
ENTRYPOINT ["/app/assaymanager"]
