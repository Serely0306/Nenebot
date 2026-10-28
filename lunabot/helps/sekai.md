# 世界计划国服功能 (sekai)

以国服 Suite / MySekai 抓包功能为主，同时保留卡牌一览、查卡、歌曲列表、区域道具等常用静态查询和组卡模拟。个人数据来自已经上传的抓包，不再登录游戏或调用游戏实时 API；个人查询结果对应最近一次可用抓包，游戏内发生变化后需要重新抓取。静态查询使用国服 MasterData 和图片资源，不要求先上传个人抓包。

不加区服前缀时默认国服，也可以显式使用 `cn`，例如 `/cnmsr`。日服等其他区服查询已停用。国服缺少图片时可以按资源配置尝试日服备用图片源，账号数据与 MasterData 仍使用国服。

本文使用 `/` 表示 LunaBot 收到的命令前缀。如果所在群使用 `*` 调用 LunaBot，将示例开头的 `/` 换为 `*`。多数命令后加 `help` 可查看对应帮助。

---

## 开始使用

1. 发送 `/抓包` 查看教程，上传国服 Suite 和 MySekai 数据。
2. 发送 `/绑定 游戏ID` 保存查询账号，用 `/抓包状态`、`/msd` 确认数据来源和时间。
3. 发送 `/卡牌一览`、`/打歌进度`、`/msr` 等命令。个人查询默认使用主账号，多账号查询可加 `u2`，例如 `/msr u2`。只查公开卡牌或歌曲列表时可以跳过绑定和上传。
4. 需要自动推送时，在群内发送 `/pjsk订阅 msr`；该群还需要超级管理开启 MSR 权限。

独立的查活动、查曲、原尺寸卡面、谱面、卡池、虚拟 Live、表情及娱乐等功能，以及实时个人信息、榜线、心愿榜、送火、注册和新账号验证均已停用。卡牌查询等混合命令保留完整的常用查询分支。停用命令仍保留入口并提示改用 HarukiBot NEO：原前缀 `*` 改为 `/`，原前缀 `/` 则去掉，后面的命令和参数保持不变。

---

### 抓包上传帮助
`/抓包` `/抓包帮助` `/pjsk upload help`

获取当前抓包教程。Suite 用于卡盒、育成、成绩、记录和组卡；MySekai 用于烤森资源、蓝图、材料及生日刷新。

### 绑定和查询账号
`/绑定` `/pjsk bind` `/pjsk id`

不带参数时查看当前国服绑定；带纯数字国服游戏 ID 时保存绑定，并设为主账号。可以绑定多个账号。

绑定只检查 ID 格式并保存查询 ID，不验证账号存在或归属，也不会自动取得抓包。原有验证记录及查询权限规则继续保留，绑定本身不会授予验证权限。

- `/绑定`
- `/绑定 游戏ID`

### 解绑账号
`/解绑` `/pjsk unbind`

- `/解绑 2`：解除第二个账号的绑定。

### 设置主账号与交换顺序
`/主账号` `/设置主账号` `/pjsk set main` `/交换绑定` `/交换账号` `/pjsk swap bind`

- `/主账号 2`：将第二个账号设为默认查询账号。
- `/交换绑定 1 2`：交换两个账号的序号。
- 其他支持账号选择的查询可加 `u2`，例如 `/打歌进度 u2`。

### 默认区服
`/pjsk服务器` `/pjsk区服` `/pjsk default region`

当前仅支持 `cn`。历史设置为日服的用户在未指定区服时也使用国服，不会自动删除旧记录。

- `/pjsk服务器 cn`

### 查询抓包状态
`/抓包状态` `/抓包数据` `/pjsk check data` `/msd` `/烤森抓包数据` `/pjsk check mysekai data`

- `/抓包状态`：查询 Suite 抓包来源和上传时间。
- `/msd`：查询 MySekai 抓包来源和上传时间。

