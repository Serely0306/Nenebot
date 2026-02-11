package liarsbar

import (
	"math/rand"
	"time"
)

const (
	MIN_PLAYERS    = 2
	HAND_SIZE      = 5
	GUN_CHAMBERS   = 6
	LIVE_BULLETS   = 3
	MAX_PLAY_CARDS = 3
)

var CARD_TYPES_BASE = []string{"A", "K", "Q"}
var JOKER = "Joker"

type GameStatus int

const (
	WAITING GameStatus = iota
	PLAYING
	ENDED
)

type ShotResult int

const (
	SAFE ShotResult = iota
	HIT
	ALREADY_ELIMINATED
	GUN_ERROR
)

type ChallengeResult int

const (
	CH_SUCCESS ChallengeResult = iota
	CH_FAILURE
)

type PlayerData struct {
	ID           string
	Name         string
	Hand         []string
	Gun          []string
	GunPosition  int
	IsEliminated bool
}

type LastPlay struct {
	PlayerID        string
	PlayerName      string
	ClaimedQuantity int
	ActualCards     []string
}

type GameState struct {
	Status             GameStatus
	Players            map[string]*PlayerData
	Deck               []string
	MainCard           string
	TurnOrder          []string
	CurrentPlayerIndex int
	LastPlay           *LastPlay
	DiscardPile        []string
	CreatorID          string
	RoundStartReason   string
}

func InitializeGun() ([]string, int) {
	rand.Seed(time.Now().UnixNano())
	live := LIVE_BULLETS
	empty := GUN_CHAMBERS - live
	if empty < 0 {
		empty = 1
		live = GUN_CHAMBERS - 1
	}
	bullets := make([]string, 0, GUN_CHAMBERS)
	for i := 0; i < empty; i++ {
		bullets = append(bullets, "空弹")
	}
	for i := 0; i < live; i++ {
		bullets = append(bullets, "实弹")
	}
	// shuffle
	for i := range bullets {
		j := rand.Intn(i + 1)
		bullets[i], bullets[j] = bullets[j], bullets[i]
	}
	pos := rand.Intn(GUN_CHAMBERS)
	return bullets, pos
}
