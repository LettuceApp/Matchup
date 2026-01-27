package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"Matchup/cache"
	docs "Matchup/docs"
	"Matchup/middlewares"
	"Matchup/models"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/twinj/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Server struct {
	DB     *gorm.DB
	Router *gin.Engine
}

// ===============================
// SECURE ADMIN SEEDING
// ===============================
func seedAdmin(db *gorm.DB) error {
	adminEmail := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	adminPassword := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))

	// If environment vars aren't provided, do NOTHING.
	if adminEmail == "" || adminPassword == "" {
		log.Println("[seedAdmin] ADMIN_EMAIL or ADMIN_PASSWORD not set — skipping admin creation.")
		return nil
	}

	var existing models.User
	err := db.Where("email = ?", adminEmail).First(&existing).Error

	// If admin does not exist, create them
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.Println("[seedAdmin] Creating initial admin:", adminEmail)

		admin := models.User{
			Username: strings.Split(adminEmail, "@")[0],
			Email:    adminEmail,
			Password: adminPassword,
			IsAdmin:  true,
		}

		admin.Prepare()

		if msgs := admin.Validate(""); len(msgs) > 0 {
			log.Printf("[seedAdmin] validation failed: %+v\n", msgs)
			return nil
		}

		_, err = admin.SaveUser(db)
		if err != nil {
			log.Printf("[seedAdmin] failed to create admin: %v\n", err)
			return err
		}

		return nil
	}

	// If admin exists, ensure they stay admin
	if err == nil && !existing.IsAdmin {
		log.Println("[seedAdmin] Ensuring admin flag is set for:", adminEmail)
		return db.Model(&existing).Update("is_admin", true).Error
	}

	return err
}

var errList = make(map[string]string)

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

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Cannot connect to Postgres: %v", err)
	}
	server.DB = db

	// Auto Migrations
	if err := server.DB.AutoMigrate(
		&models.User{},
		&models.Follow{},
		&models.AnonymousDevice{},
		&models.Matchup{},
		&models.ResetPassword{},
		&models.Like{},
		&models.MatchupVote{},
		&models.MatchupVoteRollup{},
		&models.BracketLike{},
		&models.BracketComment{},
		&models.Comment{},
		&models.MatchupItem{},
		&models.Bracket{},
	); err != nil {
		log.Fatalf("Error migrating database: %v", err)
	}
	if err := ensureFollowConstraints(server.DB); err != nil {
		log.Printf("warning: follow constraints not ensured: %v", err)
	}
	if err := ensureFollowCounterDefaults(server.DB); err != nil {
		log.Printf("warning: follow counters not normalized: %v", err)
	}
	if err := ensureMatchupVoteConstraints(server.DB); err != nil {
		log.Printf("warning: matchup vote constraints not ensured: %v", err)
	}
	if err := ensurePublicIDColumns(server.DB); err != nil {
		log.Printf("warning: public_id columns not ensured: %v", err)
	}
	if err := ensureUsernameConstraints(server.DB); err != nil {
		log.Printf("warning: username constraints not ensured: %v", err)
	}

	// Redis init (safe failure)
	if err := cache.InitFromEnv(); err != nil {
		log.Printf("warning: could not connect to redis: %v", err)
	}

	// SECURE ADMIN CREATION
	if err := seedAdmin(server.DB); err != nil {
		log.Printf("error seeding admin user: %v\n", err)
	}

	server.Router = gin.Default()
	server.Router.Use(middlewares.CORSMiddleware())
	server.Router.Use(middlewares.RateLimitMiddleware())
	server.initializeRoutes()

	if os.Getenv("APP_ENV") != "production" {
		if schemes := strings.TrimSpace(os.Getenv("SWAGGER_SCHEMES")); schemes != "" {
			docs.SwaggerInfo.Schemes = splitCSV(schemes)
		} else {
			docs.SwaggerInfo.Schemes = []string{"http"}
		}
		if host := strings.TrimSpace(os.Getenv("SWAGGER_HOST")); host != "" {
			docs.SwaggerInfo.Host = host
		}
		if os.Getenv("SWAGGER_DOCS_ONLY") != "" {
			docs.SwaggerInfo.Host = "docs-only.invalid"
		}
		server.Router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}
}

func (server *Server) Run(addr string) {
	log.Fatal(http.ListenAndServe(addr, server.Router))
}

