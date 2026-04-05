package models

import "time"

type Follow struct {
	ID         uint      `db:"id" json:"id"`
	FollowerID uint      `db:"follower_id" json:"follower_id"`
	FollowedID uint      `db:"followed_id" json:"followed_id"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}
