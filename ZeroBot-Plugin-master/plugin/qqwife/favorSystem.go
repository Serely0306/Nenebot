package qqwife

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/FloatTech/floatbox/math"

	"gopkg.in/yaml.v3"

	fcext "github.com/FloatTech/floatbox/ctxext"
	"github.com/FloatTech/imgfactory"
	sql "github.com/FloatTech/sqlite"
	control "github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"

	// 画图
	"github.com/FloatTech/floatbox/file"
	"github.com/FloatTech/gg"
	"github.com/FloatTech/zbputils/img/text"

	// 货币系统
	"github.com/FloatTech/AnimeAPI/wallet"
)

// 好感度系统
type favorability struct {
	Userinfo string // 记录用户
	Favor    int    // 好感度
}

// ==================== 礼物配置系统 ====================

// 礼物定义
type giftItem struct {
	Name       string `yaml:"name"`        // 礼物名称
	Cost       int    `yaml:"cost"`        // 价格
	MinFavor   int    `yaml:"min_favor"`   // 最小好感变化
	MaxFavor   int    `yaml:"max_favor"`   // 最大好感变化
	FailChance int    `yaml:"fail_chance"` // 不喜欢的基础概率(%)
	SuccessMsg string `yaml:"success_msg"` // 喜欢时的消息
	FailMsg    string `yaml:"fail_msg"`    // 不喜欢时的消息
}

// 运行时加载的礼物列表
var loadedGifts []giftItem

// 默认礼物列表
var defaultGiftList = []giftItem{
	{"巧克力", 30, 3, 8, 10, "ta似乎很中意这份甜意", "ta对甜食表现得有些兴致索然"},
	{"玫瑰花", 50, 5, 12, 15, "ta注视花朵时目光温柔了许多", "ta只是礼貌收下，并没有太多表情"},
	{"手工饼干", 15, 1, 5, 25, "ta很珍惜这份亲手制作的心意", "ta尝了一口，觉得味道有些平淡"},
	{"奶茶", 20, 2, 7, 10, "ta捧着杯子露出了满足的神色", "ta觉得这杯茶的味道不太合心意"},
	{"游戏点卡", 60, 5, 15, 30, "ta看起来迫不及待想去使用它", "ta最近似乎对这些娱乐提不起劲"},
	{"限定手办", 100, 8, 18, 10, "ta非常看重这份独特的收藏", "ta对此类物件并没有太大的共鸣"},
	{"演唱会门票", 150, 12, 25, 20, "ta对即将到来的行程充满期待", "ta对这种嘈杂的场合感到些许负担"},
	{"神秘礼盒", 80, 1, 30, 40, "ta被这份未知的惊喜触动了", "ta对这份惊喜感到局促"},
	{"女装", 50, 1, 10, 50, "ta似乎很中意这种风格的尝试", "ta觉得这份礼物有些不合时宜"},
	{"猫咪玩偶", 40, 4, 10, 10, "ta神情松弛地摩挲着玩偶绒毛", "ta对这种占空间的物件感到些许困扰"},
	{"手写信", 5, 2, 20, 35, "ta被字里行间的情绪深深打动", "ta读完后只是静静将其收起，未发一言"},
	{"葡萄柚", 25, 2, 6, 0, "ta很喜欢", "ta受不了这种微苦而酸涩的气息"},
}

// 懒加载礼物配置
var getGiftConfig = fcext.DoOnceOnSuccess(func(ctx *zero.Ctx) bool {
	path := engine.DataFolder() + "gifts.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		// 文件不存在，使用默认并创建文件
		loadedGifts = make([]giftItem, len(defaultGiftList))
		copy(loadedGifts, defaultGiftList)
		_ = saveGiftConfig(path)
		return true
	}
	if err = yaml.Unmarshal(data, &loadedGifts); err != nil {
		loadedGifts = make([]giftItem, len(defaultGiftList))
		copy(loadedGifts, defaultGiftList)
	}
	return true
})

func saveGiftConfig(path string) error {
	// 在文件头部添加注释说明
	header := "# QQWife 礼物配置\n# 修改后重启生效\n# fail_chance: 不喜欢的基础概率(%), 好感>50时额外+20%\n\n"
	data, err := yaml.Marshal(loadedGifts)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(header), data...), 0644)
}

