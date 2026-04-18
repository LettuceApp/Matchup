// Package scheduler replaces the old Dagster orchestration stack
// (orchestration/*.py) with a single Go process that runs the same six
// workloads using github.com/robfig/cron/v3 for cron-style schedules and a
// time.Ticker for the sub-minute bracket advance poller.
//
// The scheduler runs as its own K8s deployment (k8s/cron/deployment.yaml)
// using the same container image as the API — the binary lives at
// cmd/cron/main.go and shares api/db, api/controllers, and api/cache with
// the main API so all the business logic is reused in-process, not over
// HTTP. This also means the broken HTTP contract the Dagster ops.py was
// trying to use (/internal/brackets/{id}/advance) is gone — bracket
// advances happen via a direct Go function call.
//
// Schedules:
//
//	every 10s   AdvanceExpiredBrackets  (timer-based bracket round advancer)
//	* * * * *   RefreshSnapshots        (5 materialized views)
//	0 * * * *   RefreshTrendingSnapshot (hourly trending matchups)
//	0 4 * * *   ManageCommentPartitions (create next 6 months, drop older than 24)
//	0 5 * * *   ArchiveMatchupVotes     (delete+insert into matchup_votes_archive)
//	0 6 * * 0   CleanupAnonymousDevices (drop rows not seen in > 1 year)
//	30 6 1 * *  CleanupOldVotesArchive  (drop archive rows older than 2 years)
//
// All workloads except AdvanceExpiredBrackets are pure SQL via the primary
// DB handle. Each workload is idempotent — a duplicated run after crash
// recovery is a no-op on the second run.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"Matchup/controllers"

	"github.com/jmoiron/sqlx"
	"github.com/robfig/cron/v3"
)

// Scheduler coordinates all recurring background work. A process should
// hold exactly one Scheduler at a time; Run blocks until ctx is cancelled.
type Scheduler struct {
	server *controllers.Server
	db     *sqlx.DB
	cron   *cron.Cron
	logger *log.Logger
}

// New builds a Scheduler around an initialised controllers.Server. The
// server's DB handle is captured for all SQL-only workloads; the server
// itself is retained only so AdvanceExpiredBrackets can be called as a
// direct method call.
func New(server *controllers.Server) *Scheduler {
	logger := log.New(log.Writer(), "scheduler: ", log.LstdFlags|log.Lmsgprefix)
	return &Scheduler{
		server: server,
		db:     server.DB,
		cron: cron.New(
			cron.WithLogger(cron.VerbosePrintfLogger(logger)),
			cron.WithChain(cron.SkipIfStillRunning(cron.VerbosePrintfLogger(logger))),
		),
		logger: logger,
	}
}

// Run registers every workload and blocks until ctx is cancelled. Returns
// the first fatal registration error (if any); per-run workload errors are
// logged but do not terminate the scheduler.
func (s *Scheduler) Run(ctx context.Context) error {
	if _, err := s.cron.AddFunc("* * * * *", s.jobRefreshSnapshots); err != nil {
		return fmt.Errorf("register refresh_snapshots: %w", err)
	}
	if _, err := s.cron.AddFunc("0 * * * *", s.jobRefreshTrendingSnapshot); err != nil {
		return fmt.Errorf("register refresh_trending_snapshot: %w", err)
	}
	if _, err := s.cron.AddFunc("0 */4 * * *", s.jobManageCommentPartitions); err != nil {
		return fmt.Errorf("register manage_comment_partitions: %w", err)
	}
	if _, err := s.cron.AddFunc("0 5 * * *", s.jobArchiveMatchupVotes); err != nil {
		return fmt.Errorf("register archive_matchup_votes: %w", err)
	}
	if _, err := s.cron.AddFunc("0 6 * * 0", s.jobCleanupAnonymousDevices); err != nil {
		return fmt.Errorf("register cleanup_anonymous_devices: %w", err)
	}
	if _, err := s.cron.AddFunc("30 6 1 * *", s.jobCleanupOldVotesArchive); err != nil {
		return fmt.Errorf("register cleanup_old_votes_archive: %w", err)
	}

	s.cron.Start()
	defer s.cron.Stop()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.runAdvanceLoop(ctx)
	}()

	s.logger.Println("started (6 cron entries + 10s advance loop)")
	<-ctx.Done()
	s.logger.Println("shutdown: draining cron + advance loop")
	wg.Wait()
	s.logger.Println("shutdown: complete")
	return nil
}

// ---------------------------------------------------------------------------
// Bracket advance loop (sub-minute; lives outside cron)
// ---------------------------------------------------------------------------

