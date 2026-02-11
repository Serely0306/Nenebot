package liarsbar

import (
	"errors"
	"math"
	"math/rand"
	"time"
)

func init() { rand.Seed(time.Now().UnixNano()) }

type LiarDiceGame struct {
	State *GameState
}

func NewGame(creator string) *LiarDiceGame {
	gs := &GameState{Status: WAITING, Players: make(map[string]*PlayerData), CurrentPlayerIndex: -1, CreatorID: creator}
	return &LiarDiceGame{State: gs}
}

func (g *LiarDiceGame) AddPlayer(id, name string) error {
	if g.State.Status != WAITING {
		return errors.New("游戏非等待状态，无法加入")
	}
	if _, ok := g.State.Players[id]; ok {
		return nil
	}
	gun, pos := InitializeGun()
	g.State.Players[id] = &PlayerData{ID: id, Name: name, Hand: []string{}, Gun: gun, GunPosition: pos}
	return nil
}

func (g *LiarDiceGame) getActivePlayerIDs() []string {
	ids := []string{}
	for _, pid := range g.State.TurnOrder {
		p := g.State.Players[pid]
		if p != nil && !p.IsEliminated {
			ids = append(ids, pid)
		}
	}
	if len(ids) == 0 {
		// fallback to players map order
		for pid, p := range g.State.Players {
			if !p.IsEliminated {
				ids = append(ids, pid)
			}
		}
	}
	return ids
}

func (g *LiarDiceGame) _buildDeck(playerCount int) []string {
	if playerCount <= 0 {
		return []string{}
	}
	handSize := HAND_SIZE
	numBase := len(CARD_TYPES_BASE)
	minBase := int(math.Max(5, float64(MAX_PLAY_CARDS*2)))
	totalNeeded := playerCount * handSize
	jokerCount := int(math.Ceil(float64(playerCount) / 2.0))
	totalBaseNeeded := totalNeeded - jokerCount
	perType := int(math.Ceil(float64(totalBaseNeeded) / float64(numBase)))
	if perType < minBase {
		perType = minBase
	}
	deck := []string{}
	for _, t := range CARD_TYPES_BASE {
		for i := 0; i < perType; i++ {
			deck = append(deck, t)
		}
	}
	for i := 0; i < jokerCount; i++ {
		deck = append(deck, JOKER)
	}
	return deck
}

func (g *LiarDiceGame) StartGame() (map[string]interface{}, error) {
	if g.State.Status != WAITING {
		return nil, errors.New("游戏非等待状态")
	}
	if len(g.State.Players) < MIN_PLAYERS {
		return nil, errors.New("参与人数不足")
	}
	ids := make([]string, 0, len(g.State.Players))
	for pid := range g.State.Players {
		ids = append(ids, pid)
	}
	g.State.MainCard = CARD_TYPES_BASE[rand.Intn(len(CARD_TYPES_BASE))]
	g.State.Deck = g._buildDeck(len(ids))
	// shuffle deck
	for i := range g.State.Deck {
		j := rand.Intn(i + 1)
		g.State.Deck[i], g.State.Deck[j] = g.State.Deck[j], g.State.Deck[i]
	}
	// deal
	// simple dealing: give HAND_SIZE cards from deck start
	tempHands := make(map[string][]string)
	for _, pid := range ids {
		tempHands[pid] = []string{}
	}
	for i := 0; i < HAND_SIZE; i++ {
		for _, pid := range ids {
			if len(g.State.Deck) == 0 {
				break
			}
			card := g.State.Deck[0]
			g.State.Deck = g.State.Deck[1:]
			tempHands[pid] = append(tempHands[pid], card)
		}
	}
	for pid, hand := range tempHands {
		g.State.Players[pid].Hand = hand
	}
	// set turn order randomly
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	g.State.TurnOrder = ids
	g.State.CurrentPlayerIndex = 0
	g.State.Status = PLAYING
	g.State.LastPlay = nil
	initialHands := make(map[string][]string)
	for pid, p := range g.State.Players {
		initialHands[pid] = p.Hand
	}
	first := g.GetCurrentPlayerID()
	res := map[string]interface{}{"success": true, "main_card": g.State.MainCard, "turn_order_names": ids, "initial_hands": initialHands, "first_player_id": first, "first_player_name": g.GetCurrentPlayerName()}
	return res, nil
}

