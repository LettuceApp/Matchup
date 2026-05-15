package controllers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"Matchup/cache"
	"Matchup/controllers/share"
	appdb "Matchup/db"
	"Matchup/middlewares"
	"Matchup/security"

	aws2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	// DB is the primary read/write connection pool. All writes, all
	// `SELECT FOR UPDATE`, and any read that lives inside a write
	// transaction must go through DB.
	DB *sqlx.DB
	// ReadDB is a connection pool aimed at the read replica
	// (k8s/postgres/replica-deployment.yaml). For dev/test, where no
	// replica exists, ReadDB is set to the same handle as DB so call
	// sites don't need to know whether a replica is configured.
	ReadDB *sqlx.DB
	// S3Client is a process-wide singleton initialized in Initialize().
	// Constructing s3.NewFromConfig is expensive (it walks the AWS
	// credential chain and resolves the endpoint), so doing it once at
	// startup keeps avatar/image upload latency predictable. Handlers
	// that need it (UserHandler, MatchupHandler) get it injected via
	// initializeConnectRoutes — they do not call NewFromConfig themselves.
	// Nil if AWS config could not be loaded; handlers must guard.
	S3Client *s3.Client
	Router   chi.Router
}

// initS3Client builds the singleton s3.Client at startup. Returns nil if
// the AWS config can't be loaded — uploads will then fail fast at the
// handler level instead of crashing the entire server boot. Pulled out
// of Initialize() so it stays unit-testable in isolation.
func initS3Client() *s3.Client {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-2"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		cfg aws2.Config
		err error
	)
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")
	if accessKey != "" && secretKey != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, sessionToken)),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region))
	}
	if err != nil {
		log.Printf("warning: AWS config load failed; uploads will be unavailable: %v", err)
		return nil
	}
	return s3.NewFromConfig(cfg)
}

// ===============================
// SECURE ADMIN SEEDING
// ===============================
func seedAdmin(db *sqlx.DB) error {
	adminEmail := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	adminPassword := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))

	if adminEmail == "" || adminPassword == "" {
		log.Println("[seedAdmin] ADMIN_EMAIL or ADMIN_PASSWORD not set — skipping admin creation.")
		return nil
	}

	ctx := context.Background()
	var existingID uint
	var isAdmin bool
	err := db.QueryRowContext(ctx,
		"SELECT id, is_admin FROM users WHERE email = $1", adminEmail,
	).Scan(&existingID, &isAdmin)

	if err == sql.ErrNoRows {
		log.Println("[seedAdmin] Creating initial admin:", adminEmail)

		hashedPw, err := security.Hash(adminPassword)
		if err != nil {
			return err
		}
		publicID := appdb.GeneratePublicID()
		username := strings.Split(adminEmail, "@")[0]

		_, err = db.ExecContext(ctx,
			`INSERT INTO users (public_id, username, email, password, avatar_path, bio, is_admin, is_private, followers_count, following_count, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, '', '', true, false, 0, 0, NOW(), NOW())`,
			publicID, username, adminEmail, string(hashedPw),
		)
		if err != nil {
			log.Printf("[seedAdmin] failed to create admin: %v\n", err)
			return err
		}
		return nil
	}

	if err == nil && !isAdmin {
		log.Println("[seedAdmin] Ensuring admin flag is set for:", adminEmail)
		_, err = db.ExecContext(ctx,
			"UPDATE users SET is_admin = true WHERE id = $1", existingID,
		)
		return err
	}

	return err
}

