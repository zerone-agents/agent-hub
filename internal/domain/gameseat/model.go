package gameseat

import "time"

// Character binds a stable external game identity to an existing Hub Agent.
type Character struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	TenantID  string `gorm:"type:varchar(64);not null;uniqueIndex:uk_game_seat_character,priority:1"`
	GameKey   string `gorm:"type:varchar(64);not null;uniqueIndex:uk_game_seat_character,priority:2"`
	Key       string `gorm:"type:varchar(64);not null;uniqueIndex:uk_game_seat_character,priority:3"`
	AgentID   uint64 `gorm:"not null;index"`
	Name      string `gorm:"type:varchar(128);not null"`
	AvatarURL string `gorm:"type:varchar(512)"`
	Bio       string `gorm:"type:text"`
	Voice     string `gorm:"type:varchar(128)"`
	UpdatedAt time.Time
}

func (Character) TableName() string { return "game_seat_characters" }

// PlayerRun remains stable across sessions for one game and one player.
type PlayerRun struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement"`
	TenantID string `gorm:"type:varchar(64);not null;uniqueIndex:uk_game_seat_player,priority:1"`
	GameKey  string `gorm:"type:varchar(64);not null;uniqueIndex:uk_game_seat_player,priority:2"`
	PlayerID string `gorm:"type:varchar(128);not null;uniqueIndex:uk_game_seat_player,priority:3"`
	RunID    string `gorm:"type:char(36);not null"`
	Games    int    `gorm:"not null;default:0"`
}

func (PlayerRun) TableName() string { return "game_seat_player_runs" }

type Session struct {
	ID         string        `gorm:"type:char(36);primaryKey"`
	TenantID   string        `gorm:"type:varchar(64);not null;index"`
	GameKey    string        `gorm:"type:varchar(64);not null"`
	PlayerID   string        `gorm:"type:varchar(128);not null"`
	PlayerName string        `gorm:"type:varchar(128);not null"`
	RunID      string        `gorm:"type:char(36);not null"`
	GameNumber int           `gorm:"not null"`
	Calls      int           `gorm:"not null;default:0"`
	Seats      []SessionSeat `gorm:"foreignKey:SessionID"`
	CreatedAt  time.Time
}

func (Session) TableName() string { return "game_seat_sessions" }

type SessionSeat struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	SessionID   string `gorm:"type:char(36);not null;uniqueIndex:uk_game_seat_session_seat,priority:1"`
	Seat        int    `gorm:"not null;uniqueIndex:uk_game_seat_session_seat,priority:2"`
	CharacterID uint64 `gorm:"not null"`
}

func (SessionSeat) TableName() string { return "game_seat_session_seats" }

type Event struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	SessionID string `gorm:"type:char(36);not null;uniqueIndex:uk_game_seat_event,priority:1"`
	Seq       int    `gorm:"not null;uniqueIndex:uk_game_seat_event,priority:2"`
	Type      string `gorm:"type:varchar(32);not null"`
	CreatedAt time.Time
}

func (Event) TableName() string { return "game_seat_events" }
