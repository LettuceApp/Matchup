FROM python:3.10-slim

WORKDIR /app

# Install Python dependencies
COPY requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r requirements.txt

# Copy project source
COPY . .

# Dagster home
ENV DAGSTER_HOME=/opt/dagster
RUN mkdir -p /opt/dagster

# Ensure Python can import your modules
ENV PYTHONPATH="/app"

# Config files
COPY dagster.yaml /opt/dagster/dagster.yaml
COPY workspace.yaml /app/workspace.yaml

EXPOSE 3000

# *** DO NOT SET CMD HERE *** 
# Render will inject a Start Command for each service.
