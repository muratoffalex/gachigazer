FROM python:3-alpine AS builder

COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install -r requirements.txt && \
    apk add --no-cache sqlite tzdata libc6-compat ffmpeg

RUN mkdir -p /app/data
WORKDIR /app
COPY bin/gachigazer /app/gachigazer
RUN chmod +x /app/gachigazer

CMD ["/app/gachigazer"]
