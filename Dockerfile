# Multi-stage build: compile the Go server and bundle the React app, then
# serve the static frontend from the same Go process.
FROM node:20-alpine AS web-build
WORKDIR /web
COPY web/package*.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /web/dist /src/web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=go-build /out/server /app/server
COPY --from=web-build /web/dist /app/web/dist
EXPOSE 8080
ENTRYPOINT ["/app/server", "-addr=:8080"]