// ===============================
// SERVER INITIALIZATION
// ===============================
func (server *Server) Initialize(DbUser, DbPassword, DbPort, DbHost, DbName string) {
	var dsn string

	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		dsn = os.Getenv("DATABASE_URL")
		if dsn != "" && !strings.Contains(dsn, "sslmode=") {
			if strings.Contains(dsn, "?") {
				dsn += "&sslmode=require"
			} else {
				dsn += "?sslmode=require"
			}
		}
	} else {
		dsn = fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
			DbHost, DbUser, DbPassword, DbName, DbPort,
		)
	}

	db, err := appdb.Connect(dsn)
	if err != nil {
		log.Fatalf("Cannot connect to Postgres: %v", err)
	}
	server.DB = db

	// Optional read replica. READ_DATABASE_URL points at the replica
	// service (k8s service `postgres-replica`). When unset — common
	// during local dev — read traffic transparently falls back to the
	// primary so handlers don't need a code-path branch.
	if readDSN := os.Getenv("READ_DATABASE_URL"); readDSN != "" {
		readDB, err := appdb.ConnectRead(readDSN)
		if err != nil {
			log.Printf("warning: could not connect to read replica, falling back to primary: %v", err)
			server.ReadDB = db
		} else {
			server.ReadDB = readDB
		}
	} else {
		server.ReadDB = db
	}

	// Redis init (safe failure)
	if err := cache.InitFromEnv(); err != nil {
		log.Printf("warning: could not connect to redis: %v", err)
	}

	// Build the S3 client once. Failure is non-fatal — uploads will
	// reject at the handler level if S3Client is nil — but every other
	// part of the API still comes up.
	server.S3Client = initS3Client()

	// SECURE ADMIN CREATION
	if err := seedAdmin(server.DB); err != nil {
		log.Printf("error seeding admin user: %v\n", err)
	}

	r := chi.NewRouter()

	r.Use(chiMiddleware.RealIP)

	// SentryMiddleware replaces chi's Recoverer — it captures the
	// panic to Sentry (with path + method + viewer tags) AND
	// re-panics into chi's default error rendering so clients still
	// get a 500 response. When SENTRY_DSN is empty, Init is a no-op
	// and this middleware becomes a transparent pass-through.
	r.Use(middlewares.SentryMiddleware())
	r.Use(chiMiddleware.Recoverer)

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Anon-Id", "X-Internal-Key", "Connect-Protocol-Version", "Connect-Timeout-Ms"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// ETag middleware. Self-disables for non-GET methods (so all the
	// ConnectRPC POSTs pass through untouched) and for streaming
	// responses (SSE). The wins land on /health and any future plain
	// GETs we add — small but free.
	r.Use(middlewares.ETagMiddleware())

	// Read-after-write: if the client just performed a write, route
	// reads to the primary for 5s to avoid stale-read surprises.
	r.Use(middlewares.ReadAfterWriteMiddleware())

	// Prometheus HTTP metrics. Self-skips /metrics to avoid a feedback
	// loop. Must be after Recoverer so panics still get observed with a
	// 500 status rather than corrupting the counters.
	r.Use(middlewares.MetricsMiddleware())

	// Share / SPA-crawler infrastructure. Constructed up here so its
	// middleware can install before any routes — chi panics if r.Use()
	// runs after any route has been registered. The same handler gets
	// re-used below to mount the explicit /m/{short} + /b/{short}
	// short-URL routes.
	shareHandler := share.NewHandler(server.DB, server.ReadDB)

	// Intercepts crawler GETs to long SPA URLs (/users/{u}/matchup/{id},
	// /brackets/{id}, /users/{u}, /c/{slug}) and serves rich OG HTML
	// inline. Humans pass through to the SPA. Non-GET / non-crawler
	// requests pass through untouched.
	r.Use(shareHandler.SPACrawlerMiddleware)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"backend running"}`))
	})

	// Mobile app deep-link manifests. Apple's CDN fetches the AASA;
	// Android reads assetlinks.json when verifying the app's claim
	// on this domain. Mounted before the SPA proxy so the NotFound
	// handler never forwards these to the React dev server (which
	// would 404 + break Apple's dispatch validator). Graceful 404
	// when the required env vars aren't set — dev without iOS /
	// Android config just doesn't serve them.
	r.Get("/.well-known/apple-app-site-association", AppleAppSiteAssociationHandler())
	r.Get("/.well-known/assetlinks.json", AndroidAssetLinksHandler())

	// Prometheus scrape endpoint. Exposes HTTP metrics (from our
	// middleware) plus the default Go runtime + process metrics
	// (goroutines, GC pauses, memory, file descriptors). Unauthenticated
	// by design — Prometheus lives in-cluster and scrapes by service
	// name; if this ever becomes internet-facing, put an auth layer at
	// the ingress, not here.
	r.Handle("/metrics", promhttp.Handler())

	// Share / preview routes — MUST be mounted before Connect RPC so
	// `/m/{shortID}` etc. aren't swallowed by a catch-all. These serve
	// plain HTML + PNG, not JSON — distinct from everything else on
	// the router. The handler itself + its SPACrawlerMiddleware were
	// set up above (the middleware had to land before any route was
	// registered).
	shareHandler.Mount(r)

	// Share attribution beacon. Frontend sends one POST per landing
	// with a `?ref=<channel>` query; we increment a Prometheus counter
	// and return 204. Fire-and-forget by design.
	r.Post("/share/landed", ShareLandedHandler)

	server.Router = r
	initializeConnectRoutes(r, server.DB, server.ReadDB, server.S3Client)

	// Local-dev convenience: if FRONTEND_PROXY is set (e.g.
	// "http://localhost:3000"), forward any unmatched HTTP request to
	// the React dev server. This lets a single Cloudflare quick-tunnel
	// serve both the OG-scraping API routes AND the SPA, so a human
	// tapping the /m/{id} short link gets 302'd to the SPA path and
	// lands on the actual page instead of 404'ing the API.
	//
	// Unset in production — real deployments have nginx / ingress
	// doing this split and shouldn't let Go forward arbitrary traffic.
	if target := strings.TrimRight(os.Getenv("FRONTEND_PROXY"), "/"); target != "" {
		u, err := url.Parse(target)
		if err != nil {
			log.Printf("FRONTEND_PROXY invalid URL %q: %v", target, err)
		} else {
			proxy := httputil.NewSingleHostReverseProxy(u)
			// Preserve Host header so tunnels and nginx-style absolute
			// redirects behave.
			defaultDirector := proxy.Director
			proxy.Director = func(req *http.Request) {
				defaultDirector(req)
				req.Host = u.Host
			}
			r.NotFound(func(w http.ResponseWriter, rq *http.Request) {
				proxy.ServeHTTP(w, rq)
			})
			log.Printf("frontend proxy: forwarding unmatched routes to %s", target)
		}
	}
}

func (server *Server) Run(addr string) {
	// Bind explicitly via net.Listen so we surface a real error if the
	// socket can't be created — http.ListenAndServe swallows the bind
	// error inside its return value, which is easy to miss when the only
	// log is "starting…". On Fly Machines the private IP is IPv6-only,
	// so the listener has to dual-stack via [::]; rewrite ":8888" to
	// "[::]:8888" so the listener is IPv6 with v4-mapped accept enabled.
	if strings.HasPrefix(addr, ":") {
		addr = "[::]" + addr
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}
	log.Printf("listening on %s (actual=%s)", addr, ln.Addr().String())
	log.Fatal(http.Serve(ln, server.Router))
}

func splitCSV(input string) []string {
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
