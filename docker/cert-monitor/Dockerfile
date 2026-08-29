FROM python:3-alpine3.23

WORKDIR /app

COPY requirements.txt /tmp/requirements.txt
RUN apk add --no-cache ca-certificates && \
    pip install --no-cache-dir --requirement /tmp/requirements.txt && \
    rm -f /tmp/requirements.txt

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1
