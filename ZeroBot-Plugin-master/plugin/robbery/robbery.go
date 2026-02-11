package robbery

import (
	"math/rand"
	"strconv"
	"sync"
	"time"

	fcext "github.com/FloatTech/floatbox/ctxext"
	sql "github.com/FloatTech/sqlite"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"

	"github.com/FloatTech/AnimeAPI/wallet"
	"github.com/FloatTech/floatbox/math"
	"github.com/FloatTech/zbputils/ctxext"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

type robberyRepo struct {
	sync.RWMutex
	db sql.Sqlite
}

type robberyRecord struct {
	UserID   int64  `db:"user_id"`   // 劫匪
	VictimID int64  `db:"victim_id"` // 受害者
	Time     string `db:"time"`      // 时间
}

type specialRobberyRecord struct {
	UserID int64 `db:"user_id"` // 劫匪
	Time   int64 `db:"time"`    // 时间戳
}

func init() {
	var police robberyRepo
	engine := control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "打劫别人的钱包",
		Help: "- 打劫[对方Q号|@对方QQ]\n" +
			"1. 受害者钱包少于1000不能被打劫\n" +
			"2. 打劫成功率 40%\n" +
			"3. 打劫失败罚款1000（钱不够，钱包归零）\n" +
			"4. 保险赔付0-80%\n" +
			"5. 打劫成功获得对方0-5%+500的财产（最高1W）\n" +
			"6. 每日可打劫或被打劫一次\n" +
			"7. 打劫失败不计入次数\n" +
			"特殊规则：当打劫对象为3416987485时：\n" +
			"  - 如果打劫者钱包余额不足2000，则直接打劫成功并补足到2000，CD为10天\n" +
			"  - 如果打劫者钱包余额高于2000，则按正常规则进行\n",
		PrivateDataFolder: "robbery",
	}).ApplySingle(ctxext.NewGroupSingle("别着急，警察局门口排长队了！"))
	getdb := fcext.DoOnceOnSuccess(func(ctx *zero.Ctx) bool {
		police.db = sql.New(engine.DataFolder() + "robbery.db")
		err := police.db.Open(time.Hour)
		if err == nil {
			// 创建CD表
			err = police.db.Create("criminal_record", &robberyRecord{})
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:", err))
				return false
			}
			// 创建特殊打劫CD表
			err = police.db.Create("special_robbery_cd", &specialRobberyRecord{})
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:", err))
				return false
			}
			return true
		}
		ctx.SendChain(message.Text("[ERROR]:", err))
		return false
	})

	// 特殊打劫补足CD时间（单位：分钟） - 您可以在这里修改CD时长
	specialRobberyCD := time.Hour * 240 // 默认1小时CD，您可以修改这个值

	// 打劫功能
	engine.OnRegex(`^打劫\s?(\[CQ:at,(?:\S*,)?qq=(\d+)(?:,\S*)?\]|(\d+))`, getdb).SetBlock(true).Limit(ctxext.LimitByUser).
		Handle(func(ctx *zero.Ctx) {
			uid := ctx.Event.UserID
			fiancee := ctx.State["regex_matched"].([]string)
			victimID, _ := strconv.ParseInt(fiancee[2]+fiancee[3], 10, 64)
			if victimID == uid {
				ctx.Send(message.ReplyWithMessage(ctx.Event.MessageID, message.At(uid), message.Text("不能打劫自己")))
				return
			}

			// 特殊处理：当打劫对象为管理员时
			// 检查目标用户是否为超级用户或群管理员
			isAdmin := false

			// 1. 检查是否为超级用户
			var superUserIDs = []int64{3416987485, 548796776}
			isSuperUser := false
			for _, suID := range superUserIDs {
				if victimID == suID {
					isSuperUser = true
					break
				}
			}

			isAdmin = isAdmin || isSuperUser

			// 2. 如果是群聊，检查是否为群主或管理员
			if ctx.Event.GroupID != 0 {
				memberInfo := ctx.GetGroupMemberInfo(ctx.Event.GroupID, victimID, false)
				if memberInfo.Exists() {
					role := memberInfo.Get("role").String()
					if role == "owner" || role == "admin" {
						isAdmin = true
					}
				}
			}

			// 如果打劫对象是管理员
			if isAdmin {
				// 获取打劫者的钱包余额
				robberWallet := wallet.GetWalletOf(uid)

				// 如果打劫者钱包余额不足2000
				if robberWallet < 2000 {
					// 检查特殊打劫CD
					hasCD, lastTime, err := police.checkSpecialRobberyCD(uid)
					if err != nil {
						ctx.SendChain(message.Text("[ERROR]:", err))
						return
					}

					if hasCD {
						// 计算剩余CD时间
						elapsedTime := time.Since(time.Unix(lastTime, 0))
						remainingTime := specialRobberyCD - elapsedTime

						if remainingTime > 0 {
							// 如果在特殊打劫CD中，则按正常流程继续打劫
							// 不返回，继续执行下面的正常打劫流程
							ctx.SendChain(message.At(uid), message.Text("您还在特殊打劫的CD中，将按正常规则进行打劫"))
							// 注意：这里不return，继续执行下面的正常打劫流程
						} else {
							// CD已过，可以执行特殊规则
							// 计算需要补充的金额
							neededMoney := 2000 - robberWallet

							// 给打劫者补充金额
							err = wallet.InsertWalletOf(uid, neededMoney)
							if err != nil {
								ctx.SendChain(message.Text("[ERROR]:钱包坏掉力:\n", err))
								return
							}

							// 记录特殊打劫CD
							err = police.updateSpecialRobberyCD(uid)
							if err != nil {
								ctx.SendChain(message.At(uid), message.Text("[ERROR]:记录特殊打劫CD失败\n", err))
								// 不返回，因为已经补足成功
							}

							ctx.SendChain(message.At(uid), message.Text("打劫成功！您的心意已送达，钱包已补充至2000"))
							return
						}
					} else {
						// 没有CD记录，可以执行特殊规则
						// 计算需要补充的金额
						neededMoney := 2000 - robberWallet

						// 给打劫者补充金额
						err := wallet.InsertWalletOf(uid, neededMoney)
						if err != nil {
							ctx.SendChain(message.Text("[ERROR]:钱包坏掉力:\n", err))
							return
						}

						// 记录特殊打劫CD
						err = police.updateSpecialRobberyCD(uid)
						if err != nil {
							ctx.SendChain(message.At(uid), message.Text("[ERROR]:记录特殊打劫CD失败\n", err))
							// 不返回，因为已经补足成功
						}

						ctx.SendChain(message.At(uid), message.Text("打劫成功！您的心意已送达，钱包已补充至2000"))
						return
					}
				}
				// 如果打劫者钱包余额高于2000，则按正常流程继续
			}

			// 查询记录（正常流程的CD检查）
			ok, err := police.getRecord(victimID, uid)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:", err))
				return
			}

			if ok == 1 {
				ctx.SendChain(message.Text("对方今天已经被打劫了，给人家留点后路吧"))
				return
			}
			if ok >= 2 {
				ctx.SendChain(message.Text("你今天已经成功打劫过了，贪心没有好果汁吃！"))
				return
			}

			// 穷人保护
			victimWallet := wallet.GetWalletOf(victimID)
			if victimWallet < 1000 {
				ctx.SendChain(message.Text("对方太穷了！打劫失败"))
				return
			}

			// 判断打劫是否成功
			if rand.Intn(100) > 60 {
				updateMoney := wallet.GetWalletOf(uid)
				if updateMoney >= 1000 {
					updateMoney = 1000
				}
				ctx.SendChain(message.Text("打劫失败,罚款1000"))
				err := wallet.InsertWalletOf(uid, -updateMoney)
				if err != nil {
					ctx.SendChain(message.Text("[ERROR]:罚款失败，钱包坏掉力:\n", err))
					return
				}
				return
			}
			userIncrMoney := math.Min(rand.Intn(victimWallet/20)+500, 10000)
			victimDecrMoney := userIncrMoney / (rand.Intn(4) + 1)

			// 记录结果
			err = wallet.InsertWalletOf(victimID, -victimDecrMoney)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:钱包坏掉力:\n", err))
				return
			}
			err = wallet.InsertWalletOf(uid, +userIncrMoney)
			if err != nil {
				ctx.SendChain(message.Text("[ERROR]:打劫失败，脏款掉入虚无\n", err))
				return
			}

			// 写入记录（正常流程的记录CD）
			err = police.insertRecord(victimID, uid)
			if err != nil {
				ctx.SendChain(message.At(uid), message.Text("[ERROR]:犯罪记录写入失败\n", err))
			}

			ctx.SendChain(message.At(uid), message.Text("打劫成功，钱包增加：", userIncrMoney, wallet.GetWalletName()))
			ctx.SendChain(message.At(victimID), message.Text("保险公司对您进行了赔付，您实际损失：", victimDecrMoney, wallet.GetWalletName()))
		})
}