// 根据名字查找礼物，找不到返回nil
func findGiftByName(name string) *giftItem {
	for i := range loadedGifts {
		if loadedGifts[i].Name == name {
			return &loadedGifts[i]
		}
	}
	return nil
}

// 检查礼物名是否合法（作为handler的前置条件）
func checkGiftName(ctx *zero.Ctx) bool {
	patternParsed := ctx.State[zero.KeyPattern].([]zero.PatternParsed)
	giftName := extractGiftName(patternParsed[0].Text())
	if giftName == "礼物" {
		return true
	}
	return findGiftByName(giftName) != nil
}

// 从 Text() 提取礼物名（兼容不同的返回格式）
func extractGiftName(texts []string) string {
	if len(texts) == 0 {
		return ""
	}
	name := texts[0]
	// 如果返回的是完整匹配 "买xxx给"，需要去掉前后缀
	name = strings.TrimPrefix(name, "买")
	name = strings.TrimSuffix(name, "给")
	return name
}

func init() {
	// ==================== 查好感度 ====================
	engine.OnMessage(zero.NewPattern(nil).Text(`^查好感度`).At().AsRule(), zero.OnlyGroup, getdb).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			patternParsed := ctx.State[zero.KeyPattern].([]zero.PatternParsed)
			fiancee, _ := strconv.ParseInt(patternParsed[1].At(), 10, 64)
			uid := ctx.Event.UserID
			favor, err := 民政局.查好感度(uid, fiancee)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:", err))
				return
			}
			ctx.SendChain(
				message.At(uid),
				message.Text("\n当前你们好感度为", favor),
			)
		})

	// ==================== 礼物系统（买礼物给=随机，买xxx给=指定） ====================
	engine.OnMessage(zero.NewPattern(nil).Text(`^买(.+)给`).At().AsRule(), zero.OnlyGroup, getdb, getGiftConfig, checkGiftName).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			uid := ctx.Event.UserID
			patternParsed := ctx.State[zero.KeyPattern].([]zero.PatternParsed)
			gay, _ := strconv.ParseInt(patternParsed[1].At(), 10, 64)
			giftName := extractGiftName(patternParsed[0].Text())

			if gay == uid {
				ctx.Send(message.ReplyWithMessage(ctx.Event.MessageID, message.At(uid), message.Text("你想给自己买什么礼物呢?")))
				return
			}
			// 获取CD
			groupInfo, err := 民政局.查看设置(gid)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:", err))
				return
			}
			ok, err := 民政局.判断CD(gid, uid, "买礼物", groupInfo.GiftCDtime)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:", err))
				return
			}
			if !ok {
				ctx.SendChain(message.Text("舔狗，你的礼物CD还没好呢。"))
				return
			}
			// 获取好感度
			favor, err := 民政局.查好感度(uid, gay)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:好感度库发生问题力\n", err))
				return
			}
			// 对接小熊饼干
			walletinfo := wallet.GetWalletOf(uid)
			if walletinfo < 1 {
				ctx.SendChain(message.Text("你钱包没钱啦！"))
				return
			}

			if giftName == "礼物" {
				// 原始逻辑：随机花费
				moneyToFavor := rand.Intn(math.Min(walletinfo, 100)) + 1
				newFavor := 1
				moodMax := 2
				if favor > 50 {
					newFavor = moneyToFavor % 10 // 礼物厌倦
				} else {
					moodMax = 5
					newFavor += rand.Intn(moneyToFavor)
				}
				mood := rand.Intn(moodMax)
				if mood == 0 {
					newFavor = -newFavor
				}
				err = wallet.InsertWalletOf(uid, -moneyToFavor)
				if err != nil {
					ctx.SendChain(message.Text("[ERROR]:钱包坏掉力:\n", err))
					return
				}
				lastfavor, err := 民政局.更新好感度(uid, gay, newFavor)
				if err != nil {
					ctx.SendChain(message.Text("[ERROR]:好感度数据库发生问题力\n", err))
					return
				}
				err = 民政局.记录CD(gid, uid, "买礼物")
				if err != nil {
					ctx.SendChain(message.At(uid), message.Text("[ERROR]:你的技能CD记录失败\n", err))
				}
				var changeStr string
				if newFavor >= 0 {
					changeStr = fmt.Sprintf("(+%d)", newFavor)
				} else {
					changeStr = fmt.Sprintf("(%d)", newFavor)
				}
				if mood == 0 {
					ctx.SendChain(message.Text("你花了", moneyToFavor, wallet.GetWalletName(), "买了一件女装送给了ta,ta很不喜欢\n好感度 ", favor, " → ", lastfavor, " ", changeStr))
				} else {
					ctx.SendChain(message.Text("你花了", moneyToFavor, wallet.GetWalletName(), "买了一件女装送给了ta,ta很喜欢\n好感度 ", favor, " → ", lastfavor, " ", changeStr))
				}
			} else {
				// 指定礼物模式
				g := findGiftByName(giftName)
				if g == nil {
					return
				}
				selectedGift := *g
				if walletinfo < selectedGift.Cost {
					ctx.SendChain(message.Text("你的", wallet.GetWalletName(), "不够买【", selectedGift.Name, "】（需要", selectedGift.Cost, "）"))
					return
				}
				// 好感度缩放机制
				favorRange := selectedGift.MaxFavor - selectedGift.MinFavor
				if favorRange < 1 {
					favorRange = 1
				}
				newFavor := selectedGift.MinFavor + rand.Intn(favorRange+1)
				actualFailChance := selectedGift.FailChance
				if favor > 50 {
					newFavor = (newFavor + 1) / 2
					actualFailChance += 20
				}
				isDislike := rand.Intn(100) < actualFailChance
				if isDislike {
					newFavor = -(rand.Intn(5) + 1)
				}
				err = wallet.InsertWalletOf(uid, -selectedGift.Cost)
				if err != nil {
					ctx.SendChain(message.Text("[ERROR]:钱包坏掉力:\n", err))
					return
				}
				lastfavor, err := 民政局.更新好感度(uid, gay, newFavor)
				if err != nil {
					ctx.SendChain(message.Text("[ERROR]:好感度数据库发生问题力\n", err))
					return
				}
				err = 民政局.记录CD(gid, uid, "买礼物")
				if err != nil {
					ctx.SendChain(message.At(uid), message.Text("[ERROR]:你的技能CD记录失败\n", err))
				}
				var resultMsg string
				if isDislike {
					resultMsg = selectedGift.FailMsg
				} else {
					resultMsg = selectedGift.SuccessMsg
				}
				var changeStr string
				if newFavor >= 0 {
					changeStr = fmt.Sprintf("(+%d)", newFavor)
				} else {
					changeStr = fmt.Sprintf("(%d)", newFavor)
				}
				ctx.SendChain(message.Text(
					"你花了", selectedGift.Cost, wallet.GetWalletName(),
					"买了", selectedGift.Name, "送给了ta\n",
					resultMsg,
					"\n好感度 ", favor, " → ", lastfavor, " ", changeStr,
				))
			}
		})

	// ==================== 好感度列表（分页+转发） ====================
	engine.OnRegex(`^好感度列表\s*(.*)$`, zero.OnlyGroup, getdb).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			uid := ctx.Event.UserID
			arg := strings.TrimSpace(ctx.State["regex_matched"].([]string)[1])

			allFavor, err := 民政局.getGroupFavorability(uid)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]: ", err))
				return
			}
			members := ctx.GetThisGroupMemberListNoCache().Array()
			memberMap := make(map[int64]bool)
			for _, m := range members {
				memberMap[m.Get("user_id").Int()] = true
			}
			var groupFavor []favorability
			for _, favor := range allFavor {
				targetID, e := strconv.ParseInt(favor.Userinfo, 10, 64)
				if e != nil || targetID == 0 {
					continue
				}
				if memberMap[targetID] {
					groupFavor = append(groupFavor, favor)
				}
			}
			if len(groupFavor) == 0 {
				ctx.SendChain(message.At(uid), message.Text("\n你在本群还没有和任何人建立好感度哦~\n试试主动一点，和群友互动吧！"))
				return
			}
			sort.Slice(groupFavor, func(i, j int) bool {
				return groupFavor[i].Favor > groupFavor[j].Favor
			})
			fontData, err := file.GetLazyData(text.BoldFontFile, control.Md5File, true)
			if err != nil {
				ctx.SendChain(message.Text("[qqwife]ERROR: ", err))
				return
			}
			sendFavorPages(ctx, groupFavor, arg, "群内好感度排行", fontData)
		})

	// ==================== 全局好感度列表（分页+转发） ====================
	engine.OnRegex(`^全局好感度列表\s*(.*)$`, zero.OnlyGroup, getdb).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			uid := ctx.Event.UserID
			arg := strings.TrimSpace(ctx.State["regex_matched"].([]string)[1])

			fianceeInfo, err := 民政局.getGroupFavorability(uid)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]: ", err))
				return
			}
			var validFavor []favorability
			for _, info := range fianceeInfo {
				if info.Userinfo == "" {
					continue
				}
				fianceID, e := strconv.ParseInt(info.Userinfo, 10, 64)
				if e != nil || fianceID == 0 {
					continue
				}
				validFavor = append(validFavor, info)
			}
			if len(validFavor) == 0 {
				ctx.SendChain(message.At(uid), message.Text("\n你还没有和任何人建立好感度哦~"))
				return
			}
			fontData, err := file.GetLazyData(text.BoldFontFile, control.Md5File, true)
			if err != nil {
				ctx.SendChain(message.Text("[qqwife]ERROR: ", err))
				return
			}
			sendFavorPages(ctx, validFavor, arg, "你的好感度排行", fontData)
		})

	// ==================== 好感度数据整理 ====================
	engine.OnFullMatch("好感度数据整理", zero.SuperUserPermission, getdb).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text("开始整理力，请稍等"))
			民政局.Lock()
			defer 民政局.Unlock()
			count, err := 民政局.db.Count("favorability")
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]: ", err))
				return
			}
			if count == 0 {
				ctx.SendChain(message.Text("[ERROR]: 不存在好感度数据."))
				return
			}
			favor := favorability{}
			delInfo := make([]string, 0, count*2)
			favorInfo := make(map[string]int, count*2)
			_ = 民政局.db.FindFor("favorability", &favor, "GROUP BY Userinfo", func() error {
				delInfo = append(delInfo, favor.Userinfo)
				userList := strings.Split(favor.Userinfo, "+")
				maxQQ, _ := strconv.ParseInt(userList[0], 10, 64)
				minQQ, _ := strconv.ParseInt(userList[1], 10, 64)
				if maxQQ > minQQ {
					favor.Userinfo = userList[0] + "+" + userList[1]
				} else {
					favor.Userinfo = userList[1] + "+" + userList[0]
				}
				score, ok := favorInfo[favor.Userinfo]
				if ok {
					if score < favor.Favor {
						favorInfo[favor.Userinfo] = favor.Favor
					}
				} else {
					favorInfo[favor.Userinfo] = favor.Favor
				}
				return nil
			})
			q, s := sql.QuerySet("WHERE Userinfo", "IN", delInfo)
			err = 民政局.db.Del("favorability", q, s...)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]: 删除好感度时发生了错误。\n错误信息:", err))
			}
			for userInfo, fav := range favorInfo {
				fi := favorability{Userinfo: userInfo, Favor: fav}
				err = 民政局.db.Insert("favorability", &fi)
				if err != nil {
					userList := strings.Split(userInfo, "+")
					uid1, _ := strconv.ParseInt(userList[0], 10, 64)
					uid2, _ := strconv.ParseInt(userList[1], 10, 64)
					ctx.SendChain(message.Text("[ERROR]: 更新", ctx.CardOrNickName(uid1), "和", ctx.CardOrNickName(uid2), "的好感度时发生了错误。\n错误信息:", err))
				}
			}
			ctx.SendChain(message.Text("清理好了哦"))
		})
}

