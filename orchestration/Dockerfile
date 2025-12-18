# ================================
# Base Image
# ================================
FROM python:3.10-slim

# Install supervisord
RUN apt-get update && apt-get install -y supervisor && \
    mkdir -p /var/log/supervisor

# Working directory for your app
WORKDIR /app

# Copy project files FIRST
COPY . /app

# Install Python dependencies
RUN pip install --no-cache-dir .

# Dagster storage directory
RUN mkdir -p /opt/dagster/storage
COPY dagster.yaml /opt/dagster/dagster.yaml

# Copy workspace (must be in same folder as code)
COPY workspace.yaml /app/workspace.yaml

# ================================
# Supervisor Configuration
# ================================
RUN mkdir -p /etc/supervisor/conf.d

COPY <<EOF /etc/supervisor/conf.d/dagster.conf
[supervisord]
nodaemon=true

[program:webserver]
command=dagster-webserver -h 0.0.0.0 -p 3001 -w /app/workspace.yaml
autostart=true
autorestart=true
priority=10

[program:daemon]
command=dagster-daemon run
autostart=true
autorestart=true
priority=20
EOF

# ================================
# Start Supervisord (runs both)
# ================================
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/dagster.conf"]
