package controllers

import (
	"strconv"
	"time"

	"Matchup/models"

	"gorm.io/gorm"
)

func uintToString(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

func timePtrOrNil(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	copy := *t
	return &copy
}

func userToDTO(user *models.User) UserDTO {
	return UserDTO{
		ID:             user.PublicID,
		Username:       user.Username,
		Email:          user.Email,
		AvatarPath:     user.AvatarPath,
		IsAdmin:        user.IsAdmin,
		IsPrivate:      user.IsPrivate,
		FollowersCount: int(user.FollowersCount),
		FollowingCount: int(user.FollowingCount),
		CreatedAt:      user.CreatedAt,
		UpdatedAt:      user.UpdatedAt,
	}
}

func matchupItemToDTO(item models.MatchupItem) MatchupItemDTO {
	return MatchupItemDTO{
		ID:    item.PublicID,
		Item:  item.Item,
		Votes: item.Votes,
	}
}

func resolveUserPublicID(db *gorm.DB, user *models.User, userID uint) string {
	if user != nil && user.PublicID != "" {
		return user.PublicID
	}
	if db == nil || userID == 0 {
		return ""
	}
	var record struct {
		PublicID string
	}
	if err := db.Model(&models.User{}).Select("public_id").Where("id = ?", userID).Take(&record).Error; err != nil {
		return ""
	}
	return record.PublicID
}

func resolveBracketPublicID(db *gorm.DB, bracketID *uint) *string {
	if db == nil || bracketID == nil || *bracketID == 0 {
		return nil
	}
	var record struct {
		PublicID string
	}
	if err := db.Model(&models.Bracket{}).Select("public_id").Where("id = ?", *bracketID).Take(&record).Error; err != nil {
		return nil
	}
	if record.PublicID == "" {
		return nil
	}
	return &record.PublicID
}

func resolveMatchupItemPublicID(db *gorm.DB, itemID uint) string {
	if db == nil || itemID == 0 {
		return ""
	}
	var record struct {
		PublicID string
	}
	if err := db.Model(&models.MatchupItem{}).Select("public_id").Where("id = ?", itemID).Take(&record).Error; err != nil {
		return ""
	}
	return record.PublicID
}

func commentToDTO(db *gorm.DB, comment models.Comment) CommentDTO {
	userPublicID := resolveUserPublicID(db, &comment.Author, comment.UserID)
	return CommentDTO{
		ID:        comment.PublicID,
		UserID:    userPublicID,
		Username:  comment.Author.Username,
		Body:      comment.Body,
		CreatedAt: comment.CreatedAt,
		UpdatedAt: comment.UpdatedAt,
	}
}

func bracketCommentToDTO(db *gorm.DB, comment models.BracketComment) CommentDTO {
	userPublicID := resolveUserPublicID(db, &comment.Author, comment.UserID)
	return CommentDTO{
		ID:        comment.PublicID,
		UserID:    userPublicID,
		Username:  comment.Author.Username,
		Body:      comment.Body,
		CreatedAt: comment.CreatedAt,
		UpdatedAt: comment.UpdatedAt,
	}
}

func matchupToDTO(db *gorm.DB, matchup *models.Matchup, comments []models.Comment, likesCount int64) MatchupDTO {
	items := make([]MatchupItemDTO, len(matchup.Items))
	for i, item := range matchup.Items {
		items[i] = matchupItemToDTO(item)
	}

	commentResponses := make([]CommentDTO, len(comments))
	for i, comment := range comments {
		commentResponses[i] = commentToDTO(db, comment)
	}

	bracketPublicID := resolveBracketPublicID(db, matchup.BracketID)

	var winnerItemID *string
	if matchup.WinnerItemID != nil {
		for _, item := range matchup.Items {
			if item.ID == *matchup.WinnerItemID {
				if item.PublicID != "" {
					id := item.PublicID
					winnerItemID = &id
				}
				break
			}
		}
		if winnerItemID == nil {
			if resolved := resolveMatchupItemPublicID(db, *matchup.WinnerItemID); resolved != "" {
				id := resolved
				winnerItemID = &id
			}
		}
	}

	authorID := resolveUserPublicID(db, &matchup.Author, matchup.AuthorID)

	return MatchupDTO{
		ID:              matchup.PublicID,
		Title:           matchup.Title,
		Content:         matchup.Content,
		AuthorID:        authorID,
		Author:          UserSummaryDTO{ID: authorID, Username: matchup.Author.Username},
		Items:           items,
		Comments:        commentResponses,
		LikesCount:      likesCount,
		CreatedAt:       matchup.CreatedAt,
		UpdatedAt:       matchup.UpdatedAt,
		BracketID:       bracketPublicID,
		Round:           matchup.Round,
		Seed:            matchup.Seed,
		Status:          matchup.Status,
		EndMode:         matchup.EndMode,
		DurationSeconds: matchup.DurationSeconds,
		StartTime:       timePtrOrNil(matchup.StartTime),
		EndTime:         timePtrOrNil(matchup.EndTime),
		WinnerItemID:    winnerItemID,
	}
}

func adminMatchupToDTO(db *gorm.DB, matchup *models.Matchup, likesCount int64) AdminMatchupDTO {
	items := make([]MatchupItemDTO, len(matchup.Items))
	for i, item := range matchup.Items {
		items[i] = matchupItemToDTO(item)
	}

	authorUsername := ""
	authorID := resolveUserPublicID(db, &matchup.Author, matchup.AuthorID)
	if matchup.Author.ID != 0 {
		authorUsername = matchup.Author.Username
	}

	return AdminMatchupDTO{
		ID:             matchup.PublicID,
		Title:          matchup.Title,
		Content:        matchup.Content,
		AuthorID:       authorID,
		AuthorUsername: authorUsername,
		Items:          items,
		LikesCount:     likesCount,
		CreatedAt:      matchup.CreatedAt,
		UpdatedAt:      matchup.UpdatedAt,
	}
}

func bracketToDTO(db *gorm.DB, bracket *models.Bracket) BracketDTO {
	authorID := resolveUserPublicID(db, &bracket.Author, bracket.AuthorID)
	return BracketDTO{
		ID:                   bracket.PublicID,
		Title:                bracket.Title,
		Description:          bracket.Description,
		AuthorID:             authorID,
		Size:                 bracket.Size,
		Status:               bracket.Status,
		CurrentRound:         bracket.CurrentRound,
		AdvanceMode:          bracket.AdvanceMode,
		RoundDurationSeconds: bracket.RoundDurationSeconds,
		RoundStartedAt:       timePtrOrNil(bracket.RoundStartedAt),
		RoundEndsAt:          timePtrOrNil(bracket.RoundEndsAt),
		LikesCount:           bracket.LikesCount,
	}
}

func likeToDTO(like models.Like, userPublicID string, matchupPublicID string) LikeDTO {
	return LikeDTO{
		ID:        like.PublicID,
		UserID:    userPublicID,
		MatchupID: matchupPublicID,
		CreatedAt: like.CreatedAt,
	}
}

func bracketLikeToDTO(like models.BracketLike, userPublicID string, bracketPublicID string) BracketLikeDTO {
	return BracketLikeDTO{
		ID:        like.PublicID,
		UserID:    userPublicID,
		BracketID: bracketPublicID,
		CreatedAt: like.CreatedAt,
	}
}

func matchupVoteToDTO(vote models.MatchupVote, userPublicID *string) MatchupVoteDTO {
	return MatchupVoteDTO{
		ID:            vote.PublicID,
		UserID:        userPublicID,
		MatchupID:     vote.MatchupPublicID,
		MatchupItemID: vote.MatchupItemPublicID,
		CreatedAt:     vote.CreatedAt,
	}
}

func adminBracketToDTO(bracket *models.Bracket, likesCount int64, authorUsername string) AdminBracketDTO {
	authorID := bracket.Author.PublicID
	return AdminBracketDTO{
		ID:             bracket.PublicID,
		Title:          bracket.Title,
		AuthorID:       authorID,
		AuthorUsername: authorUsername,
		Status:         bracket.Status,
		CurrentRound:   bracket.CurrentRound,
		LikesCount:     likesCount,
		CreatedAt:      bracket.CreatedAt,
		UpdatedAt:      bracket.UpdatedAt,
	}
}
