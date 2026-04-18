# Matchup Hub

Matchup Hub is a full-stack voting and bracket platform. Users create matchups — head-to-head comparisons between two or more items — and other users vote on them. Matchups can be standalone or grouped into elimination brackets that advance round by round automatically or on a timer. The app includes social features such as likes, comments, follows, and user profiles.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend API | Go 1.25, ConnectRPC (protobuf over HTTP/2) |
| Frontend | React (Create React App), Framer Motion, Radix UI |
| Database | PostgreSQL 14 |
| Cache / pub-sub | Redis 7 |
| Migrations | Goose |
| Orchestration | In-process Go scheduler (`api/scheduler`, `api/cmd/cron`) |
| Container runtime | Docker / Docker Compose |
| Production deployment | Kubernetes on Azure AKS, Azure Container Registry |

## Prerequisites

- **Go** 1.25 or newer
- **Node** 18 or newer
- **Docker** and **Docker Compose**
- **buf** CLI (only needed if you modify `.proto` files — `brew install bufbuild/buf/buf`)

## Environment configuration

Copy the example env file and fill in your values:

```bash
cp .env.example .env
```

Required variables include `DB_USER`, `DB_PASSWORD`, `DB_HOST`, `DB_PORT`, `DB_NAME`, `REDIS_ADDR`, and any mail / storage keys referenced in `docker-compose.yml`.

## Running locally with Docker Compose

The easiest way to run the full stack (Postgres, Redis, API, frontend, and the cron scheduler) in one command:

```bash
docker-compose up --build
```

| Service | URL |
|---------|-----|
| Frontend | http://localhost:3000 |
| API | http://localhost:8888 |
| Postgres | localhost:5432 |
| Redis | localhost:6379 |

## Running services individually

**Backend API**

```bash
cd api
go run ./cmd/main.go
```

Database migrations run automatically on startup via Goose. To run them manually:

```bash
cd api
go run ./migrations/migrate.go
```

**Frontend**

```bash
cd frontend
npm install
npm start
```

**Regenerating protobuf code** (only needed after editing `.proto` files)

```bash
cd api
buf generate
```

## Project structure

```
.
├── api/                  # Go backend
│   ├── cmd/              # Entry points: api server, worker, cron
│   ├── controllers/      # ConnectRPC handler implementations
│   ├── middlewares/      # Auth, rate limiting
│   ├── migrations/       # Goose SQL migrations
│   ├── models/           # DB model structs and queries
│   ├── proto/            # Protobuf service definitions
│   ├── scheduler/        # Cron-style background workload scheduler
│   └── gen/              # Generated protobuf Go code (do not edit)
├── frontend/             # React frontend
│   ├── src/
│   │   ├── components/   # Shared UI components
│   │   ├── hooks/        # Custom React hooks
│   │   ├── pages/        # Page-level components
│   │   ├── services/     # API client functions
│   │   └── styles/       # CSS files
│   └── Dockerfile
├── k8s/                  # Kubernetes manifests (AKS)
└── docker-compose.yml
```

## Testing

**Backend** (runs from the `api/` directory):

```bash
cd api
go test ./controllers/... ./middlewares/...
```

**Frontend** (runs from the `frontend/` directory):

```bash
cd frontend
npm test -- --watchAll=false
```

## Deployment (AKS)

Images are built and pushed to Azure Container Registry, then applied to the cluster:

```bash
az acr login --name matchupacr

docker build -t matchupacr.azurecr.io/matchup-api:v1 ./api
docker push matchupacr.azurecr.io/matchup-api:v1

docker build -t matchupacr.azurecr.io/matchup-frontend:v1 -f frontend/Dockerfile .
docker push matchupacr.azurecr.io/matchup-frontend:v1

kubectl apply -f k8s/
kubectl rollout restart deployment/matchup-api -n matchup
kubectl rollout restart deployment/matchup-frontend -n matchup
```