func (g *LiarDiceGame) GetCurrentPlayerID() string {
	if g.State.Status != PLAYING || len(g.State.TurnOrder) == 0 || g.State.CurrentPlayerIndex < 0 || g.State.CurrentPlayerIndex >= len(g.State.TurnOrder) {
		return ""
	}
	return g.State.TurnOrder[g.State.CurrentPlayerIndex]
}
func (g *LiarDiceGame) GetCurrentPlayerName() string {
	id := g.GetCurrentPlayerID()
	if id == "" {
		return ""
	}
	p := g.State.Players[id]
	if p == nil {
		return ""
	}
	return p.Name
}

func (g *LiarDiceGame) ProcessPlayCard(playerID string, indices1based []int) (map[string]interface{}, error) {
	if g.State.Status != PLAYING {
		return nil, errors.New("游戏未在进行中")
	}
	if g.GetCurrentPlayerID() != playerID {
		return nil, errors.New("还没轮到你")
	}
	p := g.State.Players[playerID]
	if p == nil {
		return nil, errors.New("你不在游戏中")
	}
	if len(p.Hand) == 0 {
		return nil, errors.New("手牌为空")
	}
	if len(indices1based) < 1 || len(indices1based) > MAX_PLAY_CARDS {
		return nil, errors.New("出牌数量不合法")
	}
	// convert
	idxs := make(map[int]struct{})
	cards := []string{}
	for _, i := range indices1based {
		if i < 1 || i > len(p.Hand) {
			return nil, errors.New("无效编号")
		}
		if _, ok := idxs[i-1]; ok {
			return nil, errors.New("编号重复")
		}
		idxs[i-1] = struct{}{}
		cards = append(cards, p.Hand[i-1])
	}
	// remove from hand
	newHand := []string{}
	for i, c := range p.Hand {
		if _, ok := idxs[i]; !ok {
			newHand = append(newHand, c)
		}
	}
	p.Hand = newHand
	g.State.LastPlay = &LastPlay{PlayerID: playerID, PlayerName: p.Name, ClaimedQuantity: len(cards), ActualCards: cards}
	// check all-hands-empty -> may trigger reshuffle
	reshRes := g.checkAndHandleAllHandsEmptyInternal("玩家出牌后")
	if reshRes["reshuffled"].(bool) {
		// include play context
		reshRes["accepted_play_info"] = nil
		reshRes["player_who_played_id"] = playerID
		reshRes["player_who_played_name"] = p.Name
		reshRes["played_quantity"] = len(cards)
		reshRes["played_cards"] = cards
		reshRes["played_hand_empty"] = len(p.Hand) == 0
		reshRes["action"] = "play"
		return reshRes, nil
	}

	// advance turn
	nextID, nextName := g.advanceTurn()
	if nextID == "" {
		// no next -> end game
		g.State.Status = ENDED
		winner := g.getWinnerID()
		res := map[string]interface{}{"success": true, "action": "play", "player_id": playerID, "player_name": p.Name, "quantity_played": len(cards), "actual_cards": cards, "main_card": g.State.MainCard, "hand_after_play": p.Hand, "played_hand_empty": len(p.Hand) == 0, "accepted_play_info": nil, "game_ended": true}
		if winner != "" {
			res["winner_id"] = winner
			res["winner_name"] = g.State.Players[winner].Name
		}
		return res, nil
	}

	res := map[string]interface{}{"success": true, "action": "play", "player_id": playerID, "player_name": p.Name, "quantity_played": len(cards), "actual_cards": cards, "main_card": g.State.MainCard, "hand_after_play": p.Hand, "played_hand_empty": len(p.Hand) == 0, "next_player_id": nextID, "next_player_name": nextName, "next_player_hand_empty": len(g.State.Players[nextID].Hand) == 0, "reshuffled": false, "game_ended": false}
	return res, nil
}

