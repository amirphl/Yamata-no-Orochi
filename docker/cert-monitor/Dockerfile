FROM python:3-alpine3.23

WORKDIR /app

COPY cert_monitor_requirements.txt /tmp/requirements.txt
RUN apk add --no-cache ca-certificates && \
    pip install --no-cache-dir --requirement /tmp/requirements.txt && \
    rm -f /tmp/requirements.txt

COPY cert_monitor.py /app/cert_monitor.py
RUN chmod 0444 /app/cert_monitor.py

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

CMD ["python", "/app/cert_monitor.py"]