### 抓包数据来源
`/抓包模式` `/pjsk data mode`

可用模式取决于已配置的数据源与授权。读取上传数据不等于调用游戏实时 API。

- `/抓包模式 local`：仅使用本机上传的数据。
- `/抓包模式 default`：本机数据获取失败后尝试 Haruki 工具箱。
- `/抓包模式 latest`：比较可用来源，使用最新的数据。
- `/抓包模式 haruki`：仅使用 Haruki 工具箱。

### ID 与抓包可见性
`/隐藏id` `/展示id` `/pjsk hide id` `/pjsk show id` `/隐藏抓包` `/展示抓包` `/pjsk hide suite` `/pjsk show suite`

控制查询结果中的 ID 展示及原有抓包可见性规则。隐藏后需要恢复时使用对应的展示命令。

### 查询已有验证记录
`/pjsk验证列表` `/pjsk验证状态` `/pjsk verify list`

可以查看之前已验证的国服账号。新的账号验证已停用；旧验证与权限记录保留，不会因离线绑定自动新增或清除。

### 卡牌一览与查卡
`/卡牌一览` `/卡面一览` `/卡一览` `/pjsk box` `/查卡` `/查卡牌` `/卡牌列表` `/card` `/cards` `/pjsk card` `/pjsk cards` `/pjsk member`

卡牌一览保留全部国服卡牌的查询，支持原有角色、属性、稀有度、限定及排序等筛选。有 Suite 时标记持有情况，加 `box` 仅显示持有卡牌；获取时间、技能值等个人视图需要对应的 Suite 数据。通用查卡入口支持不带 `box` 的多卡列表和单卡详情。独立的原尺寸卡面及卡牌剧情入口仍然停用。

- `/卡牌一览`
- `/卡牌一览 四星 限定`
- `/卡牌一览 miku box`
- `/卡牌一览 miku 获取时间 按年`
- `/查卡 miku`
- `/查卡 123`：按卡牌 ID 查看单卡详情。
- `/查卡 miku box`

### 卡牌养成材料
`/卡牌材料统计` `/卡面材料统计` `/卡面养成` `/pjsk card material`

统计当前绑定账号持有卡牌的剩余养成材料，需要 Suite。

- `/卡牌材料统计`

### 综合抓包详情
`/盒` `/pjskdetail`

汇总账号资源、材料、编组、卡牌、加成、挑战与队长次数等 Suite 信息，不查询实时个人资料。

### 挑战与综合力加成
`/挑战信息` `/挑战进度` `/pjsk challenge info` `/加成信息` `/加成详情` `/pjsk power bonus info`

- `/挑战信息`：查看挑战 Live 的等级、成绩和奖励进度。
- `/加成信息`：查看账号的角色、组合及属性加成。

### 区域道具与材料
`/区域道具` `/pjsk area item` `/材料信息` `/pjsk material`

- `/区域道具 mmj`：按当前账号的区域道具等级查询升级材料。
- `/区域道具 蓝 all`：查看对应属性道具的全部等级。
- `/区域道具 蓝 full`：只按公开道具数据展示，与 `all` 一样无需个人抓包。
- `/材料信息`：查看持有材料和金币；可加 `all`。

### 羁绊与队长次数
`/羁绊` `/牵绊` `/pjsk bonds` `/队长次数` `/角色次数` `/pjsk leader count`

- `/羁绊`：查看账号的羁绊等级。
- `/羁绊 miku`：筛选一位角色。
- `/队长次数`：查看角色作为队长的游玩次数。

### 角色等级与 CR 任务
`/角色等级` `/角色养成` `/印章任务` `/pjskcr` `/cr任务` `/crtest` `/pjsk cr test`

- `/角色等级 miku`：查看角色养成进度。
- `/cr任务 miku`：查看角色的 CR 任务。
- `/cr任务 miku all 队长次数`：查看指定任务的全量档位。

