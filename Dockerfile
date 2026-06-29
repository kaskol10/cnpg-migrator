# syntax=docker/dockerfile:1
# ARM64 runtime image. Frontend builds on the CI host arch; Go cross-compiles to arm64.

FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS backend
WORKDIR /app
RUN apk add --no-cache git ca-certificates file
COPY backend/go.mod backend/go.sum* ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /app/backend/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /server ./cmd/server
RUN file /server | grep -q 'ELF.*aarch64'

FROM --platform=$TARGETPLATFORM alpine:3.20
RUN apk add --no-cache ca-certificates file
WORKDIR /app
COPY --from=backend /server ./server
COPY --from=frontend /app/backend/web/dist ./web/dist
RUN chmod +x ./server && file ./server | grep -q 'ELF.*aarch64'
EXPOSE 8080
ENTRYPOINT ["./server"]
