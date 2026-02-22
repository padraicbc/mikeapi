FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY api /app/mikeapi

EXPOSE 9000
ENTRYPOINT ["/app/mikeapi"]