// ==================== 好感度列表分页绘图 ====================

const favorPerPage = 15

func parseFavorPageArg(arg string) (startPage, endPage int, isForward bool) {
	arg = strings.TrimSpace(arg)
	switch {
	case arg == "":
		return 1, 1, false
	case strings.EqualFold(arg, "all") || arg == "全部":
		return 1, 9999, true
	case strings.Contains(arg, "-"):
		parts := strings.SplitN(arg, "-", 2)
		s, _ := strconv.Atoi(parts[0])
		e, _ := strconv.Atoi(parts[1])
		if s < 1 {
			s = 1
		}
		if e < s {
			e = s
		}
		return s, e, true
	default:
		p, _ := strconv.Atoi(arg)
		if p < 1 {
			p = 1
		}
		return p, p, false
	}
}

func sendFavorPages(ctx *zero.Ctx, favorData []favorability, arg, title string, fontData []byte) {
	startPage, endPage, isForward := parseFavorPageArg(arg)
	totalCount := len(favorData)
	totalPages := (totalCount + favorPerPage - 1) / favorPerPage
	if totalPages == 0 {
		totalPages = 1
	}
	if startPage > totalPages {
		startPage = totalPages
	}
	if endPage > totalPages {
		endPage = totalPages
	}
	if isForward {
		msg := make(message.Message, 0, endPage-startPage+1)
		for p := startPage; p <= endPage; p++ {
			start := (p - 1) * favorPerPage
			end := start + favorPerPage
			if end > totalCount {
				end = totalCount
			}
			imgData, err := drawFavorPage(favorData[start:end], p, totalPages, totalCount, title, ctx, fontData)
			if err != nil {
				ctx.SendChain(message.Text("[qqwife]ERROR: ", err))
				return
			}
			msg = append(msg, ctxext.FakeSenderForwardNode(ctx, message.ImageBytes(imgData)))
		}
		ctx.SendGroupForwardMessage(ctx.Event.GroupID, msg)
	} else {
		start := (startPage - 1) * favorPerPage
		end := start + favorPerPage
		if end > totalCount {
			end = totalCount
		}
		imgData, err := drawFavorPage(favorData[start:end], startPage, totalPages, totalCount, title, ctx, fontData)
		if err != nil {
			ctx.SendChain(message.Text("[qqwife]ERROR: ", err))
			return
		}
		ctx.SendChain(message.ImageBytes(imgData))
	}
}

