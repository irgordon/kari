ARG GO_BUILDER_IMAGE
FROM ${GO_BUILDER_IMAGE} AS builder

ENV GOTOOLCHAIN=local
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY api ./api
COPY proto ./proto

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/kari-api ./api/cmd/kari-api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/kari-migrate ./api/cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./api/cmd/healthcheck

FROM scratch

WORKDIR /app
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/kari-api /app/kari-api
COPY --from=builder /out/kari-migrate /app/kari-migrate
COPY --from=builder /out/healthcheck /app/healthcheck

USER 1001:1001
EXPOSE 8080
ENTRYPOINT ["/app/kari-api"]
