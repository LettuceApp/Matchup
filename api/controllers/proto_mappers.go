package controllers

import (
	"time"

	adminv1 "Matchup/gen/admin/v1"
	commentv1 "Matchup/gen/comment/v1"
	commonv1 "Matchup/gen/common/v1"
	likev1 "Matchup/gen/like/v1"
	matchupv1 "Matchup/gen/matchup/v1"
	bracketv1 "Matchup/gen/bracket/v1"
	userv1 "Matchup/gen/user/v1"
	appdb "Matchup/db"
	"Matchup/models"

	"github.com/jmoiron/sqlx"
)

// ---- time helpers ----

func rfc3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func rfc3339Ptr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// ---- user protos ----

func userToProto(user *models.User) *userv1.UserProfile {
	// Deleted users — self-deleted or admin-banned — surface to the
	// outside world as a single anonymous "[deleted]" placeholder.
	// Public fields (username, email, avatar, bio) are blanked server-
	// side so even an admin-dash user list can't accidentally leak
	// the old identity. Counters stay accurate — they're stat, not
	// identifying, and wiping them would confuse engagement history.
	if user.DeletedAt != nil {
		return &userv1.UserProfile{
			Id:             user.PublicID,
			Username:       "",
			Email:          "",
			AvatarPath:     "",
			IsAdmin:        false,
			IsPrivate:      true,
			FollowersCount: int64(user.FollowersCount),
			FollowingCount: int64(user.FollowingCount),
			CreatedAt:      rfc3339(user.CreatedAt),
			UpdatedAt:      rfc3339(user.UpdatedAt),
			IsDeleted:      true,
		}
	}

	var bio *string
	if user.Bio != "" {
		b := user.Bio
		bio = &b
	}
	return &userv1.UserProfile{
		Id:             user.PublicID,
		Username:       user.Username,
		Email:          user.Email,
		AvatarPath:     user.AvatarPath,
		IsAdmin:        user.IsAdmin,
		IsPrivate:      user.IsPrivate,
		FollowersCount: int64(user.FollowersCount),
		FollowingCount: int64(user.FollowingCount),
		CreatedAt:      rfc3339(user.CreatedAt),
		UpdatedAt:      rfc3339(user.UpdatedAt),
		Bio:            bio,
		IsVerified:     user.EmailVerifiedAt != nil,
	}
}

func followListUserToProto(dto FollowUserDTO) *userv1.FollowListUser {
	return &userv1.FollowListUser{
		Id:             dto.ID,
		Username:       dto.Username,
		Email:          dto.Email,
		AvatarPath:     dto.AvatarPath,
		IsAdmin:        dto.IsAdmin,
		FollowersCount: int64(dto.FollowersCount),
		FollowingCount: int64(dto.FollowingCount),
		ViewerFollowing:  dto.ViewerFollowing,
		ViewerFollowedBy: dto.ViewerFollowedBy,
		Mutual:           dto.Mutual,
	}
}

// ---- common protos ----

func matchupItemToProto(item models.MatchupItem) *commonv1.MatchupItemResponse {
	return &commonv1.MatchupItemResponse{
		Id:    item.PublicID,
		Item:  item.Item,
		Votes: int32(item.Votes),
	}
}

func paginationToProto(page, limit int, total int64) *commonv1.Pagination {
	totalPages := int32(0)
	if limit > 0 {
		totalPages = int32((total + int64(limit) - 1) / int64(limit))
	}
	return &commonv1.Pagination{
		Page:       int32(page),
		Limit:      int32(limit),
		Total:      total,
		TotalPages: totalPages,
	}
}

// ---- matchup protos ----

