package liarsbar

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	ctxext "github.com/FloatTech/zbputils/ctxext"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var (
	// 注册插件引擎
	engine = control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "骗子酒馆 - 俄罗斯轮盘赌",
		Help: "- 骗子酒馆\n- 加入\n- 开始\n- 状态\n- 我的手牌\n- 结束游戏\n" +
			"--------------------------------\n游戏操作 (轮到你时)\n--------------------------------\n" +
			"- 出牌 <编号> [编号...]\n打出1-3张手牌，编号对应私聊显示的序号 (示例: /出牌 1 3)\n" +
			"- 质疑 (别名: /抓, /challenge)\n认为上一家在撒谎时使用，触发判定\n" +
			"- 等待 (别名: /过, /pass)\n仅当手牌为空时可用，跳过出牌阶段\n" +
			"--------------------------------\n规则说明\n--------------------------------\n" +
			"1. 使用 /骗子酒馆 创建牌局，其他玩家发送 /加入 进场。\n" +
			"2. 游戏开始后，机器人会通过私聊发送手牌和编号。\n" +
			"3. 质疑失败或被质疑且撒谎，将触发左轮判定(概率淘汰)。\n" +
			"4. 活到最后的人获胜。" +
			"--------------------------------\n注意事项\n--------------------------------\n" +
			"1. 请确保群聊私聊权限开启(或bot具有管理权限)，并且玩家没有屏蔽私聊消息。\n" +
			"2. Joker 是万能牌，在判断声称是否属实时，它等同于当前的主牌。",
		PublicDataFolder: "LiarsBar",
	}).ApplySingle(ctxext.NewGroupSingle("酒桌上已经满了，请稍等上一局结束！"))

	// 游戏状态存储 (保持原有逻辑，不需要数据库则维持内存Map)
	games   = make(map[int64]*LiarDiceGame)
	gamesMu sync.Mutex
)

