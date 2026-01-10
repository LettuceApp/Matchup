package controllers

const (
	matchupStatusActive    = "active"
	matchupStatusPublished = "published"
	matchupStatusDraft     = "draft"
	matchupStatusCompleted = "completed"
)

func isMatchupOpenStatus(status string) bool {
	return status == matchupStatusActive || status == matchupStatusPublished
}
