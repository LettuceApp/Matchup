FROM python:3.10-slim

# Install system dependencies (optional: postgres client, etc.)
RUN apt-get update && apt-get install -y --no-install-recommends build-essential gcc && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy project files (including pyproject.toml)
COPY . .

# Install the orchestration package and dependencies
RUN pip install --no-cache-dir .

# Create Dagster home and storage directories
RUN mkdir -p /opt/dagster && mkdir -p /opt/dagster/storage

# Copy dagster instance configuration
COPY dagster.yaml /opt/dagster/dagster.yaml
COPY workspace.yaml /app/workspace.yaml

# Tell Dagster where to find dagster.yaml
ENV DAGSTER_HOME=/opt/dagster

# Expose Render port
ENV PORT=8000

# Start Dagster Webserver
CMD ["dagster-webserver", "-h", "0.0.0.0", "-p", "${PORT}", "-w", "workspace.yaml"]
