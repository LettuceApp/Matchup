package controllers

import (
	"testing"
	"time"

	bracketv1 "Matchup/gen/bracket/v1"
	matchupv1 "Matchup/gen/matchup/v1"
)

func TestRfc3339(t *testing.T) {
	t.Run("formats UTC correctly", func(t *testing.T) {
		ts := time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC)
		got := rfc3339(ts)
		want := "2024-06-15T12:30:00Z"
		if got != want {
			t.Errorf("rfc3339(%v) = %q, want %q", ts, got, want)
		}
	})

	t.Run("converts non-UTC to UTC", func(t *testing.T) {
		loc := time.FixedZone("EST", -5*3600)
		ts := time.Date(2024, 6, 15, 7, 30, 0, 0, loc) // 07:30 EST = 12:30 UTC
		got := rfc3339(ts)
		want := "2024-06-15T12:30:00Z"
		if got != want {
			t.Errorf("rfc3339(%v) = %q, want %q", ts, got, want)
		}
	})
}

func TestRfc3339Ptr(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		got := rfc3339Ptr(nil)
		if got != nil {
			t.Errorf("rfc3339Ptr(nil) = %v, want nil", got)
		}
	})

	t.Run("non-nil input returns formatted string", func(t *testing.T) {
		ts := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
		got := rfc3339Ptr(&ts)
		if got == nil {
			t.Fatal("rfc3339Ptr returned nil, want non-nil")
		}
		want := "2024-01-02T03:04:05Z"
		if *got != want {
			t.Errorf("rfc3339Ptr(%v) = %q, want %q", ts, *got, want)
		}
	})
}

func TestPopularMatchupToProto(t *testing.T) {
	t.Run("copies all scalar fields including author_username and created_at", func(t *testing.T) {
		round := 2
		cur := 3
		bID := "bracket-public-id"
		bAuthorID := "bracket-author-public-id"
		dto := PopularMatchupDTO{
			ID:              "matchup-public-id",
			Title:           "Best Rapper",
			AuthorID:        "author-public-id",
			AuthorUsername:  "cordell",
			BracketID:       &bID,
			BracketAuthorID: &bAuthorID,
			Round:           &round,
			CurrentRound:    &cur,
			Votes:           42,
			Likes:           7,
			Comments:        3,
			EngagementScore: 123.45,
			Rank:            1,
			CreatedAt:       "2024-06-15T12:30:00Z",
		}
		got := popularMatchupToProto(dto)

		if got.Id != "matchup-public-id" {
			t.Errorf("Id = %q, want %q", got.Id, "matchup-public-id")
		}
		if got.Title != "Best Rapper" {
			t.Errorf("Title = %q, want %q", got.Title, "Best Rapper")
		}
		if got.AuthorId != "author-public-id" {
			t.Errorf("AuthorId = %q, want %q", got.AuthorId, "author-public-id")
		}
		if got.AuthorUsername != "cordell" {
			t.Errorf("AuthorUsername = %q, want %q", got.AuthorUsername, "cordell")
		}
		if got.BracketId == nil || *got.BracketId != "bracket-public-id" {
			t.Errorf("BracketId = %v, want %q", got.BracketId, "bracket-public-id")
		}
		if got.BracketAuthorId == nil || *got.BracketAuthorId != "bracket-author-public-id" {
			t.Errorf("BracketAuthorId = %v, want %q", got.BracketAuthorId, "bracket-author-public-id")
		}
		if got.Round == nil || *got.Round != 2 {
			t.Errorf("Round = %v, want 2", got.Round)
		}
		if got.CurrentRound == nil || *got.CurrentRound != 3 {
			t.Errorf("CurrentRound = %v, want 3", got.CurrentRound)
		}
		if got.Votes != 42 {
			t.Errorf("Votes = %d, want 42", got.Votes)
		}
		if got.Likes != 7 {
			t.Errorf("Likes = %d, want 7", got.Likes)
		}
		if got.Comments != 3 {
			t.Errorf("Comments = %d, want 3", got.Comments)
		}
		if got.EngagementScore != 123.45 {
			t.Errorf("EngagementScore = %f, want 123.45", got.EngagementScore)
		}
		if got.Rank != 1 {
			t.Errorf("Rank = %d, want 1", got.Rank)
		}
		if got.CreatedAt != "2024-06-15T12:30:00Z" {
			t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, "2024-06-15T12:30:00Z")
		}
	})

	t.Run("nil round and nil bracket pass through as nil", func(t *testing.T) {
		dto := PopularMatchupDTO{
			ID:              "m1",
			Title:           "Standalone",
			AuthorID:        "a1",
			AuthorUsername:  "cj",
			Round:           nil,
			CurrentRound:    nil,
			BracketID:       nil,
			BracketAuthorID: nil,
			CreatedAt:       "2024-01-01T00:00:00Z",
		}
		got := popularMatchupToProto(dto)
		if got.Round != nil {
			t.Errorf("expected nil Round, got %v", got.Round)
		}
		if got.CurrentRound != nil {
			t.Errorf("expected nil CurrentRound, got %v", got.CurrentRound)
		}
		if got.BracketId != nil {
			t.Errorf("expected nil BracketId, got %v", got.BracketId)
		}
		if got.BracketAuthorId != nil {
			t.Errorf("expected nil BracketAuthorId, got %v", got.BracketAuthorId)
		}
	})

	t.Run("returns non-nil struct for zero-value DTO", func(t *testing.T) {
		got := popularMatchupToProto(PopularMatchupDTO{})
		if got == nil {
			t.Fatal("popularMatchupToProto returned nil")
		}
		var _ *matchupv1.PopularMatchupData = got // type assertion lives at compile time
	})
}