func matchupToProto(db sqlx.ExtContext, matchup *models.Matchup, comments []models.Comment) *matchupv1.MatchupData {
	items := make([]*commonv1.MatchupItemResponse, len(matchup.Items))
	for i, item := range matchup.Items {
		items[i] = matchupItemToProto(item)
	}

	commentProtos := make([]*matchupv1.CommentData, len(comments))
	for i, c := range comments {
		userPublicID := resolveUserPublicID(db, &c.Author, c.UserID)
		commentProtos[i] = &matchupv1.CommentData{
			Id:        c.PublicID,
			UserId:    userPublicID,
			Username:  c.Author.Username,
			Body:      c.Body,
			CreatedAt: rfc3339(c.CreatedAt),
			UpdatedAt: rfc3339(c.UpdatedAt),
		}
	}

	authorID := resolveUserPublicID(db, &matchup.Author, matchup.AuthorID)
	bracketPublicID := resolveBracketPublicID(db, matchup.BracketID)

	var winnerItemID *string
	if matchup.WinnerItemID != nil {
		for _, item := range matchup.Items {
			if item.ID == *matchup.WinnerItemID && item.PublicID != "" {
				id := item.PublicID
				winnerItemID = &id
				break
			}
		}
		if winnerItemID == nil {
			if resolved := resolveMatchupItemPublicID(db, *matchup.WinnerItemID); resolved != "" {
				winnerItemID = &resolved
			}
		}
	}

	var round *int32
	if matchup.Round != nil {
		r := int32(*matchup.Round)
		round = &r
	}
	var seed *int32
	if matchup.Seed != nil {
		s := int32(*matchup.Seed)
		seed = &s
	}

	var imageURL *string
	if matchup.ImagePath != "" {
		u := appdb.ProcessMatchupImagePath(matchup.ImagePath)
		imageURL = &u
	}
	tags := []string(matchup.Tags)
	if tags == nil {
		tags = []string{}
	}

	shortID := ""
	if matchup.ShortID != nil {
		shortID = *matchup.ShortID
	}

	return &matchupv1.MatchupData{
		Id:              matchup.PublicID,
		ShortId:         shortID,
		Title:           matchup.Title,
		Content:         matchup.Content,
		AuthorId:        authorID,
		Author:          &commonv1.UserSummaryResponse{Id: authorID, Username: matchup.Author.Username},
		Items:           items,
		Comments:        commentProtos,
		LikesCount:      int32(matchup.LikesCount),
		CreatedAt:       rfc3339(matchup.CreatedAt),
		UpdatedAt:       rfc3339(matchup.UpdatedAt),
		BracketId:       bracketPublicID,
		Round:           round,
		Seed:            seed,
		Status:          matchup.Status,
		EndMode:         matchup.EndMode,
		DurationSeconds: int32(matchup.DurationSeconds),
		StartTime:       rfc3339Ptr(matchup.StartTime),
		EndTime:         rfc3339Ptr(matchup.EndTime),
		WinnerItemId:    winnerItemID,
		ImageUrl:        imageURL,
		Tags:            tags,
	}
}

func popularMatchupToProto(dto PopularMatchupDTO) *matchupv1.PopularMatchupData {
	var round, currentRound *int32
	if dto.Round != nil {
		r := int32(*dto.Round)
		round = &r
	}
	if dto.CurrentRound != nil {
		cr := int32(*dto.CurrentRound)
		currentRound = &cr
	}
	return &matchupv1.PopularMatchupData{
		Id:              dto.ID,
		Title:           dto.Title,
		AuthorId:        dto.AuthorID,
		AuthorUsername:  dto.AuthorUsername,
		BracketId:       dto.BracketID,
		BracketAuthorId: dto.BracketAuthorID,
		Round:           round,
		CurrentRound:    currentRound,
		Votes:           int32(dto.Votes),
		Likes:           int32(dto.Likes),
		Comments:        int32(dto.Comments),
		EngagementScore: dto.EngagementScore,
		Rank:            int32(dto.Rank),
		CreatedAt:       dto.CreatedAt,
	}
}

func voteToProto(vote models.MatchupVote) *matchupv1.MatchupVoteData {
	var userID *string
	if vote.UserID != nil {
		s := resolveUserPublicID(nil, nil, *vote.UserID)
		userID = &s
	}
	return &matchupv1.MatchupVoteData{
		Id:            vote.PublicID,
		UserId:        userID,
		MatchupId:     vote.MatchupPublicID,
		MatchupItemId: vote.PickedItemID(),
		CreatedAt:     rfc3339(vote.CreatedAt),
	}
}

// ---- bracket protos ----

func bracketToProto(db sqlx.ExtContext, bracket *models.Bracket) *bracketv1.BracketData {
	authorID := resolveUserPublicID(db, &bracket.Author, bracket.AuthorID)
	tags := []string(bracket.Tags)
	if tags == nil {
		tags = []string{}
	}
	shortID := ""
	if bracket.ShortID != nil {
		shortID = *bracket.ShortID
	}

	return &bracketv1.BracketData{
		Id:                   bracket.PublicID,
		ShortId:              shortID,
		Title:                bracket.Title,
		Description:          bracket.Description,
		AuthorId:             authorID,
		Author:               &commonv1.UserSummaryResponse{Id: authorID, Username: bracket.Author.Username},
		Size:                 int32(bracket.Size),
		Status:               bracket.Status,
		CurrentRound:         int32(bracket.CurrentRound),
		AdvanceMode:          bracket.AdvanceMode,
		RoundDurationSeconds: int32(bracket.RoundDurationSeconds),
		RoundStartedAt:       rfc3339Ptr(bracket.RoundStartedAt),
		RoundEndsAt:          rfc3339Ptr(bracket.RoundEndsAt),
		LikesCount:           int32(bracket.LikesCount),
		Tags:                 tags,
	}
}

