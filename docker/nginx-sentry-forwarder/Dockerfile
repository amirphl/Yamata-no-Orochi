FROM python:3.12-alpine

WORKDIR /app

COPY nginx_sentry_forwarder.py /app/nginx_sentry_forwarder.py
RUN chmod 0444 /app/nginx_sentry_forwarder.py

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

CMD ["python", "/app/nginx_sentry_forwarder.py"]