func (g *LiarDiceGame) ProcessChallenge(challengerID string) (map[string]interface{}, error) {
	if g.State.Status != PLAYING {
		return nil, errors.New("游戏未在进行中")
	}
	if g.GetCurrentPlayerID() != challengerID {
		return nil, errors.New("还没轮到你")
	}
	if g.State.LastPlay == nil {
		return nil, errors.New("当前没有可质疑的出牌")
	}
	last := g.State.LastPlay
	challengedID := last.PlayerID
	challenger := g.State.Players[challengerID]
	challenged := g.State.Players[challengedID]
	isTrue := true
	for _, c := range last.ActualCards {
		if !(c == g.State.MainCard || c == JOKER) {
			isTrue = false
			break
		}
	}
	var challengeRes ChallengeResult
	var loserID string
	if isTrue {
		challengeRes = CH_FAILURE
		loserID = challengerID
	} else {
		challengeRes = CH_SUCCESS
		loserID = challengedID
	}
	// move discard
	g.State.DiscardPile = append(g.State.DiscardPile, last.ActualCards...)
	g.State.LastPlay = nil
	// determine shot
	shot := g.determineShotOutcome(loserID)
	applied := g.applyShotConsequences(loserID, shot)
	res := map[string]interface{}{"success": true, "action": "challenge", "challenger_id": challengerID, "challenger_name": challenger.Name, "challenged_player_id": challengedID, "challenged_player_name": challenged.Name, "claimed_quantity": last.ClaimedQuantity, "actual_cards": last.ActualCards, "main_card": g.State.MainCard, "challenge_result": challengeRes, "loser_id": loserID, "loser_name": g.State.Players[loserID].Name, "shot_outcome": shot}
	for k, v := range applied {
		res[k] = v
	}
	// decide next player
	if !applied["game_ended"].(bool) {
		// next is challenger if still alive
		next := ""
		if p, ok := g.State.Players[challengerID]; ok && !p.IsEliminated {
			next = challengerID
			g.State.CurrentPlayerIndex = g.indexOf(challengerID)
		}
		if next == "" {
			nid, _ := g.advanceTurn()
			next = nid
		}
		if next != "" {
			res["next_player_id"] = next
			res["next_player_name"] = g.State.Players[next].Name
		}
		// after challenge resolution, check if all hands empty triggers reshuffle
		if !res["reshuffled"].(bool) {
			resh := g.checkAndHandleAllHandsEmptyInternal("质疑结算后")
			if resh["reshuffled"].(bool) {
				// merge and return
				for k, v := range resh {
					res[k] = v
				}
				return res, nil
			}
		}
	}
	return res, nil
}

