FROM python:3.10-slim

WORKDIR /app

COPY pyproject.toml ./
RUN pip install --no-cache-dir dagster dagster-webserver dagster-postgres python-dotenv pandas sqlalchemy

COPY . .

RUN mkdir -p /opt/dagster
COPY dagster.yaml /opt/dagster/dagster.yaml
COPY workspace.yaml /app/workspace.yaml

ENV DAGSTER_HOME=/opt/dagster
