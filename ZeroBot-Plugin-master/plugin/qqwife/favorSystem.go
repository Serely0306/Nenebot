package qqwife

import (
	"errors"
	"math/rand"
	"sort"
	"strconv"
	"strings"

	"github.com/FloatTech/floatbox/math"
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

func init() {
	// 好感度系统
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
			// 输出结果
			ctx.SendChain(
				message.At(uid),
				message.Text("\n当前你们好感度为", favor),
			)
		})

	// 礼物系统
	engine.OnMessage(zero.NewPattern(nil).Text(`^买礼物给|买葡萄柚给`).At().AsRule(), zero.OnlyGroup, getdb).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			uid := ctx.Event.UserID
			patternParsed := ctx.State[zero.KeyPattern].([]zero.PatternParsed)
			gay, _ := strconv.ParseInt(patternParsed[1].At(), 10, 64)
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
				ctx.SendChain(message.Text("舔狗，今天你已经送过礼物了。"))
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
			moneyToFavor := rand.Intn(math.Min(walletinfo, 100)) + 1
			// 计算钱对应的好感值
			newFavor := 1
			moodMax := 2
			if favor > 50 {
				newFavor = moneyToFavor % 10 // 礼物厌倦
			} else {
				moodMax = 5
				newFavor += rand.Intn(moneyToFavor)
			}
			// 随机对方心情
			mood := rand.Intn(moodMax)
			if mood == 0 {
				newFavor = -newFavor
			}
			// 记录结果
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
			// 写入CD
			err = 民政局.记录CD(gid, uid, "买礼物")
			if err != nil {
				ctx.SendChain(message.At(uid), message.Text("[ERROR]:你的技能CD记录失败\n", err))
			}
			// 输出结果 - 修改这里，显示从多少增加/减少至多少
			if mood == 0 {
				// 计算实际变化量（newFavor是负数）
				ctx.SendChain(message.Text("你花了", moneyToFavor, wallet.GetWalletName(), "买了一件女装送给了ta,ta很不喜欢,好感度从", favor, "减少至", lastfavor))
			} else {
				ctx.SendChain(message.Text("你花了", moneyToFavor, wallet.GetWalletName(), "买了一件女装送给了ta,ta很喜欢,好感度从", favor, "增加至", lastfavor))
			}
		})

	// 群内好感度列表（只显示当前群成员）
	engine.OnFullMatch("好感度列表", zero.OnlyGroup, getdb).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			// gid := ctx.Event.GroupID
			uid := ctx.Event.UserID

			// 获取用户的所有好感度记录
			allFavor, err := 民政局.getGroupFavorability(uid)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:ERROR: ", err))
				return
			}

			// 获取当前群成员列表
			members := ctx.GetThisGroupMemberListNoCache().Array()
			memberMap := make(map[int64]bool)
			for _, m := range members {
				memberID := m.Get("user_id").Int()
				memberMap[memberID] = true
			}

			// 过滤只显示当前群成员，并限制最多显示10个
			var groupFavor []favorability
			for _, favor := range allFavor {
				targetID, err := strconv.ParseInt(favor.Userinfo, 10, 64)
				if err != nil || targetID == 0 {
					continue
				}

				// 只显示在当前群的用户
				if memberMap[targetID] {
					groupFavor = append(groupFavor, favor)
				}

				// 最多显示10个
				if len(groupFavor) >= 10 {
					break
				}
			}

			// 如果没有找到当前群的好感度记录
			if len(groupFavor) == 0 {
				ctx.SendChain(
					message.At(uid),
					message.Text("\n你在本群还没有和任何人建立好感度哦~"),
					message.Text("\n试试主动一点，和群友互动吧！"),
				)
				return
			}

			/***********设置图片的大小和底色***********/
			number := len(groupFavor)
			fontSize := 50.0
			canvas := gg.NewContext(1150, int(170+(50+70)*float64(number)))
			canvas.SetRGB(1, 1, 1) // 白色
			canvas.Clear()

			/***********下载字体***********/
			data, err := file.GetLazyData(text.BoldFontFile, control.Md5File, true)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:ERROR: ", err))
				return
			}

			/***********设置字体颜色为黑色***********/
			canvas.SetRGB(0, 0, 0)
			/***********设置字体大小,并获取字体高度用来定位***********/
			if err = canvas.ParseFontFace(data, fontSize*2); err != nil {
				ctx.SendChain(message.Text("[ERROR]:ERROR: ", err))
				return
			}

			sl, h := canvas.MeasureString("群内好感度排行")
			/***********绘制标题***********/
			canvas.DrawString("群内好感度排行", (1100-sl)/2, 100) // 放置在中间位置
			canvas.DrawString("————————————————————", 0, 160)

			/***********设置字体大小,并获取字体高度用来定位***********/
			if err = canvas.ParseFontFace(data, fontSize); err != nil {
				ctx.SendChain(message.Text("[ERROR]:ERROR: ", err))
				return
			}

			// 按照好感度排序（虽然getGroupFavorability已经排序了，但再排一次确保）
			sort.Slice(groupFavor, func(i, j int) bool {
				return groupFavor[i].Favor > groupFavor[j].Favor
			})

			// 绘制每个好感度条目
			for i, info := range groupFavor {
				targetID, _ := strconv.ParseInt(info.Userinfo, 10, 64)
				userName := ctx.CardOrNickName(targetID)

				// 绘制用户信息
				canvas.SetRGB255(0, 0, 0)
				canvas.DrawString(userName+"("+info.Userinfo+")", 10, float64(180+(50+70)*i))

				// 绘制好感度数值
				canvas.DrawString(strconv.Itoa(info.Favor), 1020, float64(180+60+(50+70)*i))

				// 绘制背景条
				canvas.DrawRectangle(10, float64(180+60+(50+70)*i)-h/2, 1000, 50)
				canvas.SetRGB255(150, 150, 150)
				canvas.Fill()

				// 绘制好感度条
				canvas.SetRGB255(0, 0, 0)
				barWidth := float64(info.Favor) * 10 // 好感度*10为宽度
				if barWidth > 1000 {
					barWidth = 1000
				}
				canvas.DrawRectangle(10, float64(180+60+(50+70)*i)-h/2, barWidth, 50)

				// 根据好感度设置不同颜色
				if info.Favor >= 80 {
					canvas.SetRGB255(255, 105, 180) // 粉色 - 高好感度
				} else if info.Favor >= 50 {
					canvas.SetRGB255(255, 165, 0) // 橙色 - 中等好感度
				} else {
					canvas.SetRGB255(231, 27, 100) // 红色 - 低好感度
				}
				canvas.Fill()
			}

			// 生成图片
			data, err = imgfactory.ToBytes(canvas.Image())
			if err != nil {
				ctx.SendChain(message.Text("[qqwife]ERROR: ", err))
				return
			}
			ctx.SendChain(message.ImageBytes(data))
		})

	// 全局好感度列表（显示所有群的好感度）
	engine.OnFullMatch("全局好感度列表", zero.OnlyGroup, getdb).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			uid := ctx.Event.UserID
			fianceeInfo, err := 民政局.getGroupFavorability(uid)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:ERROR: ", err))
				return
			}

			// 限制显示数量
			number := len(fianceeInfo)
			if number > 10 {
				number = 10
			}

			/***********设置图片的大小和底色***********/
			fontSize := 50.0
			canvas := gg.NewContext(1150, int(170+(50+70)*float64(number)))
			canvas.SetRGB(1, 1, 1) // 白色
			canvas.Clear()

			/***********下载字体***********/
			data, err := file.GetLazyData(text.BoldFontFile, control.Md5File, true)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:ERROR: ", err))
				return
			}

			/***********设置字体颜色为黑色***********/
			canvas.SetRGB(0, 0, 0)
			/***********设置字体大小,并获取字体高度用来定位***********/
			if err = canvas.ParseFontFace(data, fontSize*2); err != nil {
				ctx.SendChain(message.Text("[ERROR]:ERROR: ", err))
				return
			}

			sl, h := canvas.MeasureString("你的好感度排行")
			/***********绘制标题***********/
			canvas.DrawString("你的好感度排行", (1100-sl)/2, 100) // 放置在中间位置
			canvas.DrawString("————————————————————", 0, 160)

			/***********设置字体大小,并获取字体高度用来定位***********/
			if err = canvas.ParseFontFace(data, fontSize); err != nil {
				ctx.SendChain(message.Text("[ERROR]:ERROR: ", err))
				return
			}

			i := 0
			for _, info := range fianceeInfo {
				if i >= 10 { // 只显示前10个
					break
				}

				if info.Userinfo == "" {
					continue
				}

				fianceID, err := strconv.ParseInt(info.Userinfo, 10, 64)
				if err != nil || fianceID == 0 {
					continue
				}

				userName := ctx.CardOrNickName(fianceID)
				canvas.SetRGB255(0, 0, 0)
				canvas.DrawString(userName+"("+info.Userinfo+")", 10, float64(180+(50+70)*i))
				canvas.DrawString(strconv.Itoa(info.Favor), 1020, float64(180+60+(50+70)*i))
				canvas.DrawRectangle(10, float64(180+60+(50+70)*i)-h/2, 1000, 50)
				canvas.SetRGB255(150, 150, 150)
				canvas.Fill()
				canvas.SetRGB255(0, 0, 0)

				// 绘制好感度条
				barWidth := float64(info.Favor) * 10
				if barWidth > 1000 {
					barWidth = 1000
				}
				canvas.DrawRectangle(10, float64(180+60+(50+70)*i)-h/2, barWidth, 50)

				// 根据好感度设置颜色
				if info.Favor >= 80 {
					canvas.SetRGB255(255, 105, 180) // 粉色
				} else if info.Favor >= 50 {
					canvas.SetRGB255(255, 165, 0) // 橙色
				} else {
					canvas.SetRGB255(231, 27, 100) // 红色
				}
				canvas.Fill()
				i++
			}

			// 添加底部信息
			canvas.SetRGB255(100, 100, 100)
			if err = canvas.ParseFontFace(data, fontSize*0.6); err != nil {
				ctx.SendChain(message.Text("[ERROR]:ERROR: ", err))
				return
			}

			data, err = imgfactory.ToBytes(canvas.Image())
			if err != nil {
				ctx.SendChain(message.Text("[qqwife]ERROR: ", err))
				return
			}
			ctx.SendChain(message.ImageBytes(data))
		})

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
				// 解析旧数据
				userList := strings.Split(favor.Userinfo, "+")
				maxQQ, _ := strconv.ParseInt(userList[0], 10, 64)
				minQQ, _ := strconv.ParseInt(userList[1], 10, 64)
				if maxQQ > minQQ {
					favor.Userinfo = userList[0] + "+" + userList[1]
				} else {
					favor.Userinfo = userList[1] + "+" + userList[0]
				}
				// 判断是否是重复的
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
			// 删除旧数据
			q, s := sql.QuerySet("WHERE Userinfo", "IN", delInfo)
			err = 民政局.db.Del("favorability", q, s...)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]: 删除好感度时发生了错误。\n错误信息:", err))
			}
			for userInfo, favor := range favorInfo {
				favorInfo := favorability{
					Userinfo: userInfo,
					Favor:    favor,
				}
				err = 民政局.db.Insert("favorability", &favorInfo)
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

// 获取好感度数据组
type favorList []favorability

func (s favorList) Len() int {
	return len(s)
}
func (s favorList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s favorList) Less(i, j int) bool {
	return s[i].Favor > s[j].Favor
}

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

		// 跳过无效数据
		if target == "" || target == "0" {
			return nil
		}

		list = append(list, favorability{
			Userinfo: target,
			Favor:    info.Favor,
		})
		return nil
	}, "*"+uidStr+"*")

	// 按好感度从高到低排序
	sort.Sort(list)
	return
}

// 设置好感度 正增负减
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
		if err == nil { // 如果旧数据存在就删除旧数据
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
