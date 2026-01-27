package models

import "time"

type Follow struct {
	ID         uint      `gorm:"primary_key;autoIncrement" json:"id"`
	FollowerID uint      `gorm:"not null;index;uniqueIndex:idx_follows_unique;index:idx_follows_follower_created,priority:1" json:"follower_id"`
	FollowedID uint      `gorm:"not null;index;uniqueIndex:idx_follows_unique;index:idx_follows_followed_created,priority:1" json:"followed_id"`
	CreatedAt  time.Time `gorm:"autoCreateTime;index:idx_follows_followed_created,priority:2;index:idx_follows_follower_created,priority:2" json:"created_at"`
}
