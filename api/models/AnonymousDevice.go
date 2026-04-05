package models

import "time"

type AnonymousDevice struct {
	ID            uint      `db:"id" json:"id"`
	DeviceID      string    `db:"device_id" json:"device_id"`
	UserAgentHash string    `db:"user_agent_hash" json:"-"`
	IPHash        string    `db:"ip_hash" json:"-"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	LastSeenAt    time.Time `db:"last_seen_at" json:"last_seen_at"`
}
