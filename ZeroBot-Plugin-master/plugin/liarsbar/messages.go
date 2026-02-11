package liarsbar

import (
	"fmt"
	"strings"
)

func formatHand(hand []string) string {
	if len(hand) == 0 {
		return "无"
	}
	parts := make([]string, len(hand))
	for i, c := range hand {
		parts[i] = fmt.Sprintf("[%d:%s]", i+1, c)
	}
	return strings.Join(parts, " ")
}

func buildJoinText(playerName string, count int, isAI bool) string {
	if isAI {
		return fmt.Sprintf("🤖 %s 已添加！当前 %d 人。", playerName, count)
	}
	return fmt.Sprintf("✅ %s 已加入！当前 %d 人。", playerName, count)
}

func buildStartText(turnNames []string, mainCard string, firstName string) string {
	return fmt.Sprintf("🎉 游戏开始！%d 人参与。\n👑 本轮主牌: 【%s】\n📜 顺序: %s\n\n👉 请第一位 @%s(%s) 出牌！\n(/出牌 编号 [编号...])", len(turnNames), mainCard, strings.Join(turnNames, ", "), firstName, firstName)
}

func buildPlayAnnouncement(playerName string, quantity int, mainCard string, nextName string, nextEmpty bool) string {
	s := fmt.Sprintf("%s 打出 %d 张，声称主牌【%s】。\n", playerName, quantity, mainCard)
	if nextName != "" {
		if nextEmpty {
			s += fmt.Sprintf("轮到 %s (手牌空，请 /质疑 或 /等待)", nextName)
		} else {
			s += fmt.Sprintf("轮到 %s，请 /质疑 或 /出牌 <编号...>", nextName)
		}
	}
	return s
}

func buildChallengeMessages(challengerID, challengerName, challengedID, challengedName, loserID, loserName, mainCard string, actualCards []string, chRes ChallengeResult, shotRes ShotResult) []string {
	msgs := []string{}
	// reveal
	reveal := fmt.Sprintf("%s 质疑 %s 的 %d 张 👑%s！\n亮牌: 【%s】", playerMention(challengerID, challengerName), playerMention(challengedID, challengedName), len(actualCards), mainCard, strings.Join(actualCards, ", "))
	msgs = append(msgs, reveal)

	// challenge outcome
	if chRes == CH_SUCCESS {
		msgs = append(msgs, fmt.Sprintf("质疑成功！%s 没有完全打出主牌/Joker。", challengedName))
	} else {
		msgs = append(msgs, fmt.Sprintf("质疑失败！%s 确实是主牌/Joker。", challengedName))
	}

	// who shoots next
	msgs = append(msgs, fmt.Sprintf("轮到 %s 开枪！", playerMention(loserID, loserName)))

	// shot result line
	switch shotRes {
	case SAFE:
		msgs = append(msgs, fmt.Sprintf("%s 扣动扳机... 咔嚓！【空弹】！安全！", loserName))
	case HIT:
		msgs = append(msgs, fmt.Sprintf("%s 扣动扳机... 砰！【实弹】！%s 被淘汰！", loserName, playerMention(loserID, loserName)))
	case ALREADY_ELIMINATED:
		msgs = append(msgs, fmt.Sprintf("ℹ️ %s 已被淘汰。", loserName))
	default:
		msgs = append(msgs, "开枪结果未知")
	}
	return msgs
}

func buildStatusText(g *LiarDiceGame, requester string) string {
	sb := &strings.Builder{}
	gs := g.State
	sb.WriteString("🎲 骗子酒馆状态\n")
	if gs.Status == WAITING {
		sb.WriteString("状态: WAITING\n玩家:\n")
		for _, p := range gs.Players {
			sb.WriteString("- " + p.Name + "\n")
		}
		sb.WriteString("\n➡️ /加入 参与 (需 2 人)\n➡️ 发起者可 /开始")
		return sb.String()
	}
	sb.WriteString(fmt.Sprintf("状态: PLAYING\n👑 主牌: 【%s】\n", gs.MainCard))
	sb.WriteString("顺序: ")
	order := []string{}
	for _, id := range gs.TurnOrder {
		if p := gs.Players[id]; p != nil {
			order = append(order, p.Name)
		}
	}
	sb.WriteString(strings.Join(order, ", ") + "\n")
	cur := gs.TurnOrder[gs.CurrentPlayerIndex]
	sb.WriteString("当前轮到: ")
	if p := gs.Players[cur]; p != nil {
		sb.WriteString(p.Name + "\n")
	} else {
		sb.WriteString("未知\n")
	}
	sb.WriteString("--------------------\n玩家状态:\n")
	for _, id := range gs.TurnOrder {
		p := gs.Players[id]
		if p == nil {
			continue
		}
		status := "😀"
		if p.IsEliminated {
			status = "☠️"
		}
		sb.WriteString(fmt.Sprintf("%s %s: %d张\n", status, p.Name, len(p.Hand)))
	}
	if requester != "" {
		if rp := gs.Players[requester]; rp != nil && !rp.IsEliminated {
			sb.WriteString("--------------------\n你的手牌: " + formatHand(rp.Hand))
		}
	}
	return sb.String()
}

func playerMention(id, name string) string {
	// return plain @Name(Name) format to match python behaviour
	if name == "" {
		return "@UNKNOWN"
	}
	return fmt.Sprintf("@%s(%s)", name, name)
}

func buildReshuffleText(res map[string]interface{}, g *LiarDiceGame) string {
	reason := "洗牌"
	if v, ok := res["reason"].(string); ok && v != "" {
		reason = v
	}
	main := ""
	if v, ok := res["new_main_card"].(string); ok {
		main = v
	}
	orderNames := []string{}
	if v, ok := res["turn_order_names"].([]string); ok {
		orderNames = v
	} else {
		for _, id := range g.State.TurnOrder {
			if p := g.State.Players[id]; p != nil {
				s := p.Name
				if p.IsEliminated {
					s += " (淘汰)"
				}
				orderNames = append(orderNames, s)
			}
		}
	}
	nextID, _ := res["next_player_id"].(string)
	nextName, _ := res["next_player_name"].(string)
	out := fmt.Sprintf("🔄 %s！重新洗牌发牌！\n👑 新主牌: 【%s】\n📜 顺序: %s\n(新手牌已尝试私信发送)\n👉 轮到 %s 出牌。", reason, main, strings.Join(orderNames, ", "), playerMention(nextID, nextName))
	return out
}

func buildGameEndText(winnerID, winnerName string) string {
	if winnerID == "" {
		return "🎉 游戏结束！没有幸存者。"
	}
	// mention if numeric
	mention := playerMention(winnerID, winnerName)
	return fmt.Sprintf("🎉 游戏结束！最后的胜者是: %s！", mention)
}