这些入口使用个人 Suite。纯公开的 `/角色等级任务` 总览及强制刷新入口已停用。

### 打歌成绩与奖励
`/打歌进度` `/歌曲进度` `/pjsk progress` `/歌曲奖励` `/打歌奖励` `/pjsk music rewards`

- `/打歌进度 master`：查看账号的 FC / AP 等进度。
- `/歌曲奖励`：按上传成绩统计尚可取得的奖励。

### 歌曲列表与定数排行
`/歌曲列表` `/歌曲一览` `/pjsk song list` `/pjsk music list` `/难度排行` `/定数表` `/歌曲定数` `/pjsk music constant`

保留指定难度、等级和等级范围的完整国服歌曲列表及定数排行，不要求附带成绩筛选。有 Suite 时可以标记已取得的成绩；使用 `clear`、`fc`、`ap`、`未clear`、`未fc` 或 `未ap` 时按个人上传成绩筛选。

- `/歌曲列表 master`
- `/歌曲列表 expert 25 27`
- `/歌曲列表 master 未fc`
- `/歌曲列表 expert 25 27 ap`
- `/难度排行 master 32`
- `/定数表 master 未ap`

### B30 与 Another Vocal
`/b30` `/pjsk b30` `/pjsk rating` `/anvo` `/pjsk anvo`

- `/b30`：按上传的个人成绩生成 Best 30。
- `/anvo miku`：查看该角色 Another Vocal 的持有情况。

### 活动与抽卡记录
`/活动记录` `/冲榜记录` `/pjsk event record` `/抽卡记录` `/抽卡历史` `/pjsk gacha record`

仅展示已上传 Suite 中的历史记录，不实时查询排名。抓包未包含且过去未保存的记录无法补齐。

- `/活动记录`
- `/活动记录 顺序 全部`
- `/抽卡记录`
- `/抽卡记录 卡池ID`：用数字卡池 ID 筛选记录。

### 组卡与顶配模拟
`/组卡` `/活动组卡` `/pjsk deck` `/挑战组卡` `/pjsk challenge deck` `/长草组卡` `/pjsk no event deck` `/加成组卡` `/pjsk bonus deck` `/烤森组卡` `/mysekai deck`

保留活动、挑战、长草、加成与 MySekai 五类组卡。默认使用已上传账号的数据及公开资源，也可以使用国服顶配、满配、次顶配、次满配及中配模拟，不调用游戏实时 API。

- `/组卡`
- `/挑战组卡 miku`
- `/长草组卡`
- `/加成组卡`
- `/烤森组卡`
- `/组卡 顶配`
- `/组卡 次顶配`

顶配模拟使用国服当前的全卡满养成配置，不代表账号实际持有情况；次顶配使用区域道具 15 级配置。更多筛选参数可在对应命令后加 `help` 查看。

### 实效计算
`/实效` `/倍率` `/时效` `/pjsk score up`

输入五个位置的技能加分，计算协力卡组实效。只做数值计算，不需要个人抓包，也不下载图片或访问游戏实时 API。

- `/实效 100 200 300 400 500`

### MySekai 资源图
`/msr` `/msmap` `/msa` `/pjsk mysekai res`

需要上传国服 MySekai 数据，且当前群已开启 MSR 权限。普通资源及生日资源均使用抓包快照绘图。

- `/msr`
- `/msr u2`：查询第二个绑定账号。
- `/msr all`：同时显示已经采集的资源。
- `/msr force`：跳过抓包时间检查，仍然读取已有数据，不会强制向游戏获取新数据。

### MySekai 蓝图与家具
`/msb` `/pjsk mysekai blueprint` `/msf` `/家具列表` `/pjsk mysekai furniture`

- `/msb`：按账号数据查看可制作的蓝图。
- `/msb miku`：查看指定角色的家具对话进度。
- `/msf`：查看家具列表。
- `/msf 家具ID`：用数字家具 ID 查看详情，最多同时查询 10 个。