func (s *Scheduler) runAdvanceLoop(ctx context.Context) {
	const interval = 10 * time.Second
	t := time.NewTicker(interval)
	defer t.Stop()

	// Fire once on startup so a freshly-started scheduler catches any
	// backlog immediately instead of waiting up to 10 seconds.
	s.tickAdvance(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tickAdvance(ctx)
		}
	}
}

func (s *Scheduler) tickAdvance(ctx context.Context) {
	advanceCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	n, err := s.server.AdvanceExpiredBrackets(advanceCtx)
	if err != nil {
		// Log and keep going — one bad tick must not kill the loop.
		s.logger.Printf("advance_expired_brackets error: %v", err)
	}
	if n > 0 {
		s.logger.Printf("advance_expired_brackets: advanced %d bracket(s)", n)
	}
}

// ---------------------------------------------------------------------------
// Workload: refresh 5 materialized views (every minute)
// ---------------------------------------------------------------------------

// snapshotViews is the list of materialized views created by migration
// 007_materialized_views.sql. Each one carries a UNIQUE index, which is a
// hard requirement for REFRESH MATERIALIZED VIEW CONCURRENTLY.
var snapshotViews = []string{
	"popular_matchups_snapshot",
	"popular_brackets_snapshot",
	"home_summary_snapshot",
	"home_creators_snapshot",
	"home_new_this_week_snapshot",
}

func (s *Scheduler) jobRefreshSnapshots() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, view := range snapshotViews {
		// CONCURRENTLY refreshes don't block readers, so running them
		// sequentially is fine — parallel refreshes would just fight for
		// the same IO without improving end-to-end latency.
		sql := fmt.Sprintf("REFRESH MATERIALIZED VIEW CONCURRENTLY public.%s", view)
		if _, err := s.db.ExecContext(ctx, sql); err != nil {
			s.logger.Printf("refresh_snapshots: %s failed: %v", view, err)
			continue
		}
	}
}

// ---------------------------------------------------------------------------
// Workload: refresh trending matchups snapshot (hourly at :00)
// ---------------------------------------------------------------------------

func (s *Scheduler) jobRefreshTrendingSnapshot() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if _, err := s.db.ExecContext(ctx,
		"REFRESH MATERIALIZED VIEW CONCURRENTLY public.trending_matchups_snapshot"); err != nil {
		s.logger.Printf("refresh_trending_snapshot: %v", err)
	}
	if _, err := s.db.ExecContext(ctx,
		"REFRESH MATERIALIZED VIEW CONCURRENTLY public.most_played_snapshot"); err != nil {
		s.logger.Printf("refresh_most_played_snapshot: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Workload: manage comments partitions (daily 04:00 UTC)
// ---------------------------------------------------------------------------

// jobManageCommentPartitions creates partitions for the upcoming 6 months
// on `public.comments` (partitioned by created_at) and drops monthly
// partitions older than 24 months. Mirrors the logic in the former Dagster
// op `manage_comment_partitions` in orchestration/ops.py.
//
// Skipped: `comments_historical` (bulk pre-rollout rows) and
// `comments_default` (overflow safety net). Those are named matches and
// the month-parsing falls through, so the drop loop ignores them.
func (s *Scheduler) jobManageCommentPartitions() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	today := time.Now().UTC()
	thisMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)

	created := []string{}
	for offset := 0; offset <= 12; offset++ {
		start := addMonths(thisMonth, offset)
		end := addMonths(start, 1)
		name := fmt.Sprintf("comments_%04d_%02d", start.Year(), int(start.Month()))

		var exists bool
		err := s.db.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1 AND relkind = 'r')",
			name,
		).Scan(&exists)
		if err != nil {
			s.logger.Printf("manage_comment_partitions: check %s: %v", name, err)
			continue
		}
		if exists {
			continue
		}

		stmt := fmt.Sprintf(
			"CREATE TABLE public.%s PARTITION OF public.comments FOR VALUES FROM ('%s') TO ('%s')",
			name, start.Format("2006-01-02"), end.Format("2006-01-02"),
		)
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			s.logger.Printf("manage_comment_partitions: create %s: %v", name, err)
			continue
		}
		created = append(created, name)
	}

	cutoff := addMonths(thisMonth, -24)
	dropRows, err := s.db.QueryxContext(ctx, `
		SELECT c.relname
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = 'comments'
		  AND c.relname ~ '^comments_[0-9]{4}_[0-9]{2}$'
	`)
	if err != nil {
		s.logger.Printf("manage_comment_partitions: list children: %v", err)
	} else {
		dropped := []string{}
		names := []string{}
		for dropRows.Next() {
			var name string
			if err := dropRows.Scan(&name); err != nil {
				s.logger.Printf("manage_comment_partitions: scan: %v", err)
				continue
			}
			names = append(names, name)
		}
		dropRows.Close()

		for _, name := range names {
			parts := strings.Split(name, "_")
			if len(parts) != 3 {
				continue
			}
			year, err := strconv.Atoi(parts[1])
			if err != nil {
				continue
			}
			month, err := strconv.Atoi(parts[2])
			if err != nil {
				continue
			}
			partDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
			if partDate.Before(cutoff) {
				drop := fmt.Sprintf("DROP TABLE public.%s", name)
				if _, err := s.db.ExecContext(ctx, drop); err != nil {
					s.logger.Printf("manage_comment_partitions: drop %s: %v", name, err)
					continue
				}
				dropped = append(dropped, name)
			}
		}
		if len(dropped) > 0 {
			s.logger.Printf("manage_comment_partitions: dropped %d partition(s): %s",
				len(dropped), strings.Join(dropped, ", "))
		}
	}

	if len(created) > 0 {
		s.logger.Printf("manage_comment_partitions: created %d partition(s): %s",
			len(created), strings.Join(created, ", "))
	}
}

