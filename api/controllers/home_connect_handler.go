package controllers

import (
	"context"
	"log"
	"time"

	bracketv1 "Matchup/gen/bracket/v1"
	homev1 "Matchup/gen/home/v1"
	"Matchup/gen/home/v1/homev1connect"
	matchupv1 "Matchup/gen/matchup/v1"
	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
)

var _ homev1connect.HomeServiceHandler = (*HomeHandler)(nil)

// HomeHandler implements homev1connect.HomeServiceHandler.
type HomeHandler struct {
	DB *sqlx.DB
}

func (h *HomeHandler) GetHomeSummary(ctx context.Context, req *connect.Request[homev1.GetHomeSummaryRequest]) (*connect.Response[homev1.GetHomeSummaryResponse], error) {
	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	isAdmin := httpctx.IsAdminRequest(ctx)

	// Popular matchups
	popularMatchupRows := []popularMatchupRow{}
	if err := sqlx.SelectContext(ctx, h.DB, &popularMatchupRows,
		"SELECT * FROM popular_matchups_snapshot ORDER BY rank ASC LIMIT 5"); err != nil {
		if !isMissingRelation(err, "popular_matchups_snapshot") {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		log.Printf("popular matchups snapshot missing: %v", err)
	}

	matchupIDs := make([]uint, 0, len(popularMatchupRows))
	matchupAuthorIDs := make([]uint, 0, len(popularMatchupRows))
	matchupBracketIDs := make([]uint, 0, len(popularMatchupRows))
	matchupBracketAuthorIDs := make([]uint, 0, len(popularMatchupRows))
	for _, row := range popularMatchupRows {
		matchupIDs = append(matchupIDs, row.ID)
		matchupAuthorIDs = append(matchupAuthorIDs, row.AuthorID)
		if row.BracketID != nil {
			matchupBracketIDs = append(matchupBracketIDs, *row.BracketID)
		}
		if row.BracketAuthorID != nil {
			matchupBracketAuthorIDs = append(matchupBracketAuthorIDs, *row.BracketAuthorID)
		}
	}
	matchupPublicIDs := loadMatchupPublicIDMap(h.DB, matchupIDs)
	matchupAuthorPublicIDs := loadUserPublicIDMap(h.DB, matchupAuthorIDs)
	matchupBracketPublicIDs := loadBracketPublicIDMap(h.DB, matchupBracketIDs)
	matchupBracketAuthorPublicIDs := loadUserPublicIDMap(h.DB, matchupBracketAuthorIDs)

	var protoPopularMatchups []*matchupv1.PopularMatchupData
	for i := range popularMatchupRows {
		var matchup models.Matchup
		if err := sqlx.GetContext(ctx, h.DB, &matchup, "SELECT * FROM matchups WHERE id = $1", popularMatchupRows[i].ID); err != nil {
			continue
		}
		if err := sqlx.GetContext(ctx, h.DB, &matchup.Author, "SELECT * FROM users WHERE id = $1", matchup.AuthorID); err != nil {
			continue
		}
		matchup.Author.ProcessAvatarPath()
		allowed, _, err := canViewUserContent(h.DB, viewerID, hasViewer, &matchup.Author, matchup.Visibility, isAdmin)
		if err != nil {
			log.Printf("home summary visibility error: %v", err)
			continue
		}
		if !allowed {
			continue
		}
		var bracketID *string
		if popularMatchupRows[i].BracketID != nil {
			if id := matchupBracketPublicIDs[*popularMatchupRows[i].BracketID]; id != "" {
				bracketID = &id
			}
		}
		var bracketAuthorID *string
		if popularMatchupRows[i].BracketAuthorID != nil {
			if id := matchupBracketAuthorPublicIDs[*popularMatchupRows[i].BracketAuthorID]; id != "" {
				bracketAuthorID = &id
			}
		}
		dto := PopularMatchupDTO{
			ID:              matchupPublicIDs[popularMatchupRows[i].ID],
			Title:           popularMatchupRows[i].Title,
			AuthorID:        matchupAuthorPublicIDs[popularMatchupRows[i].AuthorID],
			BracketID:       bracketID,
			BracketAuthorID: bracketAuthorID,
			Round:           popularMatchupRows[i].Round,
			CurrentRound:    popularMatchupRows[i].CurrentRound,
			Votes:           popularMatchupRows[i].Votes,
			Likes:           popularMatchupRows[i].Likes,
			Comments:        popularMatchupRows[i].Comments,
			EngagementScore: popularMatchupRows[i].EngagementScore,
			Rank:            popularMatchupRows[i].Rank,
		}
		protoPopularMatchups = append(protoPopularMatchups, popularMatchupToProto(dto))
	}

	// Popular brackets — prefer snapshot, fall back to direct table query
	popularBracketRows := []popularBracketRow{}
	snapshotErr := sqlx.SelectContext(ctx, h.DB, &popularBracketRows,
		"SELECT * FROM popular_brackets_snapshot ORDER BY rank ASC LIMIT 12")
	if snapshotErr != nil {
		if !isMissingRelation(snapshotErr, "popular_brackets_snapshot") {
			return nil, connect.NewError(connect.CodeInternal, snapshotErr)
		}
		log.Printf("popular brackets snapshot missing, using direct query: %v", snapshotErr)
	}

	var protoPopularBrackets []*bracketv1.PopularBracketData

	if len(popularBracketRows) > 0 {
		// Use snapshot path
		bracketIDs := make([]uint, 0, len(popularBracketRows))
		bracketAuthorIDs := make([]uint, 0, len(popularBracketRows))
		for _, row := range popularBracketRows {
			bracketIDs = append(bracketIDs, row.ID)
			bracketAuthorIDs = append(bracketAuthorIDs, row.AuthorID)
		}
		bracketPublicIDs := loadBracketPublicIDMap(h.DB, bracketIDs)
		bracketAuthorPublicIDs := loadUserPublicIDMap(h.DB, bracketAuthorIDs)

		for i := range popularBracketRows {
			var bracket models.Bracket
			if err := sqlx.GetContext(ctx, h.DB, &bracket, "SELECT * FROM brackets WHERE id = $1", popularBracketRows[i].ID); err != nil {
				continue
			}
			if err := sqlx.GetContext(ctx, h.DB, &bracket.Author, "SELECT * FROM users WHERE id = $1", bracket.AuthorID); err != nil {
				continue
			}
			bracket.Author.ProcessAvatarPath()
			allowed, _, err := canViewUserContent(h.DB, viewerID, hasViewer, &bracket.Author, bracket.Visibility, isAdmin)
			if err != nil {
				log.Printf("home summary visibility error: %v", err)
				continue
			}
			if !allowed || bracket.Status != "active" {
				continue
			}
			var likesCount int64
			_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM bracket_likes WHERE bracket_id = $1", bracket.ID)
			var commentsCount int64
			_ = sqlx.GetContext(ctx, h.DB, &commentsCount, "SELECT COUNT(*) FROM bracket_comments WHERE bracket_id = $1", bracket.ID)
			dto := PopularBracketDTO{
				ID:              bracketPublicIDs[popularBracketRows[i].ID],
				Title:           popularBracketRows[i].Title,
				AuthorID:        bracketAuthorPublicIDs[popularBracketRows[i].AuthorID],
				CurrentRound:    popularBracketRows[i].CurrentRound,
				Size:            bracket.Size,
				Votes:           0,
				Likes:           likesCount,
				Comments:        commentsCount,
				EngagementScore: popularBracketRows[i].EngagementScore,
				Rank:            popularBracketRows[i].Rank,
			}
			protoPopularBrackets = append(protoPopularBrackets, popularBracketToProto(dto))
		}
	} else {
		// Fallback: query brackets table directly, most recent first
		var fallbackBrackets []models.Bracket
		_ = sqlx.SelectContext(ctx, h.DB, &fallbackBrackets,
			`SELECT b.* FROM brackets b
			 JOIN users u ON u.id = b.author_id
			 WHERE b.status IN ('active', 'completed', 'draft')
			 ORDER BY b.created_at DESC LIMIT 12`)
		bracketIDs := make([]uint, 0, len(fallbackBrackets))
		for i := range fallbackBrackets {
			bracketIDs = append(bracketIDs, fallbackBrackets[i].ID)
		}
		bracketPublicIDs := loadBracketPublicIDMap(h.DB, bracketIDs)

		for i := range fallbackBrackets {
			b := &fallbackBrackets[i]
			if err := sqlx.GetContext(ctx, h.DB, &b.Author, "SELECT * FROM users WHERE id = $1", b.AuthorID); err != nil {
				continue
			}
			b.Author.ProcessAvatarPath()
			allowed, _, err := canViewUserContent(h.DB, viewerID, hasViewer, &b.Author, b.Visibility, isAdmin)
			if err != nil || !allowed {
				continue
			}
			var likesCount int64
			_ = sqlx.GetContext(ctx, h.DB, &likesCount, "SELECT COUNT(*) FROM bracket_likes WHERE bracket_id = $1", b.ID)
			var commentsCount int64
			_ = sqlx.GetContext(ctx, h.DB, &commentsCount, "SELECT COUNT(*) FROM bracket_comments WHERE bracket_id = $1", b.ID)
			authorPublicID := loadUserPublicIDMap(h.DB, []uint{b.AuthorID})[b.AuthorID]
			dto := PopularBracketDTO{
				ID:       bracketPublicIDs[b.ID],
				Title:    b.Title,
				AuthorID: authorPublicID,
				Size:     b.Size,
				Likes:    likesCount,
				Comments: commentsCount,
			}
			protoPopularBrackets = append(protoPopularBrackets, popularBracketToProto(dto))
		}
	}

	// Stats snapshot
	votesToday := int64(0)
	activeMatchups := int64(0)
	activeBrackets := int64(0)
	var summarySnapshot HomeSummarySnapshot
	if err := sqlx.GetContext(ctx, h.DB, &summarySnapshot, "SELECT * FROM home_summary_snapshot LIMIT 1"); err == nil {
		votesToday = summarySnapshot.VotesToday
		activeMatchups = summarySnapshot.ActiveMatchups
		activeBrackets = summarySnapshot.ActiveBrackets
	} else if !isMissingRelation(err, "home_summary_snapshot") {
		log.Printf("home summary snapshot error: %v", err)
	} else {
		voteCutoff := time.Now().Add(-24 * time.Hour)
		_ = sqlx.GetContext(ctx, h.DB, &votesToday, "SELECT COUNT(*) FROM matchup_votes WHERE created_at >= $1", voteCutoff)
		_ = sqlx.GetContext(ctx, h.DB, &activeMatchups,
			"SELECT COUNT(*) FROM matchups WHERE status IN ($1, $2)", matchupStatusActive, matchupStatusPublished)
		_ = sqlx.GetContext(ctx, h.DB, &activeBrackets, "SELECT COUNT(*) FROM brackets WHERE status = $1", "active")
	}

	// Total engagements for viewer
	totalEngagements := 0.0
	if hasViewer && viewerID > 0 {
		s := &Server{DB: h.DB}
		if total, err := s.calculateUserEngagement(viewerID); err == nil {
			totalEngagements = total
		} else {
			log.Printf("calculate user engagement: %v", err)
		}
	}

	// New this week
	var protoNewThisWeek []*homev1.HomeMatchupData
	var newThisWeekSnapshots []HomeMatchupSnapshot
	newThisWeekErr := sqlx.SelectContext(ctx, h.DB, &newThisWeekSnapshots,
		"SELECT * FROM home_new_this_week_snapshot ORDER BY rank ASC LIMIT 6")
	if newThisWeekErr == nil && len(newThisWeekSnapshots) > 0 {
		authorIDs := make([]uint, 0, len(newThisWeekSnapshots))
		authorSeen := map[uint]struct{}{}
		for _, m := range newThisWeekSnapshots {
			if m.AuthorID == 0 {
				continue
			}
			if _, seen := authorSeen[m.AuthorID]; seen {
				continue
			}
			authorSeen[m.AuthorID] = struct{}{}
			authorIDs = append(authorIDs, m.AuthorID)
		}
		authorMap := map[uint]models.User{}
		if len(authorIDs) > 0 {
			query, args, err := sqlx.In("SELECT * FROM users WHERE id IN (?)", authorIDs)
			if err == nil {
				query = h.DB.Rebind(query)
				var authors []models.User
				if err := sqlx.SelectContext(ctx, h.DB, &authors, query, args...); err == nil {
					for _, a := range authors {
						authorMap[a.ID] = a
					}
				}
			}
		}
		snapshotMatchupIDs := make([]uint, 0)
		snapshotBracketIDs := make([]uint, 0)
		for _, m := range newThisWeekSnapshots {
			if m.ID != 0 {
				snapshotMatchupIDs = append(snapshotMatchupIDs, m.ID)
			}
			if m.BracketID != nil {
				snapshotBracketIDs = append(snapshotBracketIDs, *m.BracketID)
			}
		}
		snapshotMatchupPublicIDs := loadMatchupPublicIDMap(h.DB, snapshotMatchupIDs)
		snapshotBracketPublicIDs := loadBracketPublicIDMap(h.DB, snapshotBracketIDs)

		for _, m := range newThisWeekSnapshots {
			author, ok := authorMap[m.AuthorID]
			if !ok {
				continue
			}
			visibility := m.Visibility
			if visibility == "" {
				visibility = "public"
			}
			allowed, _, err := canViewUserContent(h.DB, viewerID, hasViewer, &author, visibility, isAdmin)
			if err != nil {
				log.Printf("home summary visibility error: %v", err)
				continue
			}
			if !allowed {
				continue
			}
			var snapshotBracketID *string
			if m.BracketID != nil {
				if id := snapshotBracketPublicIDs[*m.BracketID]; id != "" {
					snapshotBracketID = &id
				}
			}
			protoNewThisWeek = append(protoNewThisWeek, &homev1.HomeMatchupData{
				Id:        snapshotMatchupPublicIDs[m.ID],
				Title:     m.Title,
				AuthorId:  author.PublicID,
				BracketId: snapshotBracketID,
				CreatedAt: rfc3339(m.CreatedAt),
			})
		}
	} else if newThisWeekErr != nil && !isMissingRelation(newThisWeekErr, "home_new_this_week_snapshot") {
		log.Printf("home summary new_this_week snapshot: %v", newThisWeekErr)
	} else {
		recentCutoff := time.Now().Add(-7 * 24 * time.Hour)
		var recentMatchups []models.Matchup
		if err := sqlx.SelectContext(ctx, h.DB, &recentMatchups,
			"SELECT * FROM matchups WHERE created_at >= $1 AND bracket_id IS NULL ORDER BY created_at DESC LIMIT 6",
			recentCutoff); err == nil {
			recentAuthorIDs := make([]uint, len(recentMatchups))
			for i, m := range recentMatchups {
				recentAuthorIDs[i] = m.AuthorID
			}
			recentAuthorMap := map[uint]models.User{}
			if len(recentAuthorIDs) > 0 {
				query, args, err := sqlx.In("SELECT * FROM users WHERE id IN (?)", recentAuthorIDs)
				if err == nil {
					query = h.DB.Rebind(query)
					var authors []models.User
					if err := sqlx.SelectContext(ctx, h.DB, &authors, query, args...); err == nil {
						for _, a := range authors {
							a.ProcessAvatarPath()
							recentAuthorMap[a.ID] = a
						}
					}
				}
			}
			for i := range recentMatchups {
				if author, ok := recentAuthorMap[recentMatchups[i].AuthorID]; ok {
					recentMatchups[i].Author = author
				}
			}
			for _, m := range recentMatchups {
				allowed, _, err := canViewUserContent(h.DB, viewerID, hasViewer, &m.Author, m.Visibility, isAdmin)
				if err != nil || !allowed {
					continue
				}
				protoNewThisWeek = append(protoNewThisWeek, &homev1.HomeMatchupData{
					Id:       m.PublicID,
					Title:    m.Title,
					AuthorId: m.Author.PublicID,
					CreatedAt: rfc3339(m.CreatedAt),
				})
			}
		}
	}

	// Creators to follow
	creatorIDs := []uint{}
	followingMap := map[uint]bool{}
	followedByMap := map[uint]bool{}

	var creatorSnapshots []HomeCreatorSnapshot
	creatorsErr := sqlx.SelectContext(ctx, h.DB, &creatorSnapshots,
		"SELECT * FROM home_creators_snapshot ORDER BY rank ASC LIMIT 10")
	if creatorsErr == nil {
		for _, creator := range creatorSnapshots {
			if creator.ID == 0 {
				continue
			}
			if hasViewer && viewerID > 0 && creator.ID == viewerID {
				continue
			}
			creatorIDs = append(creatorIDs, creator.ID)
		}
	} else if !isMissingRelation(creatorsErr, "home_creators_snapshot") {
		log.Printf("home summary creators snapshot: %v", creatorsErr)
	} else {
		creatorQuery := "SELECT id, username, avatar_path, followers_count, following_count, is_private FROM users WHERE is_admin = false"
		creatorArgs := []interface{}{}
		if hasViewer && viewerID > 0 {
			creatorQuery += " AND id <> $1"
			creatorArgs = append(creatorArgs, viewerID)
		}
		creatorQuery += " ORDER BY followers_count DESC, id ASC LIMIT 5"
		var creatorUsers []models.User
		if err := sqlx.SelectContext(ctx, h.DB, &creatorUsers, creatorQuery, creatorArgs...); err == nil {
			for _, c := range creatorUsers {
				creatorIDs = append(creatorIDs, c.ID)
			}
			creatorSnapshots = make([]HomeCreatorSnapshot, 0, len(creatorUsers))
			for _, c := range creatorUsers {
				creatorSnapshots = append(creatorSnapshots, HomeCreatorSnapshot{
					ID: c.ID, Username: c.Username, AvatarPath: c.AvatarPath,
					FollowersCount: c.FollowersCount, FollowingCount: c.FollowingCount,
					IsPrivate: c.IsPrivate,
				})
			}
		}
	}

	if hasViewer && viewerID > 0 && len(creatorIDs) > 0 {
		var followingIDs []uint
		if err := sqlx.SelectContext(ctx, h.DB, &followingIDs,
			"SELECT followed_id FROM follows WHERE follower_id = $1", viewerID); err == nil {
			for _, id := range followingIDs {
				followingMap[id] = true
			}
		}
		var followerIDs []uint
		if err := sqlx.SelectContext(ctx, h.DB, &followerIDs,
			"SELECT follower_id FROM follows WHERE followed_id = $1", viewerID); err == nil {
			for _, id := range followerIDs {
				followedByMap[id] = true
			}
		}
	}

	creatorPublicIDs := loadUserPublicIDMap(h.DB, creatorIDs)
	var protoCreators []*homev1.HomeCreatorData
	for _, creator := range creatorSnapshots {
		if creator.ID == 0 {
			continue
		}
		if hasViewer && viewerID > 0 && creator.ID == viewerID {
			continue
		}
		viewerFollowing := followingMap[creator.ID]
		viewerFollowedBy := followedByMap[creator.ID]
		protoCreators = append(protoCreators, &homev1.HomeCreatorData{
			Id:               creatorPublicIDs[creator.ID],
			Username:         creator.Username,
			AvatarPath:       creator.AvatarPath,
			FollowersCount:   int32(creator.FollowersCount),
			FollowingCount:   int32(creator.FollowingCount),
			IsPrivate:        creator.IsPrivate,
			ViewerFollowing:  viewerFollowing,
			ViewerFollowedBy: viewerFollowedBy,
			Mutual:           viewerFollowing && viewerFollowedBy,
		})
	}

	var topCreator *homev1.HomeCreatorData
	if len(protoCreators) > 0 {
		topCreator = protoCreators[0]
	}

	summary := &homev1.HomeSummaryData{
		PopularMatchups:  protoPopularMatchups,
		PopularBrackets:  protoPopularBrackets,
		TotalEngagements: totalEngagements,
		VotesToday:       int32(votesToday),
		ActiveMatchups:   int32(activeMatchups),
		ActiveBrackets:   int32(activeBrackets),
		NewThisWeek:      protoNewThisWeek,
		TopCreator:       topCreator,
		CreatorsToFollow: protoCreators,
	}

	return connect.NewResponse(&homev1.GetHomeSummaryResponse{Summary: summary}), nil
}
