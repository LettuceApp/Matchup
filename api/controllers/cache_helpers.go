package controllers

import (
	"Matchup/cache"
	"context"
	"fmt"
)

func invalidateBracketSummaryCache(bracketID uint) {
	if bracketID == 0 {
		return
	}
	_ = cache.DeleteByPrefix(context.Background(), fmt.Sprintf("bracket_summary:%d:", bracketID))
}

func invalidateHomeSummaryCache(userID uint) {
	if userID == 0 {
		return
	}
	_ = cache.Delete(context.Background(), fmt.Sprintf("home_summary:%d", userID))
}
