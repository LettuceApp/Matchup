FROM python:3.12-slim

# Start at repo root
WORKDIR /app

# System deps for psycopg2, etc.
RUN apt-get update && apt-get install -y \
    build-essential \
    libpq-dev \
  && rm -rf /var/lib/apt/lists/*

# Install Dagster + deps
COPY orchestration/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir -r requirements.txt

# Copy the whole repo into the image
COPY . .

# Dagster instance home
ENV DAGSTER_HOME=/opt/dagster
RUN mkdir -p $DAGSTER_HOME && touch $DAGSTER_HOME/dagster.yaml

# 🔹 Go into the *project root* where pyproject.toml lives
#    (Matchup/orchestration/matchup_orchestration)
WORKDIR /app/orchestration/matchup_orchestration

# 🔹 Install this project so `matchup_orchestration` is a real package
RUN pip install -e .

EXPOSE 3001

# 🔹 Run Dagster dev from the project folder
CMD ["dagster", "dev", "-h", "0.0.0.0", "-p", "3001"]
