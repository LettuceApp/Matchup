package controllers

import (
	"Matchup/cache"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"github.com/gin-gonic/gin"
)

type HomeSummary struct {
	PopularMatchups  []PopularMatchupDTO `json:"popular_matchups"`
	PopularBrackets  []PopularBracketDTO `json:"popular_brackets"`
	TotalEngagements float64             `json:"total_engagements"`
	VotesToday       int64               `json:"votes_today"`
	ActiveMatchups   int64               `json:"active_matchups"`
	ActiveBrackets   int64               `json:"active_brackets"`
	NewThisWeek      []HomeMatchupDTO    `json:"new_this_week"`
	TopCreator       *HomeCreatorDTO     `json:"top_creator"`
	CreatorsToFollow []HomeCreatorDTO    `json:"creators_to_follow"`
}

type HomeSummarySnapshot struct {
	VotesToday     int64     `gorm:"column:votes_today"`
	ActiveMatchups int64     `gorm:"column:active_matchups"`
	ActiveBrackets int64     `gorm:"column:active_brackets"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

type HomeCreatorSnapshot struct {
	ID             uint   `gorm:"column:user_id"`
	Username       string `gorm:"column:username"`
	AvatarPath     string `gorm:"column:avatar_path"`
	FollowersCount int64  `gorm:"column:followers_count"`
	FollowingCount int64  `gorm:"column:following_count"`
	IsPrivate      bool   `gorm:"column:is_private"`
	Rank           int64  `gorm:"column:rank"`
}

type HomeMatchupSnapshot struct {
	ID         uint      `gorm:"column:matchup_id"`
	Title      string    `gorm:"column:title"`
	AuthorID   uint      `gorm:"column:author_id"`
	BracketID  *uint     `gorm:"column:bracket_id"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	Visibility string    `gorm:"column:visibility"`
	Rank       int64     `gorm:"column:rank"`
}

