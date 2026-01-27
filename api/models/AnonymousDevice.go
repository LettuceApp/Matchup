package models

import "time"

type AnonymousDevice struct {
	ID            uint      `gorm:"primary_key;autoIncrement" json:"id"`
	DeviceID      string    `gorm:"size:36;not null;uniqueIndex" json:"device_id"`
	UserAgentHash string    `gorm:"not null" json:"-"`
	IPHash        string    `gorm:"not null" json:"-"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	LastSeenAt    time.Time `gorm:"autoUpdateTime" json:"last_seen_at"`
}
