FROM python:3.10-slim

RUN apt-get update && apt-get install -y build-essential gcc && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY . .

RUN pip install --no-cache-dir .

RUN mkdir -p /opt/dagster/storage

COPY dagster.yaml /opt/dagster/dagster.yaml
COPY workspace.yaml /app/workspace.yaml

ENV DAGSTER_HOME=/opt/dagster

# Render sets PORT at runtime
CMD dagster-webserver -h 0.0.0.0 -p $PORT -w workspace.yaml