func init() {
	engine.OnFullMatchGroup([]string{"/骗子酒馆", "/liardice", "/pzjg"}).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			if gid == 0 {
				ctx.SendChain(message.Text("请在群聊中使用此命令创建游戏。"))
				return
			}
			gamesMu.Lock()
			defer gamesMu.Unlock()
			if _, ok := games[gid]; ok {
				ctx.Send("本群已有游戏，请先 /结束游戏 再创建。")
				return
			}
			games[gid] = NewGame(strconv.FormatInt(ctx.Event.UserID, 10))
			ctx.SendChain(message.Text("🍻 骗子酒馆开张！"), message.Text("➡️ /加入 参与 (需2人)。"))
		})

	engine.OnFullMatchGroup([]string{"/加入"}).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			uid := strconv.FormatInt(ctx.Event.UserID, 10)
			if gid == 0 {
				ctx.SendChain(message.Text("请在群聊中使用此命令"))
				return
			}
			gamesMu.Lock()
			g, ok := games[gid]
			gamesMu.Unlock()
			if !ok {
				ctx.Send("ℹ️ 无等待中游戏")
				return
			}
			senderName := strconv.FormatInt(ctx.Event.UserID, 10)
			if ctx.Event.Sender != nil && ctx.Event.Sender.NickName != "" {
				senderName = ctx.Event.Sender.NickName
			}
			err := g.AddPlayer(uid, senderName)
			if err != nil {
				ctx.Send(fmt.Sprint("加入失败: ", err))
				return
			}
			count := len(g.State.Players)
			ctx.SendChain(message.Text(buildJoinText(senderName, count, false)))
		})

	engine.OnFullMatchGroup([]string{"/开始", "/start"}).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			if gid == 0 {
				ctx.Send("群聊命令")
				return
			}
			gamesMu.Lock()
			g, ok := games[gid]
			gamesMu.Unlock()
			if !ok {
				ctx.Send("ℹ️ 无游戏")
				return
			}
			if len(g.State.Players) < MIN_PLAYERS {
				ctx.Send(fmt.Sprintf("❌至少需 %d 人", MIN_PLAYERS))
				return
			}
			_, err := g.StartGame()
			if err != nil {
				ctx.Send(fmt.Sprint("开始失败:", err))
				return
			}
			// send private hands, collect failures
			failures := []string{}
			for pid := range g.State.Players {
				p := g.State.Players[pid]
				if pidInt, err := strconv.ParseInt(pid, 10, 64); err == nil {
					id := ctx.SendPrivateMessage(pidInt, message.Text(fmt.Sprintf("✋ 手牌: %s\n👑 主牌: 【%s】", formatHand(p.Hand), g.State.MainCard)))
					if id == 0 {
						failures = append(failures, playerMention(pid, p.Name))
					}
				} else {
					failures = append(failures, playerMention(pid, p.Name))
				}
			}
			for _, f := range failures {
				ctx.SendChain(message.Text(fmt.Sprintf("未能向%s发送私信。", f)))
			}
			orderNames := []string{}
			for _, id := range g.State.TurnOrder {
				orderNames = append(orderNames, g.State.Players[id].Name)
			}
			first := g.GetCurrentPlayerName()
			ctx.SendChain(message.Text(buildStartText(orderNames, g.State.MainCard, first)))
		})

	engine.OnPrefixGroup([]string{"/出牌", "/play", "/打出"}).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			if gid == 0 {
				ctx.Send("群聊命令")
				return
			}
			gamesMu.Lock()
			g, ok := games[gid]
			gamesMu.Unlock()
			if !ok {
				ctx.Send("ℹ️无游戏")
				return
			}
			args := strings.TrimSpace(ctx.State["args"].(string))
			if args == "" {
				ctx.Send("请提供编号，例如: /出牌 1 2")
				return
			}
			parts := strings.Fields(args)
			idxs := []int{}
			for _, p := range parts {
				if v, err := strconv.Atoi(p); err == nil {
					idxs = append(idxs, v)
				}
			}
			uid := strconv.FormatInt(ctx.Event.UserID, 10)
			res, err := g.ProcessPlayCard(uid, idxs)
			if err != nil {
				ctx.Send(fmt.Sprint(buildErrorText(err)))
				return
			}
			// announce with full result handling
			if ge, ok := res["game_ended"].(bool); ok && ge {
				winnerID, _ := res["winner_id"].(string)
				winnerName, _ := res["winner_name"].(string)
				ctx.SendChain(message.Text(buildGameEndText(winnerID, winnerName)))
			} else if rs, ok := res["reshuffled"].(bool); ok && rs {
				// reshuffle announcement
				ctx.SendChain(message.Text(buildReshuffleText(res, g)))
				// try private new hands, notify failures
				if nh, ok2 := res["new_hands"].(map[string][]string); ok2 {
					fails := []string{}
					for pid, hand := range nh {
						if pidInt, err := strconv.ParseInt(pid, 10, 64); err == nil {
							id := ctx.SendPrivateMessage(pidInt, message.Text(fmt.Sprintf("✋ 新手牌: %s\n👑 新主牌: 【%s】", formatHand(hand), res["new_main_card"].(string))))
							if id == 0 {
								fails = append(fails, playerMention(pid, g.State.Players[pid].Name))
							}
						} else {
							fails = append(fails, playerMention(pid, g.State.Players[pid].Name))
						}
					}
					for _, f := range fails {
						ctx.SendChain(message.Text(fmt.Sprintf("未能向%s发送私信。", f)))
					}
				}
			} else {
				nextID, _ := res["next_player_id"].(string)
				nextName := ""
				nextEmpty := false
				if nextID != "" {
					if np := g.State.Players[nextID]; np != nil {
						nextName = np.Name
						nextEmpty = len(np.Hand) == 0
					}
				}
				// send private hand to player who played
				if handIf, ok := res["hand_after_play"]; ok {
					handSlice := toStringSlice(handIf)
					if pidInt, err := strconv.ParseInt(uid, 10, 64); err == nil {
						id := ctx.SendPrivateMessage(pidInt, message.Text(fmt.Sprintf("✋ 你的手牌: %s", formatHand(handSlice))))
						if id == 0 {
							ctx.SendChain(message.Text(fmt.Sprintf("未能向%s发送私信。", playerMention(uid, g.State.Players[uid].Name))))
						}
					} else {
						ctx.SendChain(message.Text(fmt.Sprintf("未能向%s发送私信。", playerMention(uid, g.State.Players[uid].Name))))
					}
				}

				// construct announcement, mention next player once
				announcement := fmt.Sprintf("%s 打出 %d 张，声称主牌【%s】。\n", g.State.Players[uid].Name, res["quantity_played"].(int), g.State.MainCard)
				if nextID != "" {
					announcement += fmt.Sprintf("轮到 %s", playerMention(nextID, nextName))
					if nextEmpty {
						announcement += " (手牌空，请 /质疑 或 /等待)"
					} else {
						announcement += "，请 /质疑 或 /出牌 <编号...>"
					}
				}
				ctx.SendChain(message.Text(announcement))
			}
		})

	engine.OnFullMatchGroup([]string{"/质疑", "/challenge", "/抓"}).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			if gid == 0 {
				ctx.Send("群聊命令")
				return
			}
			gamesMu.Lock()
			g, ok := games[gid]
			gamesMu.Unlock()
			if !ok {
				ctx.Send("ℹ️无游戏")
				return
			}
			uid := strconv.FormatInt(ctx.Event.UserID, 10)
			res, err := g.ProcessChallenge(uid)
			if err != nil {
				ctx.Send(fmt.Sprint(buildErrorText(err)))
				return
			}
			// build and send messages (use ids for mentions)
			challengerID, _ := res["challenger_id"].(string)
			challengedID, _ := res["challenged_player_id"].(string)
			loserID, _ := res["loser_id"].(string)
			challengerName, _ := res["challenger_name"].(string)
			challengedName, _ := res["challenged_player_name"].(string)
			loserName, _ := res["loser_name"].(string)
			actual := toStringSlice(res["actual_cards"])
			var chRes ChallengeResult = CH_FAILURE
			switch v := res["challenge_result"].(type) {
			case ChallengeResult:
				chRes = v
			case int:
				if v == int(CH_SUCCESS) {
					chRes = CH_SUCCESS
				}
			}
			var shotRes ShotResult = SAFE
			switch v := res["shot_outcome"].(type) {
			case ShotResult:
				shotRes = v
			case int:
				if v == int(HIT) {
					shotRes = HIT
				}
			}
			msgs := buildChallengeMessages(challengerID, challengerName, challengedID, challengedName, loserID, loserName, g.State.MainCard, actual, chRes, shotRes)
			for _, m := range msgs {
				ctx.SendChain(message.Text(m))
			}

			// then handle reshuffle / next / end
			if ge, ok := res["game_ended"].(bool); ok && ge {
				winnerID, _ := res["winner_id"].(string)
				winnerName, _ := res["winner_name"].(string)
				ctx.SendChain(message.Text(buildGameEndText(winnerID, winnerName)))
				gamesMu.Lock()
				delete(games, gid)
				gamesMu.Unlock()
			} else if rs, ok := res["reshuffled"].(bool); ok && rs {
				ctx.SendChain(message.Text(buildReshuffleText(res, g)))
				if nh, ok2 := res["new_hands"].(map[string][]string); ok2 {
					fails := []string{}
					for pid, hand := range nh {
						if pidInt, err := strconv.ParseInt(pid, 10, 64); err == nil {
							id := ctx.SendPrivateMessage(pidInt, message.Text(fmt.Sprintf("✋ 新手牌: %s\n👑 新主牌: 【%s】", formatHand(hand), res["new_main_card"].(string))))
							if id == 0 {
								fails = append(fails, playerMention(pid, g.State.Players[pid].Name))
							}
						} else {
							fails = append(fails, playerMention(pid, g.State.Players[pid].Name))
						}
					}
					for _, f := range fails {
						ctx.SendChain(message.Text(fmt.Sprintf("未能向%s发送私信。", f)))
					}
				}
			} else {
				if nid, ok := res["next_player_id"].(string); ok && nid != "" {
					nname, _ := res["next_player_name"].(string)
					nextHandEmpty := false
					if np := g.State.Players[nid]; np != nil {
						nextHandEmpty = len(np.Hand) == 0
					}
					nextMsg := fmt.Sprintf("下一轮，轮到 %s", playerMention(nid, nname))
					if nextHandEmpty {
						nextMsg += " (手牌空，请 /质疑 或 /等待)"
					} else {
						nextMsg += " 出牌。\n请使用 /出牌 <编号...>"
					}
					ctx.SendChain(message.Text(nextMsg))
				}
			}
		})

	engine.OnFullMatchGroup([]string{"/等待", "/wait", "/过"}).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			if gid == 0 {
				ctx.Send("群聊命令")
				return
			}
			gamesMu.Lock()
			g, ok := games[gid]
			gamesMu.Unlock()
			if !ok {
				ctx.Send("ℹ️无游戏")
				return
			}
			uid := strconv.FormatInt(ctx.Event.UserID, 10)
			res, err := g.ProcessWait(uid)
			if err != nil {
				ctx.Send(fmt.Sprint(buildErrorText(err)))
				return
			}
			if ge, ok := res["game_ended"].(bool); ok && ge {
				winnerID, _ := res["winner_id"].(string)
				winnerName, _ := res["winner_name"].(string)
				ctx.SendChain(message.Text(buildGameEndText(winnerID, winnerName)))
			} else if rs, ok := res["reshuffled"].(bool); ok && rs {
				ctx.SendChain(message.Text(buildReshuffleText(res, g)))
				if nh, ok2 := res["new_hands"].(map[string][]string); ok2 {
					fails := []string{}
					for pid, hand := range nh {
						if pidInt, err := strconv.ParseInt(pid, 10, 64); err == nil {
							id := ctx.SendPrivateMessage(pidInt, message.Text(fmt.Sprintf("✋ 新手牌: %s\n👑 新主牌: 【%s】", formatHand(hand), res["new_main_card"].(string))))
							if id == 0 {
								fails = append(fails, playerMention(pid, g.State.Players[pid].Name))
							}
						} else {
							fails = append(fails, playerMention(pid, g.State.Players[pid].Name))
						}
					}
					for _, f := range fails {
						ctx.SendChain(message.Text(fmt.Sprintf("未能向%s发送私信。", f)))
					}
				}
			} else {
				// normal next
				nextID, _ := res["next_player_id"].(string)
				nextName, _ := res["next_player_name"].(string)
				msg := fmt.Sprintf("%s 等待。\n", g.State.Players[uid].Name)
				if nextID != "" {
					msg += fmt.Sprintf("下一轮，轮到 %s", playerMention(nextID, nextName))
					if np := g.State.Players[nextID]; np != nil && len(np.Hand) == 0 {
						msg += " (手牌空，请 /质疑 或 /等待)"
					} else {
						msg += " 出牌。"
					}
				}
				ctx.SendChain(message.Text(msg))
			}
		})

	engine.OnFullMatchGroup([]string{"/状态", "/status", "/游戏状态"}).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			if gid == 0 {
				ctx.Send("群聊命令")
				return
			}
			gamesMu.Lock()
			g, ok := games[gid]
			gamesMu.Unlock()
			if !ok {
				ctx.Send("ℹ️无游戏")
				return
			}
			uid := strconv.FormatInt(ctx.Event.UserID, 10)
			ctx.SendChain(message.Text(buildStatusText(g, uid)))
		})

	engine.OnFullMatchGroup([]string{"/我的手牌", "/hand", "/手牌"}).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			if gid == 0 {
				ctx.Send("群聊命令")
				return
			}
			gamesMu.Lock()
			g, ok := games[gid]
			gamesMu.Unlock()
			if !ok {
				ctx.Send("ℹ️无游戏")
				return
			}
			uid := strconv.FormatInt(ctx.Event.UserID, 10)
			p := g.State.Players[uid]
			if p == nil {
				ctx.Send("ℹ️未参与")
				return
			}
			if p.IsEliminated {
				ctx.Send("☠️ 已淘汰")
				return
			}
			// send private
			if pidInt, err := strconv.ParseInt(uid, 10, 64); err == nil {
				id := ctx.SendPrivateMessage(pidInt, message.Text("✋ 你的手牌: "+formatHand(g.State.Players[uid].Hand)))
				if id != 0 {
					ctx.SendChain(message.Text("🤫 已私信"))
				} else {
					ctx.SendChain(message.Text(fmt.Sprintf("未能向%s发送私信。", playerMention(uid, g.State.Players[uid].Name))))
				}
			} else {
				ctx.Send("私信失败：ID 无效")
			}
		})

	engine.OnFullMatchGroup([]string{"/结束游戏", "/endgame", "/强制结束"}).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			if gid == 0 {
				ctx.Send("群聊命令")
				return
			}
			gamesMu.Lock()
			defer gamesMu.Unlock()
			if _, ok := games[gid]; ok {
				delete(games, gid)
				ctx.Send("🛑 游戏已被强制结束。")
			} else {
				ctx.Send("ℹ️无游戏")
			}
		})
}

func buildErrorText(err error) string { return "⚠️ 操作失败: " + err.Error() }

func toStringSlice(v interface{}) []string {
	if v == nil {
		return []string{}
	}
	res := []string{}
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		for _, x := range t {
			if s, ok := x.(string); ok {
				res = append(res, s)
			}
		}
	}
	return res
}

func shotOutcomeText(v interface{}) string {
	switch s := v.(type) {
	case ShotResult:
		if s == HIT {
			return "砰！【实弹】！被淘汰！"
		}
		if s == SAFE {
			return "咔嚓！【空弹】！安全！"
		}
	default:
	}
	return "开枪结果未知"
}