func popularBracketToProto(dto PopularBracketDTO) *bracketv1.PopularBracketData {
	return &bracketv1.PopularBracketData{
		Id:              dto.ID,
		Title:           dto.Title,
		AuthorId:        dto.AuthorID,
		AuthorUsername:  dto.AuthorUsername,
		CurrentRound:    int32(dto.CurrentRound),
		Size:            int32(dto.Size),
		Votes:           int32(dto.Votes),
		Likes:           int32(dto.Likes),
		Comments:        int32(dto.Comments),
		EngagementScore: dto.EngagementScore,
		Rank:            int32(dto.Rank),
		CreatedAt:       dto.CreatedAt,
	}
}

// ---- comment protos ----

func commentToProto(db sqlx.ExtContext, comment models.Comment) *commentv1.CommentData {
	userPublicID := resolveUserPublicID(db, &comment.Author, comment.UserID)
	return &commentv1.CommentData{
		Id:        comment.PublicID,
		UserId:    userPublicID,
		Username:  comment.Author.Username,
		Body:      comment.Body,
		CreatedAt: rfc3339(comment.CreatedAt),
		UpdatedAt: rfc3339(comment.UpdatedAt),
	}
}

func bracketCommentToProto(db sqlx.ExtContext, comment models.BracketComment) *commentv1.BracketCommentData {
	userPublicID := resolveUserPublicID(db, &comment.Author, comment.UserID)
	return &commentv1.BracketCommentData{
		Id:        comment.PublicID,
		UserId:    userPublicID,
		Username:  comment.Author.Username,
		Body:      comment.Body,
		CreatedAt: rfc3339(comment.CreatedAt),
		UpdatedAt: rfc3339(comment.UpdatedAt),
	}
}

// ---- like protos ----

func likeToProto(like models.Like, userPublicID, matchupPublicID string) *likev1.LikeData {
	return &likev1.LikeData{
		Id:        like.PublicID,
		UserId:    userPublicID,
		MatchupId: matchupPublicID,
		CreatedAt: rfc3339(like.CreatedAt),
	}
}

func bracketLikeToProto(like models.BracketLike, userPublicID, bracketPublicID string) *likev1.BracketLikeData {
	return &likev1.BracketLikeData{
		Id:        like.PublicID,
		UserId:    userPublicID,
		BracketId: bracketPublicID,
		CreatedAt: rfc3339(like.CreatedAt),
	}
}

// ---- admin protos ----

func adminMatchupToProto(db sqlx.ExtContext, matchup *models.Matchup) *adminv1.AdminMatchupData {
	items := make([]*commonv1.MatchupItemResponse, len(matchup.Items))
	for i, item := range matchup.Items {
		items[i] = matchupItemToProto(item)
	}
	authorID := resolveUserPublicID(db, &matchup.Author, matchup.AuthorID)
	authorUsername := ""
	if matchup.Author.ID != 0 {
		authorUsername = matchup.Author.Username
	}
	return &adminv1.AdminMatchupData{
		Id:             matchup.PublicID,
		Title:          matchup.Title,
		Content:        matchup.Content,
		AuthorId:       authorID,
		AuthorUsername: authorUsername,
		Items:          items,
		LikesCount:     int64(matchup.LikesCount),
		CreatedAt:      rfc3339(matchup.CreatedAt),
		UpdatedAt:      rfc3339(matchup.UpdatedAt),
	}
}

func adminBracketToProto(bracket *models.Bracket, authorUsername string) *adminv1.AdminBracketData {
	return &adminv1.AdminBracketData{
		Id:             bracket.PublicID,
		Title:          bracket.Title,
		AuthorId:       bracket.Author.PublicID,
		AuthorUsername: authorUsername,
		Status:         bracket.Status,
		CurrentRound:   int32(bracket.CurrentRound),
		LikesCount:     int64(bracket.LikesCount),
		CreatedAt:      rfc3339(bracket.CreatedAt),
		UpdatedAt:      rfc3339(bracket.UpdatedAt),
	}
}
