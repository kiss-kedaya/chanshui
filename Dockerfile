# syntax=docker/dockerfile:1

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /out/chanshui . \
 && CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /out/feedback-export ./cmd/feedback-export

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
 && addgroup -S chanshui \
 && adduser -S -G chanshui chanshui
WORKDIR /app
COPY --from=server /out/chanshui /app/chanshui
COPY --from=server /out/feedback-export /app/feedback-export
COPY --from=web /src/web/out-build /app/web
RUN mkdir -p /data && chown -R chanshui:chanshui /data
USER chanshui
ENV CHANSHUI_ADMIN_TOKEN=""
EXPOSE 8770
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -qO- http://127.0.0.1:8770/api/health >/dev/null || exit 1
ENTRYPOINT ["/app/chanshui"]
CMD ["-addr","0.0.0.0:8770","-web","/app/web","-db","/data/jobs.db","-workers","16","-queue","128","-upstream-concurrency","64"]
