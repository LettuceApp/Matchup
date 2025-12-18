FROM python:3.10-slim

WORKDIR /app

# Install dependencies
COPY pyproject.toml poetry.lock* /app/
RUN pip install .

# Copy your Dagster code
COPY . /app

# Copy Dagster config
RUN mkdir -p /opt/dagster/storage
COPY dagster.yaml /opt/dagster/dagster.yaml

# Webserver as default CMD
CMD ["dagster-webserver", "-h", "0.0.0.0", "-p", "3000", "-w", "/app/workspace.yaml"]
