FROM python:3.10-slim

WORKDIR /app

COPY requirements.txt /app/requirements.txt
RUN pip install -r requirements.txt

COPY . .

ENV DAGSTER_HOME=/opt/dagster
RUN mkdir -p /opt/dagster

ENV PYTHONPATH="/app"

COPY dagster.yaml /opt/dagster/dagster.yaml
COPY workspace.yaml /app/workspace.yaml

EXPOSE 3000

CMD ["dagster-webserver", "-h", "0.0.0.0", "-p", "3000", "-w", "/app/workspace.yaml"]
