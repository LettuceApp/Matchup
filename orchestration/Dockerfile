FROM python:3.10-slim

WORKDIR /app

COPY . /app

RUN pip install --no-cache-dir .

# Ensure Dagster has a home directory
RUN mkdir -p /opt/dagster
ENV DAGSTER_HOME=/opt/dagster

COPY dagster.yaml /opt/dagster/dagster.yaml
COPY workspace.yaml /app/workspace.yaml

EXPOSE 3000

# The CMD is ignored by Render since the Start Command overrides it.
CMD ["dagster-webserver", "-h", "0.0.0.0", "-p", "3000", "-w", "/app/workspace.yaml"]