func drawFavorPage(items []favorability, page, totalPages, totalCount int, title string, ctx *zero.Ctx, fontData []byte) ([]byte, error) {
	number := len(items)
	if number == 0 {
		return nil, errors.New("当前页没有数据")
	}
	fontSize := 50.0
	canvas := gg.NewContext(1150, int(270+(50+70)*float64(number)))
	canvas.SetRGB(1, 1, 1)
	canvas.Clear()

	canvas.SetRGB(0, 0, 0)
	if err := canvas.ParseFontFace(fontData, fontSize*2); err != nil {
		return nil, err
	}
	sl, _ := canvas.MeasureString(title)
	canvas.DrawString(title, (1100-sl)/2, 100)
	canvas.DrawString("————————————————————", 0, 160)

	if err := canvas.ParseFontFace(fontData, fontSize); err != nil {
		return nil, err
	}
	_, h := canvas.MeasureString("焯")

	for i, info := range items {
		targetID, _ := strconv.ParseInt(info.Userinfo, 10, 64)
		userName := ctx.CardOrNickName(targetID)

		canvas.SetRGB255(0, 0, 0)
		canvas.DrawString(userName+"("+info.Userinfo+")", 10, float64(180+(50+70)*i))
		canvas.DrawString(strconv.Itoa(info.Favor), 1020, float64(180+60+(50+70)*i))

		canvas.DrawRectangle(10, float64(180+60+(50+70)*i)-h/2, 1000, 50)
		canvas.SetRGB255(150, 150, 150)
		canvas.Fill()

		barWidth := float64(info.Favor) * 10
		if barWidth > 1000 {
			barWidth = 1000
		}
		canvas.DrawRectangle(10, float64(180+60+(50+70)*i)-h/2, barWidth, 50)

		switch {
		case info.Favor >= 80:
			canvas.SetRGB255(255, 105, 180) // 粉色
		case info.Favor >= 50:
			canvas.SetRGB255(255, 165, 0) // 橙色
		default:
			canvas.SetRGB255(231, 27, 100) // 红色
		}
		canvas.Fill()
	}

	if err := canvas.ParseFontFace(fontData, fontSize*0.7); err != nil {
		return nil, err
	}
	canvas.SetRGB255(120, 120, 120)
	pageInfo := fmt.Sprintf("第 %d/%d 页 · 共 %d 人", page, totalPages, totalCount)
	pw, _ := canvas.MeasureString(pageInfo)
	canvas.DrawString(pageInfo, (1100-pw)/2, float64(180+(50+70)*number)+20)

	return imgfactory.ToBytes(canvas.Image())
}

