package responses

import "time"

type BracketResponse struct {
	ID           uint      `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Status       string    `json:"status"`
	CurrentRound int       `json:"current_round"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	Author BracketAuthorResponse `json:"author"`
}

type BracketAuthorResponse struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}
