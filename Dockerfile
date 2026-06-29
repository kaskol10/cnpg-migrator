# syntax=docker/dockerfile:1
# Multi-arch image (linux/amd64 + linux/arm64).
# Frontend builds on the CI host arch; Go cross-compiles to the target arch.

FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS backend
ARG TARGETARCH
WORKDIR /app
RUN apk add --no-cache git ca-certificates file
COPY backend/go.mod backend/go.sum* ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /app/backend/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -o /server ./cmd/server
RUN case "${TARGETARCH}" in \
      arm64) file /server | grep -q 'ELF.*aarch64' ;; \
      amd64) file /server | grep -q 'ELF.*x86-64' ;; \
      *) echo "unsupported arch: ${TARGETARCH}" && exit 1 ;; \
    esac

FROM --platform=$TARGETPLATFORM alpine:3.20
ARG TARGETARCH
RUN apk add --no-cache ca-certificates file
WORKDIR /app
COPY --from=backend /server ./server
COPY --from=frontend /app/backend/web/dist ./web/dist
RUN chmod +x ./server && case "${TARGETARCH}" in \
      arm64) file ./server | grep -q 'ELF.*aarch64' ;; \
      amd64) file ./server | grep -q 'ELF.*x86-64' ;; \
      *) echo "unsupported arch: ${TARGETARCH}" && exit 1 ;; \
    esac
EXPOSE 8080
ENTRYPOINT ["./server"]
