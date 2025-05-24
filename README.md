# Matchup

Matchup is a full stack web project consisting of a Go backend API and a React frontend. The backend exposes REST endpoints while the frontend provides the user interface. It is a polling app with some social media features. Users can creae matchups and items in the matchups to vote on. Users can create accounts, vote on matchups, like matchups and more.

## Prerequisites

- **Go** 1.22 or newer
- **Node** 18 or newer
- **Docker** (for running containers)

## Environment configuration

The backend requires environment variables. An example file is provided at the repository root.

```bash
cp .env.example .env
```

Edit the newly created `.env` file and provide values for variables such as `DB_USER`, `DB_PASSWORD`, `DB_HOST`, `DB_PORT`, `DB_NAME` and any others referenced in `docker-compose.yml`.

## Running the backend

You can start the API directly with Go:

```bash
go run ./cmd/main.go
```

Or run everything using Docker Compose:

```bash
docker-compose up
```

## Running the frontend

Install dependencies and start the React development server:

```bash
cd frontend
npm install
npm start
```

## Testing

Run backend tests from the project root:

```bash
go test ./...
```

Run frontend tests from the `frontend` directory:

```bash
npm test
```