MySekai 内部使用的家具等公开资源仍然保留。

### MySekai 大门、唱片与材料
`/msg` `/msgate` `/pjsk mysekai gate` `/msm` `/mssong` `/pjsk mysekai musicrecord` `/烤森材料` `/pjsk mysekai材料`

- `/msg`：按账号进度查看大门升级材料；`/msg all` 查看全部等级。
- `/msm`：查看唱片持有情况；可加 `id` 显示编号。
- `/烤森材料`：查看材料持有情况；可加 `all`。

### MySekai 照片
`/msp` `/mysekai 照片` `/pjsk mysekai photo`

- `/msp 1`：第一张照片。
- `/msp -1`：最后一张照片。

照片索引不等于图片内容。只有抓包本身包含照片缩略图时才能展示；如果只有索引而没有缩略图，会明确提示不可用，不再通过游戏实时 API 下载照片。

### MSR 绑定 ID 限制
`/msr换绑`

将原有 MSR 查询限制 ID 切换为当前绑定的主账号。先确认主账号，再执行本命令，无需附加游戏 ID。原有权限及限制继续生效。

### 订阅普通与生日刷新推送
`/pjsk订阅` `/pjsk sub list` `/pjsk订阅 msr` `/pjsk sub msr` `/pjsk取消订阅 msr` `/pjsk unsub msr`

- `/pjsk订阅`：查看当前订阅和可订阅项目。
- `/pjsk订阅 msr`：在当前群订阅 MySekai 资源更新。
- `/pjsk取消订阅 msr`：取消该订阅。

同一 MSR 订阅同时处理普通刷新和生日刷新，不需要单独订阅生日。普通刷新在有新抓包后按原规则推送；生日刷新在抓包端成功上传完整生日快照后，按刷新事件逐次绘图推送。普通操作产生的小增量响应不等于完整生日资源快照。

订阅只决定是否推送，不能代替客户端抓包；需要 Catcher 或 iOS 模块持续捕获并上传数据。同一用户只能订阅到一个群，在其他群重新订阅会取消原群订阅。新曲、APD、虚拟 Live、MasterData 与广告奖励订阅已停用。

### 群聊 MSR 权限（超级管理）
`/pjsk开启` `/pjsk群订阅` `/pjsk开启 msr` `/pjsk关闭 msr`

- `/pjsk开启`：查看当前群开启的功能。
- `/pjsk开启 msr`：开启当前群 MSR 查询与推送权限，仅超级管理可执行。
- `/pjsk关闭 msr`：关闭当前群 MSR 权限，仅超级管理可执行。

原有国服 MSR 群权限记录继续有效，无需因本次精简重新开启。

### 账号管理记录（超级管理）
`/pjsk blacklist add` `/pjsk blacklist remove` `/pjsk user sta` `/pjsk bind history`

保留原有账号黑名单、用户统计及绑定历史管理入口，仅超级管理使用。它们不依赖游戏实时 API。

---

## 数据来源与鸣谢

- MasterData：[Sekai Viewer](https://sekai.best/)、[Haruki Master DB](https://haruki.seiunx.com/)。
- 游戏资源：[Sekai Viewer](https://sekai.best/)、[Haruki Assets](https://haruki.seiunx.com/)、[Unipjsk](https://github.com/watagashi-uni)。
- 抓包数据与工具：[Haruki 工具箱](https://haruki.seiunx.com/)。
- 组卡算法参考：[33Kit](https://3-3.dev/sekai/deck-recommend)。
- 烤森地图俯视图：[MiddleRed](https://github.com/MiddleRed)。
- 烤森家具好友码：[8823](https://github.com/kotori8823)。
- 部分代码参考：[Unibot](https://github.com/watagashi-uni/Unibot)。
- B30 定数参考：[auburnsummer](https://github.com/auburnsummer/pjskb30)。