func ensureFollowConstraints(db *gorm.DB) error {
	var count int64
	if err := db.Raw(
		"SELECT COUNT(1) FROM pg_constraint WHERE conname = ?",
		"follows_no_self_follow",
	).Scan(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		if err := db.Exec(
			"ALTER TABLE follows ADD CONSTRAINT follows_no_self_follow CHECK (follower_id <> followed_id)",
		).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureFollowCounterDefaults(db *gorm.DB) error {
	if err := db.Exec(
		"UPDATE users SET followers_count = 0 WHERE followers_count IS NULL",
	).Error; err != nil {
		return err
	}
	if err := db.Exec(
		"UPDATE users SET following_count = 0 WHERE following_count IS NULL",
	).Error; err != nil {
		return err
	}
	return nil
}

func ensureMatchupVoteConstraints(db *gorm.DB) error {
	var hasAnonColumn int64
	if err := db.Raw(
		"SELECT COUNT(1) FROM information_schema.columns WHERE table_name = 'matchup_votes' AND column_name = 'anon_id'",
	).Scan(&hasAnonColumn).Error; err != nil {
		return err
	}
	if hasAnonColumn == 0 {
		var hasDeviceColumn int64
		if err := db.Raw(
			"SELECT COUNT(1) FROM information_schema.columns WHERE table_name = 'matchup_votes' AND column_name = 'device_id'",
		).Scan(&hasDeviceColumn).Error; err != nil {
			return err
		}
		if hasDeviceColumn > 0 {
			if err := db.Exec(
				"ALTER TABLE matchup_votes RENAME COLUMN device_id TO anon_id",
			).Error; err != nil {
				return err
			}
		} else {
			if err := db.Exec(
				"ALTER TABLE matchup_votes ADD COLUMN anon_id varchar(36)",
			).Error; err != nil {
				return err
			}
		}
	}

	var hasMatchupPublicIDColumn int64
	if err := db.Raw(
		"SELECT COUNT(1) FROM information_schema.columns WHERE table_name = 'matchup_votes' AND column_name = 'matchup_public_id'",
	).Scan(&hasMatchupPublicIDColumn).Error; err != nil {
		return err
	}
	if hasMatchupPublicIDColumn == 0 {
		if err := db.Exec(
			"ALTER TABLE matchup_votes ADD COLUMN matchup_public_id uuid",
		).Error; err != nil {
			return err
		}
	}

	var hasMatchupItemPublicIDColumn int64
	if err := db.Raw(
		"SELECT COUNT(1) FROM information_schema.columns WHERE table_name = 'matchup_votes' AND column_name = 'matchup_item_public_id'",
	).Scan(&hasMatchupItemPublicIDColumn).Error; err != nil {
		return err
	}
	if hasMatchupItemPublicIDColumn == 0 {
		if err := db.Exec(
			"ALTER TABLE matchup_votes ADD COLUMN matchup_item_public_id uuid",
		).Error; err != nil {
			return err
		}
	}

	if err := db.Exec(`
		UPDATE matchup_votes mv
		SET matchup_public_id = m.public_id
		FROM matchups m
		WHERE mv.matchup_public_id IS NULL AND mv.matchup_id = m.id
	`).Error; err != nil {
		return err
	}

	if err := db.Exec(`
		UPDATE matchup_votes mv
		SET matchup_item_public_id = mi.public_id
		FROM matchup_items mi
		WHERE mv.matchup_item_public_id IS NULL AND mv.matchup_item_id = mi.id
	`).Error; err != nil {
		return err
	}

	var isNullable string
	if err := db.Raw(
		"SELECT is_nullable FROM information_schema.columns WHERE table_name = 'matchup_votes' AND column_name = 'user_id'",
	).Scan(&isNullable).Error; err != nil {
		return err
	}
	if strings.EqualFold(isNullable, "NO") {
		if err := db.Exec(
			"ALTER TABLE matchup_votes ALTER COLUMN user_id DROP NOT NULL",
		).Error; err != nil {
			return err
		}
	}

	var count int64
	if err := db.Raw(
		"SELECT COUNT(1) FROM pg_constraint WHERE conname = ?",
		"matchup_votes_user_or_device",
	).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		if err := db.Exec(
			"ALTER TABLE matchup_votes DROP CONSTRAINT matchup_votes_user_or_device",
		).Error; err != nil {
			return err
		}
	}

	if err := db.Raw(
		"SELECT COUNT(1) FROM pg_constraint WHERE conname = ?",
		"matchup_votes_user_or_anon",
	).Scan(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		if err := db.Exec(
			"ALTER TABLE matchup_votes ADD CONSTRAINT matchup_votes_user_or_anon CHECK ((user_id IS NOT NULL AND anon_id IS NULL) OR (user_id IS NULL AND anon_id IS NOT NULL))",
		).Error; err != nil {
			return err
		}
	}

	if err := db.Raw(
		"SELECT COUNT(1) FROM pg_indexes WHERE indexname = ?",
		"idx_matchup_vote_device_matchup",
	).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		if err := db.Exec(
			"DROP INDEX idx_matchup_vote_device_matchup",
		).Error; err != nil {
			return err
		}
	}

	if err := db.Raw(
		"SELECT COUNT(1) FROM pg_indexes WHERE indexname = ?",
		"idx_matchup_vote_user_matchup",
	).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		if err := db.Exec(
			"DROP INDEX idx_matchup_vote_user_matchup",
		).Error; err != nil {
			return err
		}
	}

	if err := db.Raw(
		"SELECT COUNT(1) FROM pg_indexes WHERE indexname = ?",
		"idx_matchup_vote_anon_matchup",
	).Scan(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		if err := db.Exec(
			"CREATE UNIQUE INDEX idx_matchup_vote_anon_matchup ON matchup_votes (matchup_public_id, anon_id) WHERE anon_id IS NOT NULL",
		).Error; err != nil {
			return err
		}
	}

	if err := db.Raw(
		"SELECT COUNT(1) FROM pg_indexes WHERE indexname = ?",
		"idx_matchup_vote_user_matchup_public",
	).Scan(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		if err := db.Exec(
			"CREATE UNIQUE INDEX idx_matchup_vote_user_matchup_public ON matchup_votes (matchup_public_id, user_id) WHERE user_id IS NOT NULL",
		).Error; err != nil {
			return err
		}
	}

	return nil
}