// GetHomeSummary godoc
// @Summary      Get home summary
// @Description  Get popular matchups, popular brackets, and total engagements
// @Tags         home
// @Produce      json
// @Param        user_id  query     string  false  "User ID"
// @Success      200      {object}  HomeSummaryEnvelope
// @Failure      500      {object}  ErrorResponse
// @Router       /home [get]
func (server *Server) GetHomeSummary(c *gin.Context) {
	userIDParam := c.Query("user_id")
	var userID uint
	if userIDParam != "" {
		if user, err := resolveUserByIdentifier(server.DB, userIDParam); err == nil {
			userID = user.ID
		}
	}

	viewerID, hasViewer := optionalViewerID(c)
	isAdmin := httpctx.IsAdminRequest(c)
	cacheKey := fmt.Sprintf("home_summary:%d:viewer:%d:admin:%t", userID, viewerID, isAdmin)
	ctx := context.Background()
	if cached, err := cache.Get(ctx, cacheKey); err == nil && cached != "" {
		c.Data(http.StatusOK, "application/json", []byte(cached))
		return
	}

	popularMatchupRows := []popularMatchupRow{}
	if err := server.DB.
		Table("popular_matchups_snapshot").
		Order("rank ASC").
		Limit(5).
		Scan(&popularMatchupRows).Error; err != nil {
		if !isMissingRelation(err, "popular_matchups_snapshot") {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": "failed to load popular matchups",
			})
			return
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
	matchupPublicIDs := loadMatchupPublicIDMap(server.DB, matchupIDs)
	matchupAuthorPublicIDs := loadUserPublicIDMap(server.DB, matchupAuthorIDs)
	matchupBracketPublicIDs := loadBracketPublicIDMap(server.DB, matchupBracketIDs)
	matchupBracketAuthorPublicIDs := loadUserPublicIDMap(server.DB, matchupBracketAuthorIDs)

	filteredMatchups := make([]PopularMatchupDTO, 0, len(popularMatchupRows))
	for i := range popularMatchupRows {
		var matchup models.Matchup
		if err := server.DB.Preload("Author").First(&matchup, popularMatchupRows[i].ID).Error; err != nil {
			continue
		}
		allowed, _, err := canViewUserContent(server.DB, viewerID, hasViewer, &matchup.Author, matchup.Visibility, isAdmin)
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
		filteredMatchups = append(filteredMatchups, PopularMatchupDTO{
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
		})
	}
	popularMatchups := filteredMatchups

	popularBracketRows := []popularBracketRow{}
	if err := server.DB.
		Table("popular_brackets_snapshot").
		Order("rank ASC").
		Limit(5).
		Scan(&popularBracketRows).Error; err != nil {
		if !isMissingRelation(err, "popular_brackets_snapshot") {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": "failed to load popular brackets",
			})
			return
		}
		log.Printf("popular brackets snapshot missing: %v", err)
	}

	bracketIDs := make([]uint, 0, len(popularBracketRows))
	bracketAuthorIDs := make([]uint, 0, len(popularBracketRows))
	for _, row := range popularBracketRows {
		bracketIDs = append(bracketIDs, row.ID)
		bracketAuthorIDs = append(bracketAuthorIDs, row.AuthorID)
	}
	bracketPublicIDs := loadBracketPublicIDMap(server.DB, bracketIDs)
	bracketAuthorPublicIDs := loadUserPublicIDMap(server.DB, bracketAuthorIDs)

	filteredBrackets := make([]PopularBracketDTO, 0, len(popularBracketRows))
	for i := range popularBracketRows {
		var bracket models.Bracket
		if err := server.DB.Preload("Author").First(&bracket, popularBracketRows[i].ID).Error; err != nil {
			continue
		}
		allowed, _, err := canViewUserContent(server.DB, viewerID, hasViewer, &bracket.Author, bracket.Visibility, isAdmin)
		if err != nil {
			log.Printf("home summary visibility error: %v", err)
			continue
		}
		if !allowed {
			continue
		}
		if bracket.Status != "active" {
			continue
		}
		likesCount := int64(0)
		if err := server.DB.Model(&models.BracketLike{}).
			Where("bracket_id = ?", bracket.ID).
			Count(&likesCount).Error; err != nil {
			likesCount = 0
		}
		commentsCount := int64(0)
		if err := server.DB.Model(&models.BracketComment{}).
			Where("bracket_id = ?", bracket.ID).
			Count(&commentsCount).Error; err != nil {
			commentsCount = 0
		}
		filteredBrackets = append(filteredBrackets, PopularBracketDTO{
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
		})
	}
	popularBrackets := filteredBrackets

	totalEngagements := 0.0
	if userID > 0 {
		total, err := server.calculateUserEngagement(userID)
		if err != nil {
			log.Printf("calculate user engagement: %v", err)
		} else {
			totalEngagements = total
		}
	}

	votesToday := int64(0)
	activeMatchups := int64(0)
	activeBrackets := int64(0)
	var summarySnapshot HomeSummarySnapshot
	summaryQuery := server.DB.
		Table("home_summary_snapshot").
		Limit(1).
		Scan(&summarySnapshot)
	if summaryQuery.Error == nil && summaryQuery.RowsAffected > 0 {
		votesToday = summarySnapshot.VotesToday
		activeMatchups = summarySnapshot.ActiveMatchups
		activeBrackets = summarySnapshot.ActiveBrackets
	} else if summaryQuery.Error != nil && !isMissingRelation(summaryQuery.Error, "home_summary_snapshot") {
		log.Printf("home summary snapshot error: %v", summaryQuery.Error)
	} else {
		voteCutoff := time.Now().Add(-24 * time.Hour)
		if err := server.DB.Model(&models.MatchupVote{}).
			Where("created_at >= ?", voteCutoff).
			Count(&votesToday).Error; err != nil {
			log.Printf("home summary votes_today: %v", err)
		}
		if err := server.DB.Model(&models.Matchup{}).
			Where("status IN ?", []string{matchupStatusActive, matchupStatusPublished}).
			Count(&activeMatchups).Error; err != nil {
			log.Printf("home summary active_matchups: %v", err)
		}
		if err := server.DB.Model(&models.Bracket{}).
			Where("status = ?", "active").
			Count(&activeBrackets).Error; err != nil {
			log.Printf("home summary active_brackets: %v", err)
		}
	}

	newThisWeek := make([]HomeMatchupDTO, 0)
	var newThisWeekSnapshots []HomeMatchupSnapshot
	newThisWeekQuery := server.DB.
		Table("home_new_this_week_snapshot").
		Order("rank ASC").
		Limit(6).
		Scan(&newThisWeekSnapshots)
	if newThisWeekQuery.Error == nil {
		if len(newThisWeekSnapshots) > 0 {
			authorIDs := make([]uint, 0, len(newThisWeekSnapshots))
			authorSeen := map[uint]struct{}{}
			for _, matchup := range newThisWeekSnapshots {
				if matchup.AuthorID == 0 {
					continue
				}
				if _, seen := authorSeen[matchup.AuthorID]; seen {
					continue
				}
				authorSeen[matchup.AuthorID] = struct{}{}
				authorIDs = append(authorIDs, matchup.AuthorID)
			}

			authorMap := map[uint]models.User{}
			if len(authorIDs) > 0 {
				var authors []models.User
				if err := server.DB.Where("id IN ?", authorIDs).Find(&authors).Error; err != nil {
					log.Printf("home summary new_this_week authors: %v", err)
				} else {
					for _, author := range authors {
						authorMap[author.ID] = author
					}
				}
			}

			matchupIDs := make([]uint, 0, len(newThisWeekSnapshots))
			bracketIDs := make([]uint, 0, len(newThisWeekSnapshots))
			for _, matchup := range newThisWeekSnapshots {
				if matchup.ID != 0 {
					matchupIDs = append(matchupIDs, matchup.ID)
				}
				if matchup.BracketID != nil {
					bracketIDs = append(bracketIDs, *matchup.BracketID)
				}
			}
			matchupPublicIDs := loadMatchupPublicIDMap(server.DB, matchupIDs)
			bracketPublicIDs := loadBracketPublicIDMap(server.DB, bracketIDs)

			for _, matchup := range newThisWeekSnapshots {
				author, ok := authorMap[matchup.AuthorID]
				if !ok {
					continue
				}
				visibility := matchup.Visibility
				if visibility == "" {
					visibility = "public"
				}
				allowed, _, err := canViewUserContent(
					server.DB,
					viewerID,
					hasViewer,
					&author,
					visibility,
					isAdmin,
				)
				if err != nil {
					log.Printf("home summary visibility error: %v", err)
					continue
				}
				if !allowed {
					continue
				}
				var bracketID *string
				if matchup.BracketID != nil {
					if id := bracketPublicIDs[*matchup.BracketID]; id != "" {
						bracketID = &id
					}
				}
				newThisWeek = append(newThisWeek, HomeMatchupDTO{
					ID:        matchupPublicIDs[matchup.ID],
					Title:     matchup.Title,
					AuthorID:  author.PublicID,
					BracketID: bracketID,
					CreatedAt: matchup.CreatedAt,
				})
			}
		}
	} else if !isMissingRelation(newThisWeekQuery.Error, "home_new_this_week_snapshot") {
		log.Printf("home summary new_this_week snapshot: %v", newThisWeekQuery.Error)
	} else {
		recentCutoff := time.Now().Add(-7 * 24 * time.Hour)
		var recentMatchups []models.Matchup
		if err := server.DB.Preload("Author").
			Where("created_at >= ?", recentCutoff).
			Where("bracket_id IS NULL").
			Order("created_at DESC").
			Limit(6).
			Find(&recentMatchups).Error; err != nil {
			log.Printf("home summary new_this_week: %v", err)
		} else {
			for _, matchup := range recentMatchups {
				allowed, _, err := canViewUserContent(server.DB, viewerID, hasViewer, &matchup.Author, matchup.Visibility, isAdmin)
				if err != nil {
					log.Printf("home summary visibility error: %v", err)
					continue
				}
				if !allowed {
					continue
				}
				newThisWeek = append(newThisWeek, HomeMatchupDTO{
					ID:        matchup.PublicID,
					Title:     matchup.Title,
					AuthorID:  matchup.Author.PublicID,
					BracketID: nil,
					CreatedAt: matchup.CreatedAt,
				})
			}
		}
	}

	creatorIDs := []uint{}
	followingMap := map[uint]bool{}
	followedByMap := map[uint]bool{}
	creatorsToFollow := []HomeCreatorDTO{}

	var creatorSnapshots []HomeCreatorSnapshot
	creatorSnapshotQuery := server.DB.
		Table("home_creators_snapshot").
		Order("rank ASC").
		Limit(10).
		Scan(&creatorSnapshots)
	if creatorSnapshotQuery.Error == nil {
		for _, creator := range creatorSnapshots {
			if creator.ID == 0 {
				continue
			}
			if hasViewer && viewerID > 0 && creator.ID == viewerID {
				continue
			}
			creatorIDs = append(creatorIDs, creator.ID)
		}
	} else if !isMissingRelation(creatorSnapshotQuery.Error, "home_creators_snapshot") {
		log.Printf("home summary creators snapshot: %v", creatorSnapshotQuery.Error)
	} else {
		creatorQuery := server.DB.
			Select("id", "username", "avatar_path", "followers_count", "following_count", "is_private").
			Where("is_admin = ?", false)
		if hasViewer && viewerID > 0 {
			creatorQuery = creatorQuery.Where("id <> ?", viewerID)
		}

		var creatorUsers []models.User
		if err := creatorQuery.
			Order("followers_count DESC, id ASC").
			Limit(5).
			Find(&creatorUsers).Error; err != nil {
			log.Printf("home summary creators_to_follow: %v", err)
		} else {
			for _, creator := range creatorUsers {
				creatorIDs = append(creatorIDs, creator.ID)
			}
		}
		creatorSnapshots = make([]HomeCreatorSnapshot, 0, len(creatorUsers))
		for _, creator := range creatorUsers {
			creatorSnapshots = append(creatorSnapshots, HomeCreatorSnapshot{
				ID:             creator.ID,
				Username:       creator.Username,
				AvatarPath:     creator.AvatarPath,
				FollowersCount: creator.FollowersCount,
				FollowingCount: creator.FollowingCount,
				IsPrivate:      creator.IsPrivate,
				Rank:           0,
			})
		}
	}

	if hasViewer && viewerID > 0 && len(creatorIDs) > 0 {
		var followingIDs []uint
		if err := server.DB.Model(&models.Follow{}).
			Where("follower_id = ?", viewerID).
			Pluck("followed_id", &followingIDs).Error; err == nil {
			for _, id := range followingIDs {
				followingMap[id] = true
			}
		}

		var followerIDs []uint
		if err := server.DB.Model(&models.Follow{}).
			Where("followed_id = ?", viewerID).
			Pluck("follower_id", &followerIDs).Error; err == nil {
			for _, id := range followerIDs {
				followedByMap[id] = true
			}
		}
	}

	creatorPublicIDs := loadUserPublicIDMap(server.DB, creatorIDs)

	for _, creator := range creatorSnapshots {
		if creator.ID == 0 {
			continue
		}
		if hasViewer && viewerID > 0 && creator.ID == viewerID {
			continue
		}
		viewerFollowing := followingMap[creator.ID]
		viewerFollowedBy := followedByMap[creator.ID]
		creatorsToFollow = append(creatorsToFollow, HomeCreatorDTO{
			ID:               creatorPublicIDs[creator.ID],
			Username:         creator.Username,
			AvatarPath:       creator.AvatarPath,
			FollowersCount:   int(creator.FollowersCount),
			FollowingCount:   int(creator.FollowingCount),
			IsPrivate:        creator.IsPrivate,
			ViewerFollowing:  viewerFollowing,
			ViewerFollowedBy: viewerFollowedBy,
			Mutual:           viewerFollowing && viewerFollowedBy,
		})
	}

	var topCreator *HomeCreatorDTO
	if len(creatorsToFollow) > 0 {
		top := creatorsToFollow[0]
		topCreator = &top
	}

	response := HomeSummary{
		PopularMatchups:  popularMatchups,
		PopularBrackets:  popularBrackets,
		TotalEngagements: totalEngagements,
		VotesToday:       votesToday,
		ActiveMatchups:   activeMatchups,
		ActiveBrackets:   activeBrackets,
		NewThisWeek:      newThisWeek,
		TopCreator:       topCreator,
		CreatorsToFollow: creatorsToFollow,
	}

	if jsonBytes, err := json.Marshal(gin.H{
		"status":   "success",
		"response": response,
	}); err == nil {
		_ = cache.Set(ctx, cacheKey, jsonBytes, 60*time.Second)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"response": response,
	})
}

