package controllers

import (
	"context"
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
		ThemeGradient:  user.ThemeGradient,
		WinsCount:      user.WinsCount,
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

func matchupItemToProto(db sqlx.ExtContext, item models.MatchupItem) *commonv1.MatchupItemResponse {
	// Lift the relative DB path to a full S3 URL on the wire so
	// frontends never deal with bucket/region details. Empty path
	// stays empty — frontend uses that as the "no thumbnail" signal.
	imageURL := ""
	if item.ImagePath != "" {
		imageURL = appdb.ProcessMatchupItemImagePath(item.ImagePath)
	}

	resp := &commonv1.MatchupItemResponse{
		Id:       item.PublicID,
		Item:     item.Item,
		Votes:    int32(item.Votes),
		ImageUrl: imageURL,
	}

	// User-as-item ("contender") hydration. When user_id is set, look
	// up the public_id + username + avatar so the frontend can render
	// an avatar chip in place of the text label. Lookup is N=1 per
	// matchup item (typically 2–8 items), so a per-item SELECT is
	// fine — no batching needed at this scale. If the lookup fails
	// (e.g. user was hard-deleted while the FK's ON DELETE SET NULL
	// hadn't fired yet), we just leave the user_* fields empty and
	// the frontend falls back to the plain text/image render.
	if item.UserID != nil && *item.UserID != 0 {
		var u struct {
			PublicID   string `db:"public_id"`
			Username   string `db:"username"`
			AvatarPath string `db:"avatar_path"`
		}
		if err := sqlx.GetContext(context.Background(), db, &u,
			"SELECT public_id, username, avatar_path FROM users WHERE id = $1",
			*item.UserID,
		); err == nil {
			resp.UserId = u.PublicID
			resp.UserUsername = u.Username
			resp.UserAvatarPath = appdb.ProcessAvatarPath(u.AvatarPath)
		}
	}

	return resp
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
		items[i] = matchupItemToProto(db, item)
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

	// Resolve community public_id (when present) so the frontend can
	// detect community-scoped matchups and route accordingly.
	var communityID *string
	if matchup.CommunityID != nil {
		var pid string
		if err := sqlx.GetContext(context.Background(), db, &pid,
			"SELECT public_id FROM communities WHERE id = $1", *matchup.CommunityID); err == nil && pid != "" {
			communityID = &pid
		}
	}

	return &matchupv1.MatchupData{
		Id:              matchup.PublicID,
		ShortId:         shortID,
		Title:           matchup.Title,
		Content:         matchup.Content,
		AuthorId:        authorID,
		// matchup.Author is hydrated by SELECT * FROM users (see
		// Matchup.LoadAuthor) and Author.ProcessAvatarPath() lifts the
		// raw DB column to a CDN-style absolute URL. We pass that URL
		// through as-is on the wire so the frontend doesn't need to
		// rebuild it; HomeCard renders an <img src> directly when
		// non-empty and falls back to the username initial otherwise.
		Author:          &commonv1.UserSummaryResponse{Id: authorID, Username: matchup.Author.Username, AvatarPath: matchup.Author.AvatarPath},
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
		CommunityId:     communityID,
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
		AuthorAvatarPath: dto.AuthorAvatarPath,
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

	// Resolve community public_id when this bracket is community-scoped.
	var communityID *string
	if bracket.CommunityID != nil {
		var pid string
		if err := sqlx.GetContext(context.Background(), db, &pid,
			"SELECT public_id FROM communities WHERE id = $1", *bracket.CommunityID); err == nil && pid != "" {
			communityID = &pid
		}
	}

	// Cumulative vote total across every child matchup. Computed
	// from matchup_items rather than aggregating in-memory because
	// the proto mapper doesn't have the items loaded — saves an N+1
	// fan-out from the caller. Single COUNT-style aggregate; ~ms
	// cost on a reasonably-sized bracket. Best-effort: a DB error
	// just leaves total_votes at zero rather than failing the whole
	// proto render.
	var totalVotes int64
	_ = sqlx.GetContext(context.Background(), db, &totalVotes, `
		SELECT COALESCE(SUM(mi.votes), 0)::bigint
		FROM matchup_items mi
		JOIN matchups m ON m.id = mi.matchup_id
		WHERE m.bracket_id = $1
	`, bracket.ID)

	return &bracketv1.BracketData{
		Id:                   bracket.PublicID,
		ShortId:              shortID,
		Title:                bracket.Title,
		Description:          bracket.Description,
		AuthorId:             authorID,
		Author:               &commonv1.UserSummaryResponse{Id: authorID, Username: bracket.Author.Username, AvatarPath: bracket.Author.AvatarPath},
		Size:                 int32(bracket.Size),
		Status:               bracket.Status,
		CurrentRound:         int32(bracket.CurrentRound),
		AdvanceMode:          bracket.AdvanceMode,
		RoundDurationSeconds: int32(bracket.RoundDurationSeconds),
		RoundStartedAt:       rfc3339Ptr(bracket.RoundStartedAt),
		RoundEndsAt:          rfc3339Ptr(bracket.RoundEndsAt),
		LikesCount:           int32(bracket.LikesCount),
		Tags:                 tags,
		CommunityId:          communityID,
		TotalVotes:           totalVotes,
	}
}

func popularBracketToProto(dto PopularBracketDTO) *bracketv1.PopularBracketData {
	return &bracketv1.PopularBracketData{
		Id:              dto.ID,
		Title:           dto.Title,
		AuthorId:        dto.AuthorID,
		AuthorUsername:  dto.AuthorUsername,
		AuthorAvatarPath: dto.AuthorAvatarPath,
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
		items[i] = matchupItemToProto(db, item)
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
