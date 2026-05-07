package models

import "time"

// MatchupVote is one row in matchup_votes — a recorded action by a
// user or anon device on a matchup. Kind distinguishes picks from
// skips (migration 025); a pick references a specific item via
// MatchupItemPublicID, a skip leaves it nil.
type MatchupVote struct {
	ID              uint      `db:"id" json:"id"`
	PublicID        string    `db:"public_id" json:"public_id"`
	UserID          *uint     `db:"user_id" json:"user_id,omitempty"`
	AnonID          *string   `db:"anon_id" json:"anon_id,omitempty"`
	MatchupPublicID string    `db:"matchup_public_id" json:"matchup_public_id"`
	// Nullable since migration 025: skip rows have no item.
	// Pick rows always have a non-nil pointer here.
	MatchupItemPublicID *string `db:"matchup_item_public_id" json:"matchup_item_public_id,omitempty"`
	// Kind is 'pick' or 'skip'. Defaults to 'pick' on insert via the
	// column default; existing pre-migration rows backfilled to 'pick'.
	Kind      string    `db:"kind" json:"kind"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// PickedItemID returns the item public_id for a pick row, or "" if
// this is a skip (or any other kind without an item reference).
// Most call sites want the string form for proto / JSON, and "" reads
// cleaner than nil-checks at every use site.
func (v *MatchupVote) PickedItemID() string {
	if v.MatchupItemPublicID == nil {
		return ""
	}
	return *v.MatchupItemPublicID
}