func TestPopularBracketToProto(t *testing.T) {
	t.Run("copies all scalar fields including author_username and created_at", func(t *testing.T) {
		dto := PopularBracketDTO{
			ID:              "bracket-public-id",
			Title:           "Greatest Rapper Tournament",
			AuthorID:        "author-public-id",
			AuthorUsername:  "cordell",
			CurrentRound:    2,
			Size:            16,
			Votes:           100,
			Likes:           25,
			Comments:        10,
			EngagementScore: 999.5,
			Rank:            1,
			CreatedAt:       "2024-06-15T12:30:00Z",
		}
		got := popularBracketToProto(dto)

		if got.Id != "bracket-public-id" {
			t.Errorf("Id = %q, want %q", got.Id, "bracket-public-id")
		}
		if got.Title != "Greatest Rapper Tournament" {
			t.Errorf("Title = %q, want %q", got.Title, "Greatest Rapper Tournament")
		}
		if got.AuthorId != "author-public-id" {
			t.Errorf("AuthorId = %q, want %q", got.AuthorId, "author-public-id")
		}
		if got.AuthorUsername != "cordell" {
			t.Errorf("AuthorUsername = %q, want %q", got.AuthorUsername, "cordell")
		}
		if got.CurrentRound != 2 {
			t.Errorf("CurrentRound = %d, want 2", got.CurrentRound)
		}
		if got.Size != 16 {
			t.Errorf("Size = %d, want 16", got.Size)
		}
		if got.Votes != 100 {
			t.Errorf("Votes = %d, want 100", got.Votes)
		}
		if got.Likes != 25 {
			t.Errorf("Likes = %d, want 25", got.Likes)
		}
		if got.Comments != 10 {
			t.Errorf("Comments = %d, want 10", got.Comments)
		}
		if got.EngagementScore != 999.5 {
			t.Errorf("EngagementScore = %f, want 999.5", got.EngagementScore)
		}
		if got.Rank != 1 {
			t.Errorf("Rank = %d, want 1", got.Rank)
		}
		if got.CreatedAt != "2024-06-15T12:30:00Z" {
			t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, "2024-06-15T12:30:00Z")
		}
	})

	t.Run("returns non-nil struct for zero-value DTO", func(t *testing.T) {
		got := popularBracketToProto(PopularBracketDTO{})
		if got == nil {
			t.Fatal("popularBracketToProto returned nil")
		}
		var _ *bracketv1.PopularBracketData = got
	})
}
