FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /bin/builder ./cmd/builder
RUN go build -o /bin/api ./cmd/api
RUN go build -o /bin/lb ./cmd/lb
ADD https://github.com/zanfranceschi/rinha-de-backend-2026/raw/main/resources/references.json.gz /refs.json.gz
RUN mkdir -p /data && /bin/builder --input /refs.json.gz --output /data/references.idx --centroids 512 --iterations 5

FROM alpine:3.19 AS api
COPY --from=builder /bin/api /api
COPY --from=builder /data/references.idx /data/references.idx
CMD ["/api"]

FROM scratch AS lb
COPY --from=builder /bin/lb /lb
CMD ["/lb"]