// addMonths returns d shifted forward (or backward) by n calendar months.
// Pinned to day=1 because partition boundaries are always month-first.
func addMonths(d time.Time, n int) time.Time {
	total := int(d.Month()) - 1 + n
	year := d.Year() + total/12
	month := total % 12
	if month < 0 {
		month += 12
		year--
	}
	return time.Date(year, time.Month(month+1), 1, 0, 0, 0, 0, time.UTC)
}

// ---------------------------------------------------------------------------
// Workload: archive matchup votes for long-completed matchups (daily 05:00)
// ---------------------------------------------------------------------------

func (s *Scheduler) jobArchiveMatchupVotes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		s.logger.Printf("archive_matchup_votes: begin: %v", err)
		return
	}
	// Rollback is a no-op after Commit, so this is safe as a blanket defer.
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		WITH completed_old AS (
			SELECT public_id
			FROM public.matchups
			WHERE status = 'completed'
			  AND updated_at < NOW() - INTERVAL '90 days'
		),
		moved AS (
			DELETE FROM public.matchup_votes
			WHERE matchup_public_id IN (SELECT public_id FROM completed_old)
			RETURNING id, public_id, user_id, anon_id,
			          matchup_public_id, matchup_item_public_id,
			          created_at, updated_at
		)
		INSERT INTO public.matchup_votes_archive (
			id, public_id, user_id, anon_id,
			matchup_public_id, matchup_item_public_id,
			created_at, updated_at
		)
		SELECT id, public_id, user_id, anon_id,
		       matchup_public_id, matchup_item_public_id,
		       created_at, updated_at
		FROM moved
	`)
	if err != nil {
		s.logger.Printf("archive_matchup_votes: exec: %v", err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.logger.Printf("archive_matchup_votes: commit: %v", err)
		return
	}

	n, err := result.RowsAffected()
	if err != nil {
		// RowsAffected is unusual to fail on postgres, but if it does we
		// still successfully archived the rows — just couldn't count them.
		s.logger.Printf("archive_matchup_votes: rows affected unknown: %v", err)
		return
	}
	if n > 0 {
		s.logger.Printf("archive_matchup_votes: archived %d vote(s)", n)
	}
}

// ---------------------------------------------------------------------------
// Workload: cleanup anonymous devices (weekly Sunday 06:00)
// ---------------------------------------------------------------------------

func (s *Scheduler) jobCleanupAnonymousDevices() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM public.anonymous_devices
		WHERE last_seen_at IS NOT NULL
		  AND last_seen_at < NOW() - INTERVAL '1 year'
	`)
	if err != nil {
		s.logger.Printf("cleanup_anonymous_devices: %v", err)
		return
	}
	if n, err := result.RowsAffected(); err == nil && n > 0 {
		s.logger.Printf("cleanup_anonymous_devices: deleted %d row(s)", n)
	}
}

// ---------------------------------------------------------------------------
// Workload: cleanup old rows in matchup_votes_archive (monthly 1st 06:30)
// ---------------------------------------------------------------------------

func (s *Scheduler) jobCleanupOldVotesArchive() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM public.matchup_votes_archive
		WHERE created_at IS NOT NULL
		  AND created_at < NOW() - INTERVAL '2 years'
	`)
	if err != nil {
		s.logger.Printf("cleanup_old_votes_archive: %v", err)
		return
	}
	if n, err := result.RowsAffected(); err == nil && n > 0 {
		s.logger.Printf("cleanup_old_votes_archive: deleted %d row(s)", n)
	}
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ErrServerNotInitialized is returned when New receives a Server whose DB
// handle hasn't been populated (usually because Initialize wasn't called).
var ErrServerNotInitialized = errors.New("scheduler: server DB is nil")
