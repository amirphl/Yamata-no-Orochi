FROM python:3-alpine3.23

RUN apk add --no-cache ca-certificates && \
    pip install --no-cache-dir requests cryptography

WORKDIR /app