// ==================== 好感度查询与更新 ====================

func (sql *婚姻登记) 查好感度(uid, target int64) (int, error) {
	sql.Lock()
	defer sql.Unlock()
	err := sql.db.Create("favorability", &favorability{})
	if err != nil {
		return 0, err
	}
	info := favorability{}
	if uid > target {
		userinfo := strconv.FormatInt(uid, 10) + "+" + strconv.FormatInt(target, 10)
		err = sql.db.Find("favorability", &info, "WHERE Userinfo = ?", userinfo)
		if err != nil {
			_ = sql.db.Find("favorability", &info, "WHERE Userinfo glob ?", "*"+userinfo+"*")
		}
	} else {
		userinfo := strconv.FormatInt(target, 10) + "+" + strconv.FormatInt(uid, 10)
		err = sql.db.Find("favorability", &info, "WHERE Userinfo = ?", userinfo)
		if err != nil {
			_ = sql.db.Find("favorability", &info, "WHERE Userinfo glob ?", "*"+userinfo+"*")
		}
	}
	return info.Favor, nil
}

type favorList []favorability

func (s favorList) Len() int           { return len(s) }
func (s favorList) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s favorList) Less(i, j int) bool { return s[i].Favor > s[j].Favor }

func (sql *婚姻登记) getGroupFavorability(uid int64) (list favorList, err error) {
	uidStr := strconv.FormatInt(uid, 10)
	sql.RLock()
	defer sql.RUnlock()

	info := favorability{}
	err = sql.db.FindFor("favorability", &info, "WHERE Userinfo glob ?", func() error {
		var target string
		userList := strings.Split(info.Userinfo, "+")
		switch {
		case len(userList) == 0:
			return errors.New("好感度系统数据存在错误")
		case userList[0] == uidStr:
			target = userList[1]
		default:
			target = userList[0]
		}
		if target == "" || target == "0" {
			return nil
		}
		list = append(list, favorability{Userinfo: target, Favor: info.Favor})
		return nil
	}, "*"+uidStr+"*")

	sort.Sort(list)
	return
}