func ensurePublicIDColumns(db *gorm.DB) error {
	uuidGenerator := ""
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto").Error; err == nil {
		uuidGenerator = "gen_random_uuid()"
	} else if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`).Error; err == nil {
		uuidGenerator = "uuid_generate_v4()"
	}

	type tableConfig struct {
		name       string
		constraint string
		index      string
	}

	tables := []tableConfig{
		{name: "users", constraint: "users_public_id_unique"},
		{name: "matchups", constraint: "matchups_public_id_unique"},
		{name: "brackets", constraint: "brackets_public_id_unique"},
		{name: "comments", constraint: "comments_public_id_unique"},
		{name: "bracket_comments", constraint: "bracket_comments_public_id_unique"},
		{name: "likes", constraint: "likes_public_id_unique"},
		{name: "bracket_likes", constraint: "bracket_likes_public_id_unique"},
		{name: "matchup_votes", constraint: "matchup_votes_public_id_unique"},
		{name: "matchup_items", constraint: "matchup_items_public_id_unique"},
	}

	for _, table := range tables {
		var columnCount int64
		if err := db.Raw(
			"SELECT COUNT(1) FROM information_schema.columns WHERE table_name = ? AND column_name = 'public_id'",
			table.name,
		).Scan(&columnCount).Error; err != nil {
			return err
		}
		if columnCount == 0 {
			if err := db.Exec(
				fmt.Sprintf("ALTER TABLE %s ADD COLUMN public_id uuid", table.name),
			).Error; err != nil {
				return err
			}
		}

		if uuidGenerator != "" {
			if err := db.Exec(
				fmt.Sprintf("ALTER TABLE %s ALTER COLUMN public_id SET DEFAULT %s", table.name, uuidGenerator),
			).Error; err != nil {
				return err
			}

			if err := db.Exec(
				fmt.Sprintf("UPDATE %s SET public_id = %s WHERE public_id IS NULL", table.name, uuidGenerator),
			).Error; err != nil {
				return err
			}
		} else {
			var ids []uint
			if err := db.Table(table.name).
				Where("public_id IS NULL").
				Pluck("id", &ids).Error; err != nil {
				return err
			}
			for _, id := range ids {
				if err := db.Table(table.name).
					Where("id = ?", id).
					Update("public_id", uuid.NewV4().String()).Error; err != nil {
					return err
				}
			}
		}

		if err := db.Exec(
			fmt.Sprintf("ALTER TABLE %s ALTER COLUMN public_id SET NOT NULL", table.name),
		).Error; err != nil {
			return err
		}

		var constraintCount int64
		if err := db.Raw(
			"SELECT COUNT(1) FROM pg_constraint WHERE conname = ?",
			table.constraint,
		).Scan(&constraintCount).Error; err != nil {
			return err
		}
		if constraintCount == 0 {
			if err := db.Exec(
				fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (public_id)", table.name, table.constraint),
			).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func ensureUsernameConstraints(db *gorm.DB) error {
	var columnCount int64
	if err := db.Raw(
		"SELECT COUNT(1) FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'username'",
	).Scan(&columnCount).Error; err != nil {
		return err
	}
	if columnCount == 0 {
		if err := db.Exec(
			"ALTER TABLE users ADD COLUMN username text",
		).Error; err != nil {
			return err
		}
	}

	if err := db.Exec(
		"UPDATE users SET username = lower(username) WHERE username IS NOT NULL",
	).Error; err != nil {
		return err
	}

	if err := db.Exec(
		"UPDATE users SET username = concat('user_', left(public_id::text, 8)) WHERE username IS NULL OR btrim(username) = ''",
	).Error; err != nil {
		return err
	}

	var constraintCount int64
	if err := db.Raw(
		"SELECT COUNT(1) FROM pg_constraint WHERE conname = ?",
		"users_username_not_empty",
	).Scan(&constraintCount).Error; err != nil {
		return err
	}
	if constraintCount == 0 {
		if err := db.Exec(
			"ALTER TABLE users ADD CONSTRAINT users_username_not_empty CHECK (btrim(username) <> '')",
		).Error; err != nil {
			return err
		}
	}

	var indexCount int64
	if err := db.Raw(
		"SELECT COUNT(1) FROM pg_indexes WHERE tablename = 'users' AND indexname = ?",
		"idx_users_username",
	).Scan(&indexCount).Error; err != nil {
		return err
	}
	if indexCount == 0 {
		if err := db.Exec(
			"CREATE INDEX idx_users_username ON users (username)",
		).Error; err != nil {
			return err
		}
	}

	return nil
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
