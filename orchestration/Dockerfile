FROM python:3.10

WORKDIR /app

# Create dagster home directory
RUN mkdir -p /opt/dagster/storage && mkdir -p /opt/dagster/artifacts

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

# Copy Dagster code
COPY . .

# Expose Dagster UI
EXPOSE 3001

CMD ["dagster-webserver", "-h", "0.0.0.0", "-p", "3001", "--workspace", "/app/workspace.yml"]
