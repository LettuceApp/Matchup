# ================================
# Base Image
# ================================
FROM python:3.10-slim

# Install supervisord
RUN apt-get update && apt-get install -y supervisor && \
    mkdir -p /var/log/supervisor

# Set working directory
WORKDIR /app

# Copy all project files
COPY . /app

# Install your Python package
RUN pip install --no-cache-dir .

# Create Dagster storage directory
RUN mkdir -p /opt/dagster/storage

# Copy Dagster config
COPY dagster.yaml /opt/dagster/dagster.yaml

# Copy workspace
COPY workspace.yaml /app/workspace.yaml

# ================================
# Supervisor Configuration
# ================================
RUN mkdir -p /etc/supervisor/conf.d

COPY <<EOF /etc/supervisor/conf.d/dagster.conf
[supervisord]
nodaemon=true

[program:webserver]
command=dagster-webserver -h 0.0.0.0 -p 3000 -w /app/workspace.yaml
environment=DAGSTER_HOME="/opt/dagster"
autostart=true
autorestart=true
priority=10

[program:daemon]
command=dagster-daemon run
environment=DAGSTER_HOME="/opt/dagster"
autostart=true
autorestart=true
priority=20
EOF

# ================================
# Start supervisord (runs BOTH Dagster processes)
# ================================
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/dagster.conf"]