func (g *LiarDiceGame) ProcessWait(playerID string) (map[string]interface{}, error) {
	if g.State.Status != PLAYING {
		return nil, errors.New("游戏未在进行中")
	}
	if g.GetCurrentPlayerID() != playerID {
		return nil, errors.New("还没轮到你")
	}
	p := g.State.Players[playerID]
	if len(p.Hand) > 0 {
		return nil, errors.New("手牌非空不能等待")
	}
	accepted := map[string]interface{}{}
	if g.State.LastPlay != nil {
		g.State.DiscardPile = append(g.State.DiscardPile, g.State.LastPlay.ActualCards...)
		accepted["accepted_play_info"] = g.State.LastPlay.ActualCards
		g.State.LastPlay = nil
	}
	// check all-hands-empty -> may trigger reshuffle
	reshRes := g.checkAndHandleAllHandsEmptyInternal("玩家等待后")
	if reshRes["reshuffled"].(bool) {
		reshRes["accepted_play_info"] = accepted["accepted_play_info"]
		reshRes["player_who_waited_id"] = playerID
		reshRes["player_who_waited_name"] = p.Name
		reshRes["action"] = "wait"
		return reshRes, nil
	}

	nid, nname := g.advanceTurn()
	if nid == "" {
		g.State.Status = ENDED
		winner := g.getWinnerID()
		res := map[string]interface{}{"success": true, "action": "wait", "player_id": playerID, "player_name": p.Name, "accepted_play_info": accepted["accepted_play_info"], "game_ended": true}
		if winner != "" {
			res["winner_id"] = winner
			res["winner_name"] = g.State.Players[winner].Name
		}
		return res, nil
	}

	res := map[string]interface{}{"success": true, "action": "wait", "player_id": playerID, "player_name": p.Name, "next_player_id": nid, "next_player_name": nname, "next_player_hand_empty": len(g.State.Players[nid].Hand) == 0, "reshuffled": false, "game_ended": false}
	for k, v := range accepted {
		res[k] = v
	}
	return res, nil
}

// helpers
func (g *LiarDiceGame) indexOf(id string) int {
	for i, v := range g.State.TurnOrder {
		if v == id {
			return i
		}
	}
	return -1
}

func (g *LiarDiceGame) advanceTurn() (string, string) {
	n := len(g.State.TurnOrder)
	if n == 0 {
		return "", ""
	}
	for i := 1; i <= n; i++ {
		idx := (g.State.CurrentPlayerIndex + i) % n
		pid := g.State.TurnOrder[idx]
		if p := g.State.Players[pid]; p != nil && !p.IsEliminated {
			g.State.CurrentPlayerIndex = idx
			return pid, p.Name
		}
	}
	return "", ""
}

func (g *LiarDiceGame) determineShotOutcome(playerID string) ShotResult {
	p := g.State.Players[playerID]
	if p == nil {
		return GUN_ERROR
	}
	if p.IsEliminated {
		return ALREADY_ELIMINATED
	}
	if len(p.Gun) == 0 || p.GunPosition < 0 || p.GunPosition >= len(p.Gun) {
		return GUN_ERROR
	}
	if p.Gun[p.GunPosition] == "实弹" {
		return HIT
	}
	return SAFE
}

func (g *LiarDiceGame) applyShotConsequences(playerID string, shot ShotResult) map[string]interface{} {
	res := map[string]interface{}{"game_ended": false, "reshuffled": false}
	p := g.State.Players[playerID]
	if p == nil {
		res["error"] = "player not found"
		return res
	}
	// advance pointer
	p.GunPosition = (p.GunPosition + 1) % len(p.Gun)
	if shot == HIT {
		if !p.IsEliminated {
			p.IsEliminated = true
			// check end
			active := 0
			var lastID string
			for pid, pp := range g.State.Players {
				if !pp.IsEliminated {
					active++
					lastID = pid
				}
			}
			if active <= 1 {
				res["game_ended"] = true
				res["winner_id"] = lastID
				res["winner_name"] = g.State.Players[lastID].Name
				g.State.Status = ENDED
				return res
			}
			// reshuffle via helper
			rr := g.reshuffleInternal("玩家被淘汰后洗牌", playerID)
			for k, v := range rr {
				res[k] = v
			}
		}
	}
	return res
}

// checkAndHandleAllHandsEmptyInternal 检查所有活跃玩家手牌是否为空，若为空则触发重洗
func (g *LiarDiceGame) checkAndHandleAllHandsEmptyInternal(reason string) map[string]interface{} {
	res := map[string]interface{}{"reshuffled": false}
	active := g.getActivePlayerIDs()
	if len(active) == 0 {
		return res
	}
	allEmpty := true
	for _, pid := range active {
		p := g.State.Players[pid]
		if p != nil && len(p.Hand) > 0 {
			allEmpty = false
			break
		}
	}
	if !allEmpty {
		return res
	}
	// perform reshuffle
	return g.reshuffleInternal("所有活跃玩家手牌已空 ("+reason+")", "")
}