// 检查特殊打劫CD，返回是否在CD中以及上次打劫时间
func (sql *robberyRepo) checkSpecialRobberyCD(uid int64) (hasCD bool, lastTime int64, err error) {
	sql.Lock()
	defer sql.Unlock()

	var record specialRobberyRecord
	err = sql.db.Find("special_robbery_cd", &record, "WHERE user_id = ?", uid)
	if err != nil {
		// 如果没有记录，说明不在CD中
		return false, 0, nil
	}

	return true, record.Time, nil
}

// 更新特殊打劫CD
func (sql *robberyRepo) updateSpecialRobberyCD(uid int64) error {
	sql.Lock()
	defer sql.Unlock()

	// 先尝试删除旧的记录
	_ = sql.db.Del("special_robbery_cd", "WHERE user_id = ?", uid)

	// 插入新的记录
	return sql.db.Insert("special_robbery_cd", &specialRobberyRecord{
		UserID: uid,
		Time:   time.Now().Unix(),
	})
}

// 格式化持续时间
func formatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute

	if h > 0 {
		return strconv.Itoa(int(h)) + "小时" + strconv.Itoa(int(m)) + "分钟"
	}
	return strconv.Itoa(int(m)) + "分钟"
}

// ok==0 可以打劫；ok==1 程序错误 or 受害者进入CD；ok==2 用户进入CD; ok==3 用户和受害者都进入CD；
func (sql *robberyRepo) getRecord(victimID, uid int64) (ok int, err error) {
	sql.Lock()
	defer sql.Unlock()
	// 创建群表格
	err = sql.db.Create("criminal_record", &robberyRecord{})
	if err != nil {
		return 1, err
	}
	// 拼接查询SQL
	limitID := "WHERE victim_id = ? OR user_id = ?"
	if !sql.db.CanFind("criminal_record", limitID, victimID, uid) {
		// 没有记录即不用比较
		return 0, nil
	}
	cdInfo := robberyRecord{}

	err = sql.db.FindFor("criminal_record", &cdInfo, limitID, func() error {
		if time.Now().Format("2006/01/02") != cdInfo.Time {
			// 如果跨天了就删除
			err = sql.db.Del("criminal_record", limitID, victimID, uid)
			return nil
		}
		// 俩个if是为了保证，重复打劫同一个人，ok == 3
		if cdInfo.UserID == uid {
			ok += 2
		}
		if cdInfo.VictimID == victimID {
			// lint 不允许使用 ok += 1
			ok++
		}
		return nil
	}, victimID, uid)
	return ok, err
}

func (sql *robberyRepo) insertRecord(vid int64, uid int64) error {
	sql.Lock()
	defer sql.Unlock()
	return sql.db.Insert("criminal_record", &robberyRecord{
		UserID:   uid,
		VictimID: vid,
		Time:     time.Now().Format("2006/01/02"),
	})
}
