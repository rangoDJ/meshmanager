# syntax=docker/dockerfile:1

# --- frontend: build the React app ------------------------------------------
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# --- backend: compile a static Go binary with the frontend embedded ----------
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS backend
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /src/frontend/dist ./internal/web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X github.com/rangoDJ/meshmanager/internal/version.Version=${VERSION}" \
    -o /out/meshmanager ./cmd/meshmanager \
 && mkdir -p /out/data

# --- runtime -----------------------------------------------------------------
# Starts as root only to give PUID:PGID ownership of /data, then switches to that user
# for good before doing any work (see cmd/meshmanager/privdrop_unix.go).
FROM gcr.io/distroless/static-debian12
COPY --from=backend /out/meshmanager /usr/local/bin/meshmanager
COPY --from=backend /out/data /data
ENV LISTEN_ADDR=:8080 \
    DATA_DIR=/data \
    PUID=1000 \
    PGID=1000
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD ["meshmanager", "healthcheck"]
ENTRYPOINT ["meshmanager"]
CMD ["serve"]
