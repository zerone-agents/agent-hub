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

# h5 移动端 SPA：仅产出静态文件（vite build 时 base 默认 /static/h5/，
# 见 h5/vite.config.ts）。开发期 express 服务器（server.ts + esbuild 产物
# server.cjs）不进镜像。
FROM node:22-alpine AS h5-builder
WORKDIR /build/h5
COPY h5/package.json h5/package-lock.json ./
RUN npm ci
COPY h5/ .
RUN npx vite build

FROM golang:1.25-alpine AS backend-builder
WORKDIR /build
RUN go env -w GOPROXY="https://goproxy.cn,direct"
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /build/frontend/dist ./cmd/server/dist
# h5 产物作为 dist 的 h5 子目录一并 embed，经 /static/h5/ 对外服务
COPY --from=h5-builder /build/h5/dist ./cmd/server/dist/h5
RUN CGO_ENABLED=0 go build -o server ./cmd/server

FROM alpine:latest
RUN sed -i 's|dl-cdn.alpinelinux.org|mirrors.aliyun.com|g' /etc/apk/repositories && \
    apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend-builder /build/server .
EXPOSE 8081
CMD ["./server"]