// 更新好感度（上限100，下限0）
func (sql *婚姻登记) 更新好感度(uid, target int64, score int) (favor int, err error) {
	sql.Lock()
	defer sql.Unlock()
	err = sql.db.Create("favorability", &favorability{})
	if err != nil {
		return
	}
	info := favorability{}
	uidstr := strconv.FormatInt(uid, 10)
	targstr := strconv.FormatInt(target, 10)
	if uid > target {
		info.Userinfo = uidstr + "+" + targstr
		err = sql.db.Find("favorability", &info, "WHERE Userinfo = ?", info.Userinfo)
	} else {
		info.Userinfo = targstr + "+" + uidstr
		err = sql.db.Find("favorability", &info, "WHERE Userinfo = ?", info.Userinfo)
	}
	if err != nil {
		err = sql.db.Find("favorability", &info, "WHERE Userinfo glob ?", "*"+targstr+"+"+uidstr+"*")
		if err == nil {
			err = 民政局.db.Del("favorability", "WHERE Userinfo = ?", info.Userinfo)
		}
	}
	info.Favor += score
	if info.Favor > 100 {
		info.Favor = 100
	} else if info.Favor < 0 {
		info.Favor = 0
	}
	err = sql.db.Insert("favorability", &info)
	return info.Favor, err
}