// reshuffleInternal 执行重洗并返回结果
func (g *LiarDiceGame) reshuffleInternal(reason string, eliminatedPlayerID string) map[string]interface{} {
	res := map[string]interface{}{"reshuffled": true, "game_ended": false, "reason": reason}
	active := g.getActivePlayerIDs()
	if len(active) == 0 {
		g.State.Status = ENDED
		res["game_ended"] = true
		return res
	}
	g.State.DiscardPile = []string{}
	for pid := range g.State.Players {
		if !g.State.Players[pid].IsEliminated {
			g.State.Players[pid].Hand = []string{}
		}
	}
	g.State.MainCard = CARD_TYPES_BASE[rand.Intn(len(CARD_TYPES_BASE))]
	g.State.Deck = g._buildDeck(len(g.getActivePlayerIDs()))
	// shuffle
	for i := range g.State.Deck {
		j := rand.Intn(i + 1)
		g.State.Deck[i], g.State.Deck[j] = g.State.Deck[j], g.State.Deck[i]
	}
	// deal
	for i := 0; i < HAND_SIZE; i++ {
		for _, p2 := range g.State.Players {
			if p2.IsEliminated {
				continue
			}
			if len(g.State.Deck) == 0 {
				break
			}
			c := g.State.Deck[0]
			g.State.Deck = g.State.Deck[1:]
			p2.Hand = append(p2.Hand, c)
		}
	}
	res["new_main_card"] = g.State.MainCard
	newHands := map[string][]string{}
	for pid, p := range g.State.Players {
		if !p.IsEliminated {
			newHands[pid] = p.Hand
		}
	}
	res["new_hands"] = newHands
	// determine starter
	start := ""
	if eliminatedPlayerID != "" {
		start = eliminatedPlayerID
	}
	// find next starter after eliminated or current index
	if start == "" && g.State.CurrentPlayerIndex >= 0 && g.State.CurrentPlayerIndex < len(g.State.TurnOrder) {
		start = g.State.TurnOrder[(g.State.CurrentPlayerIndex+1)%len(g.State.TurnOrder)]
	}
	if start == "" && len(g.State.TurnOrder) > 0 {
		start = g.State.TurnOrder[0]
	}
	// find first active from start
	if start != "" {
		idx := g.indexOf(start)
		if idx < 0 {
			idx = 0
		}
		for i := 0; i < len(g.State.TurnOrder); i++ {
			cid := g.State.TurnOrder[(idx+i)%len(g.State.TurnOrder)]
			if p := g.State.Players[cid]; p != nil && !p.IsEliminated {
				g.State.CurrentPlayerIndex = g.indexOf(cid)
				res["next_player_id"] = cid
				res["next_player_name"] = p.Name
				break
			}
		}
	}
	// if cannot determine starter -> end
	if _, ok := res["next_player_id"]; !ok {
		g.State.Status = ENDED
		res["game_ended"] = true
		winner := g.getWinnerID()
		if winner != "" {
			res["winner_id"] = winner
			res["winner_name"] = g.State.Players[winner].Name
		}
	}
	// turn order names
	names := []string{}
	for _, pid := range g.State.TurnOrder {
		if p := g.State.Players[pid]; p != nil {
			s := p.Name
			if p.IsEliminated {
				s += " (淘汰)"
			}
			names = append(names, s)
		}
	}
	res["turn_order_names"] = names
	return res
}

func (g *LiarDiceGame) getWinnerID() string {
	active := g.getActivePlayerIDs()
	if len(active) == 1 {
		return active[0]
	}
	return ""
}
