FROM node:22-alpine AS frontend-builder
WORKDIR /build/frontend
COPY frontend/package.json frontend/package-lock.json frontend/.npmrc ./
RUN npm ci
COPY frontend/ .
# tsc --noEmit and vite build both run under node; the default V8 heap
# (~4GB) OOMs on this project (3MB+ main chunk pulls in shiki / mermaid /
# cytoscape). Raise the ceiling before running the build.
ENV NODE_OPTIONS="--max-old-space-size=4096"
RUN npm run build

FROM golang:1.24-alpine AS backend-builder
WORKDIR /build
RUN go env -w GOPROXY="https://goproxy.cn,direct"
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /build/frontend/dist ./cmd/server/dist
RUN CGO_ENABLED=0 go build -o server ./cmd/server

FROM alpine:latest
RUN sed -i 's|dl-cdn.alpinelinux.org|mirrors.aliyun.com|g' /etc/apk/repositories && \
    apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend-builder /build/server .
EXPOSE 8081
CMD ["./server"]
