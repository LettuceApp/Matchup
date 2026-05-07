.PHONY: test test-api test-frontend test-integration cover cover-html lint build dev setup-hooks

# ── Tests ──────────────────────────────────────────────────────────────
test: test-api test-frontend

test-api:
	cd api && go test -race -count=1 ./...

test-frontend:
	cd frontend && npm test -- --watchAll=false --passWithNoTests

test-integration:
	cd api && go test -tags integration -race -count=1 -v ./controllers/

# ── Coverage ───────────────────────────────────────────────────────────
cover:
	cd api && go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

cover-html:
	cd api && go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

# ── Lint ───────────────────────────────────────────────────────────────
lint: lint-api lint-frontend

lint-api:
	cd api && go vet ./...
	@which golangci-lint > /dev/null 2>&1 && cd api && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

lint-frontend:
	cd frontend && npx eslint src/ --max-warnings 0

# ── Build ──────────────────────────────────────────────────────────────
build:
	cd api && go build ./...

# ── Dev ────────────────────────────────────────────────────────────────
dev:
	docker compose up -d postgres redis
	@echo "Waiting for Postgres..."
	@sleep 2
	cd api && go run ./cmd

# ── Hooks ──────────────────────────────────────────────────────────────
# Point this clone at the tracked .githooks/ directory so .githooks/pre-push
# fires on every `git push`. Run once per clone; the config is per-clone
# (not in the repo), so cloning fresh requires re-running this.
setup-hooks:
	git config core.hooksPath .githooks
	@echo "✓ hooks enabled — .githooks/pre-push will run on git push"
	@echo "  Skip in emergencies with: git push --no-verify"