func isMissingRelation(err error, relation string) bool {
	if err == nil {
		return false
	}
	errLower := strings.ToLower(err.Error())
	return strings.Contains(errLower, "does not exist") && strings.Contains(errLower, strings.ToLower(relation))
}

func (server *Server) calculateUserEngagement(userID uint) (float64, error) {
	var totalVotes int64
	if err := server.DB.Raw(`
		SELECT COALESCE(SUM(mi.votes), 0)
		FROM matchup_items mi
		JOIN matchups m ON m.id = mi.matchup_id
		WHERE m.author_id = ?
	`, userID).Scan(&totalVotes).Error; err != nil {
		return 0, err
	}

	var selfVotes int64
	if err := server.DB.Raw(`
		SELECT COALESCE(COUNT(*), 0)
		FROM matchup_votes mv
		JOIN matchups m ON m.public_id = mv.matchup_public_id
		WHERE m.author_id = ? AND mv.user_id = ?
	`, userID, userID).Scan(&selfVotes).Error; err != nil {
		return 0, err
	}

	var likes int64
	if err := server.DB.Raw(`
		SELECT COALESCE(COUNT(*), 0)
		FROM likes l
		JOIN matchups m ON m.id = l.matchup_id
		WHERE m.author_id = ? AND l.user_id <> ?
	`, userID, userID).Scan(&likes).Error; err != nil {
		return 0, err
	}

	var comments int64
	if err := server.DB.Raw(`
		SELECT COALESCE(COUNT(*), 0)
		FROM comments c
		JOIN matchups m ON m.id = c.matchup_id
		WHERE m.author_id = ? AND c.user_id <> ?
	`, userID, userID).Scan(&comments).Error; err != nil {
		return 0, err
	}

	var bracketLikes int64
	if err := server.DB.Raw(`
		SELECT COALESCE(COUNT(*), 0)
		FROM bracket_likes bl
		JOIN brackets b ON b.id = bl.bracket_id
		WHERE b.author_id = ? AND bl.user_id <> ?
	`, userID, userID).Scan(&bracketLikes).Error; err != nil {
		return 0, err
	}

	var bracketComments int64
	if err := server.DB.Raw(`
		SELECT COALESCE(COUNT(*), 0)
		FROM bracket_comments bc
		JOIN brackets b ON b.id = bc.bracket_id
		WHERE b.author_id = ? AND bc.user_id <> ?
	`, userID, userID).Scan(&bracketComments).Error; err != nil {
		return 0, err
	}

	votes := totalVotes - selfVotes
	if votes < 0 {
		votes = 0
	}

	return float64(votes)*2 +
		float64(likes)*3 +
		float64(comments)*.5 +
		float64(bracketLikes)*3 +
		float64(bracketComments)*.5, nil
}
