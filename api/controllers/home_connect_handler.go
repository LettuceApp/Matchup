package controllers

import (
	"context"
	"log"

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
	DB     *sqlx.DB
	ReadDB *sqlx.DB
}

func (h *HomeHandler) GetHomeSummary(ctx context.Context, req *connect.Request[homev1.GetHomeSummaryRequest]) (*connect.Response[homev1.GetHomeSummaryResponse], error) {
	db := dbForRead(ctx, h.DB, h.ReadDB)
	viewerID, hasViewer := optionalViewerFromCtx(ctx)
	isAdmin := httpctx.IsAdminRequest(ctx)

	// Popular matchups — read from materialized view (migration 007).
	popularMatchupRows := []popularMatchupRow{}
	if err := sqlx.SelectContext(ctx, db, &popularMatchupRows,
		"SELECT * FROM popular_matchups_snapshot ORDER BY rank ASC LIMIT 5"); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
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
	matchupPublicIDs := loadMatchupPublicIDMap(db, matchupIDs)
	matchupAuthorPublicIDs := loadUserPublicIDMap(db, matchupAuthorIDs)
	matchupBracketPublicIDs := loadBracketPublicIDMap(db, matchupBracketIDs)
	matchupBracketAuthorPublicIDs := loadUserPublicIDMap(db, matchupBracketAuthorIDs)

	homeMatchupsMap := getMatchupsByIDs(db, matchupIDs)
	homeMatchupAuthorsMap := getUsersByIDs(db, matchupAuthorIDs)

	var protoPopularMatchups []*matchupv1.PopularMatchupData
	for i := range popularMatchupRows {
		matchup, ok := homeMatchupsMap[popularMatchupRows[i].ID]
		if !ok {
			continue
		}
		if matchup.Status != "active" && matchup.Status != "published" {
			continue
		}
		author, ok := homeMatchupAuthorsMap[matchup.AuthorID]
		if !ok {
			continue
		}
		matchup.Author = *author
		allowed, _, err := canViewUserContent(db, viewerID, hasViewer, &matchup.Author, matchup.Visibility, isAdmin)
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
			AuthorUsername:  matchup.Author.Username,
			BracketID:       bracketID,
			BracketAuthorID: bracketAuthorID,
			Round:           popularMatchupRows[i].Round,
			CurrentRound:    popularMatchupRows[i].CurrentRound,
			Votes:           popularMatchupRows[i].Votes,
			Likes:           popularMatchupRows[i].Likes,
			Comments:        popularMatchupRows[i].Comments,
			EngagementScore: popularMatchupRows[i].EngagementScore,
			Rank:            popularMatchupRows[i].Rank,
			CreatedAt:       rfc3339(matchup.CreatedAt),
		}
		protoPopularMatchups = append(protoPopularMatchups, popularMatchupToProto(dto))
	}

	// Trending matchups — hourly engagement from materialized view (migration 010).
	trendingMatchupRows := []popularMatchupRow{}
	if err := sqlx.SelectContext(ctx, db, &trendingMatchupRows,
		"SELECT * FROM trending_matchups_snapshot ORDER BY rank ASC LIMIT 9"); err != nil {
		// Non-fatal — view may not exist yet on older DBs.
		log.Printf("home summary trending_matchups_snapshot: %v", err)
	}

	var protoTrendingMatchups []*matchupv1.PopularMatchupData
	if len(trendingMatchupRows) > 0 {
		trendingIDs := make([]uint, 0, len(trendingMatchupRows))
		trendingAuthorIDs := make([]uint, 0, len(trendingMatchupRows))
		trendingBracketIDs := make([]uint, 0, len(trendingMatchupRows))
		trendingBracketAuthorIDs := make([]uint, 0, len(trendingMatchupRows))
		for _, row := range trendingMatchupRows {
			trendingIDs = append(trendingIDs, row.ID)
			trendingAuthorIDs = append(trendingAuthorIDs, row.AuthorID)
			if row.BracketID != nil {
				trendingBracketIDs = append(trendingBracketIDs, *row.BracketID)
			}
			if row.BracketAuthorID != nil {
				trendingBracketAuthorIDs = append(trendingBracketAuthorIDs, *row.BracketAuthorID)
			}
		}
		trendingPublicIDs := loadMatchupPublicIDMap(db, trendingIDs)
		trendingAuthorPublicIDs := loadUserPublicIDMap(db, trendingAuthorIDs)
		trendingBracketPublicIDs := loadBracketPublicIDMap(db, trendingBracketIDs)
		trendingBracketAuthorPublicIDs := loadUserPublicIDMap(db, trendingBracketAuthorIDs)

		trendingMatchupsMap := getMatchupsByIDs(db, trendingIDs)
		trendingAuthorsMap := getUsersByIDs(db, trendingAuthorIDs)

		for i := range trendingMatchupRows {
			matchup, ok := trendingMatchupsMap[trendingMatchupRows[i].ID]
			if !ok {
				continue
			}
			if matchup.Status != "active" && matchup.Status != "published" {
				continue
			}
			author, ok := trendingAuthorsMap[matchup.AuthorID]
			if !ok {
				continue
			}
			matchup.Author = *author
			allowed, _, err := canViewUserContent(db, viewerID, hasViewer, &matchup.Author, matchup.Visibility, isAdmin)
			if err != nil {
				log.Printf("home summary trending visibility error: %v", err)
				continue
			}
			if !allowed {
				continue
			}
			var tBracketID *string
			if trendingMatchupRows[i].BracketID != nil {
				if id := trendingBracketPublicIDs[*trendingMatchupRows[i].BracketID]; id != "" {
					tBracketID = &id
				}
			}
			var tBracketAuthorID *string
			if trendingMatchupRows[i].BracketAuthorID != nil {
				if id := trendingBracketAuthorPublicIDs[*trendingMatchupRows[i].BracketAuthorID]; id != "" {
					tBracketAuthorID = &id
				}
			}
			dto := PopularMatchupDTO{
				ID:              trendingPublicIDs[trendingMatchupRows[i].ID],
				Title:           trendingMatchupRows[i].Title,
				AuthorID:        trendingAuthorPublicIDs[trendingMatchupRows[i].AuthorID],
				AuthorUsername:  matchup.Author.Username,
				BracketID:       tBracketID,
				BracketAuthorID: tBracketAuthorID,
				Round:           trendingMatchupRows[i].Round,
				CurrentRound:    trendingMatchupRows[i].CurrentRound,
				Votes:           trendingMatchupRows[i].Votes,
				Likes:           trendingMatchupRows[i].Likes,
				Comments:        trendingMatchupRows[i].Comments,
				EngagementScore: trendingMatchupRows[i].EngagementScore,
				Rank:            trendingMatchupRows[i].Rank,
				CreatedAt:       rfc3339(matchup.CreatedAt),
			}
			protoTrendingMatchups = append(protoTrendingMatchups, popularMatchupToProto(dto))
		}
	}

	// Most-played matchups — hourly votes from materialized view (migration 011).
	mostPlayedRows := []popularMatchupRow{}
	if err := sqlx.SelectContext(ctx, db, &mostPlayedRows,
		"SELECT * FROM most_played_snapshot ORDER BY rank ASC LIMIT 9"); err != nil {
		// Non-fatal — view may not exist yet on older DBs.
		log.Printf("home summary most_played_snapshot: %v", err)
	}

	var protoMostPlayedMatchups []*matchupv1.PopularMatchupData
	if len(mostPlayedRows) > 0 {
		mpIDs := make([]uint, 0, len(mostPlayedRows))
		mpAuthorIDs := make([]uint, 0, len(mostPlayedRows))
		mpBracketIDs := make([]uint, 0, len(mostPlayedRows))
		mpBracketAuthorIDs := make([]uint, 0, len(mostPlayedRows))
		for _, row := range mostPlayedRows {
			mpIDs = append(mpIDs, row.ID)
			mpAuthorIDs = append(mpAuthorIDs, row.AuthorID)
			if row.BracketID != nil {
				mpBracketIDs = append(mpBracketIDs, *row.BracketID)
			}
			if row.BracketAuthorID != nil {
				mpBracketAuthorIDs = append(mpBracketAuthorIDs, *row.BracketAuthorID)
			}
		}
		mpPublicIDs := loadMatchupPublicIDMap(db, mpIDs)
		mpAuthorPublicIDs := loadUserPublicIDMap(db, mpAuthorIDs)
		mpBracketPublicIDs := loadBracketPublicIDMap(db, mpBracketIDs)
		mpBracketAuthorPublicIDs := loadUserPublicIDMap(db, mpBracketAuthorIDs)

		mpMatchupsMap := getMatchupsByIDs(db, mpIDs)
		mpAuthorsMap := getUsersByIDs(db, mpAuthorIDs)

		for i := range mostPlayedRows {
			matchup, ok := mpMatchupsMap[mostPlayedRows[i].ID]
			if !ok {
				continue
			}
			if matchup.Status != "active" && matchup.Status != "published" {
				continue
			}
			author, ok := mpAuthorsMap[matchup.AuthorID]
			if !ok {
				continue
			}
			matchup.Author = *author
			allowed, _, err := canViewUserContent(db, viewerID, hasViewer, &matchup.Author, matchup.Visibility, isAdmin)
			if err != nil {
				log.Printf("home summary most-played visibility error: %v", err)
				continue
			}
			if !allowed {
				continue
			}
			var mpBracketID *string
			if mostPlayedRows[i].BracketID != nil {
				if id := mpBracketPublicIDs[*mostPlayedRows[i].BracketID]; id != "" {
					mpBracketID = &id
				}
			}
			var mpBracketAuthorID *string
			if mostPlayedRows[i].BracketAuthorID != nil {
				if id := mpBracketAuthorPublicIDs[*mostPlayedRows[i].BracketAuthorID]; id != "" {
					mpBracketAuthorID = &id
				}
			}
			dto := PopularMatchupDTO{
				ID:              mpPublicIDs[mostPlayedRows[i].ID],
				Title:           mostPlayedRows[i].Title,
				AuthorID:        mpAuthorPublicIDs[mostPlayedRows[i].AuthorID],
				AuthorUsername:  matchup.Author.Username,
				BracketID:       mpBracketID,
				BracketAuthorID: mpBracketAuthorID,
				Round:           mostPlayedRows[i].Round,
				CurrentRound:    mostPlayedRows[i].CurrentRound,
				Votes:           mostPlayedRows[i].Votes,
				Likes:           mostPlayedRows[i].Likes,
				Comments:        mostPlayedRows[i].Comments,
				EngagementScore: mostPlayedRows[i].EngagementScore,
				Rank:            mostPlayedRows[i].Rank,
				CreatedAt:       rfc3339(matchup.CreatedAt),
			}
			protoMostPlayedMatchups = append(protoMostPlayedMatchups, popularMatchupToProto(dto))
		}
	}

	// Popular brackets — read from materialized view (migration 007).
	// Hidden from anon viewers per the members-only-brackets rule:
	// anon users can browse + vote on matchups but can't see brackets
	// at all. The materialized view query is skipped entirely for
	// anon to save the round-trip; the response just carries an empty
	// list. Authed users see the standard popular-brackets feed.
	popularBracketRows := []popularBracketRow{}
	if hasViewer {
		if err := sqlx.SelectContext(ctx, db, &popularBracketRows,
			"SELECT * FROM popular_brackets_snapshot ORDER BY rank ASC LIMIT 12"); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	var protoPopularBrackets []*bracketv1.PopularBracketData
	if len(popularBracketRows) > 0 {
		bracketIDs := make([]uint, 0, len(popularBracketRows))
		bracketAuthorIDs := make([]uint, 0, len(popularBracketRows))
		for _, row := range popularBracketRows {
			bracketIDs = append(bracketIDs, row.ID)
			bracketAuthorIDs = append(bracketAuthorIDs, row.AuthorID)
		}
		bracketPublicIDs := loadBracketPublicIDMap(db, bracketIDs)
		bracketAuthorPublicIDs := loadUserPublicIDMap(db, bracketAuthorIDs)

		snapshotBracketsMap := getBracketsByIDs(db, bracketIDs)
		snapshotAuthorsMap := getUsersByIDs(db, bracketAuthorIDs)

		for i := range popularBracketRows {
			bracket, ok := snapshotBracketsMap[popularBracketRows[i].ID]
			if !ok {
				continue
			}
			author, ok := snapshotAuthorsMap[bracket.AuthorID]
			if !ok {
				continue
			}
			bracket.Author = *author
			allowed, _, err := canViewUserContent(db, viewerID, hasViewer, &bracket.Author, bracket.Visibility, isAdmin)
			if err != nil {
				log.Printf("home summary visibility error: %v", err)
				continue
			}
			if !allowed || bracket.Status != "active" {
				continue
			}
			dto := PopularBracketDTO{
				ID:              bracketPublicIDs[popularBracketRows[i].ID],
				Title:           popularBracketRows[i].Title,
				AuthorID:        bracketAuthorPublicIDs[popularBracketRows[i].AuthorID],
				AuthorUsername:  bracket.Author.Username,
				CurrentRound:    popularBracketRows[i].CurrentRound,
				Size:            bracket.Size,
				Votes:           0,
				Likes:           int64(bracket.LikesCount),
				Comments:        int64(bracket.CommentsCount),
				EngagementScore: popularBracketRows[i].EngagementScore,
				Rank:            popularBracketRows[i].Rank,
				CreatedAt:       rfc3339(bracket.CreatedAt),
			}
			protoPopularBrackets = append(protoPopularBrackets, popularBracketToProto(dto))
		}
	}

	// Stats snapshot — single-row materialized view (migration 007).
	votesToday := int64(0)
	activeMatchups := int64(0)
	activeBrackets := int64(0)
	var summarySnapshot HomeSummarySnapshot
	if err := sqlx.GetContext(ctx, db, &summarySnapshot,
		"SELECT votes_today, active_matchups, active_brackets FROM home_summary_snapshot LIMIT 1"); err == nil {
		votesToday = summarySnapshot.VotesToday
		activeMatchups = summarySnapshot.ActiveMatchups
		activeBrackets = summarySnapshot.ActiveBrackets
	} else {
		log.Printf("home summary snapshot error: %v", err)
	}

	// Total engagements for viewer
	totalEngagements := 0.0
	if hasViewer && viewerID > 0 {
		// calculateUserEngagement is read-only — safe to point at ReadDB.
		s := &Server{DB: db}
		if total, err := s.calculateUserEngagement(viewerID); err == nil {
			totalEngagements = total
		} else {
			log.Printf("calculate user engagement: %v", err)
		}
	}

	// New this week — read from materialized view (migration 007).
	var protoNewThisWeek []*homev1.HomeMatchupData
	var newThisWeekSnapshots []HomeMatchupSnapshot
	if err := sqlx.SelectContext(ctx, db, &newThisWeekSnapshots,
		"SELECT * FROM home_new_this_week_snapshot ORDER BY rank ASC LIMIT 6"); err != nil {
		log.Printf("home summary new_this_week snapshot: %v", err)
	} else if len(newThisWeekSnapshots) > 0 {
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
				query = db.Rebind(query)
				var authors []models.User
				if err := sqlx.SelectContext(ctx, db, &authors, query, args...); err == nil {
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
		snapshotMatchupPublicIDs := loadMatchupPublicIDMap(db, snapshotMatchupIDs)
		snapshotBracketPublicIDs := loadBracketPublicIDMap(db, snapshotBracketIDs)

		for _, m := range newThisWeekSnapshots {
			author, ok := authorMap[m.AuthorID]
			if !ok {
				continue
			}
			visibility := m.Visibility
			if visibility == "" {
				visibility = "public"
			}
			allowed, _, err := canViewUserContent(db, viewerID, hasViewer, &author, visibility, isAdmin)
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
	}

	// Creators to follow — read from materialized view (migration 007).
	creatorIDs := []uint{}
	followingMap := map[uint]bool{}
	followedByMap := map[uint]bool{}

	var creatorSnapshots []HomeCreatorSnapshot
	if err := sqlx.SelectContext(ctx, db, &creatorSnapshots,
		"SELECT * FROM home_creators_snapshot ORDER BY rank ASC LIMIT 10"); err != nil {
		log.Printf("home summary creators snapshot: %v", err)
	} else {
		for _, creator := range creatorSnapshots {
			if creator.ID == 0 {
				continue
			}
			if hasViewer && viewerID > 0 && creator.ID == viewerID {
				continue
			}
			creatorIDs = append(creatorIDs, creator.ID)
		}
	}

	if hasViewer && viewerID > 0 && len(creatorIDs) > 0 {
		var followingIDs []uint
		if err := sqlx.SelectContext(ctx, db, &followingIDs,
			"SELECT followed_id FROM follows WHERE follower_id = $1", viewerID); err == nil {
			for _, id := range followingIDs {
				followingMap[id] = true
			}
		}
		var followerIDs []uint
		if err := sqlx.SelectContext(ctx, db, &followerIDs,
			"SELECT follower_id FROM follows WHERE followed_id = $1", viewerID); err == nil {
			for _, id := range followerIDs {
				followedByMap[id] = true
			}
		}
	}

	creatorPublicIDs := loadUserPublicIDMap(db, creatorIDs)
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
		TrendingMatchups:    protoTrendingMatchups,
		MostPlayedMatchups: protoMostPlayedMatchups,
	}

	return connect.NewResponse(&homev1.GetHomeSummaryResponse{Summary: summary}), nil
}
