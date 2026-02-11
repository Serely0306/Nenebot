// Package tiangou 舔狗日记（TXT版）
package tiangou

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	fcext "github.com/FloatTech/floatbox/ctxext"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var (
	entries []string
	mu      sync.RWMutex
	rng     = rand.New(rand.NewSource(time.Now().UnixNano()))
)

func loadTxt(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// 防止单行过长导致 scanner 报错：默认 64K，这里放大到 1MB
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	var out []string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tiangou.txt 内容为空（或全是空行）")
	}
	return out, nil
}

func init() {
	en := control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "舔狗日记",
		Help:             "- 舔狗日记",
		PublicDataFolder: "Tiangou",
	})

	en.OnFullMatch("舔狗日记", fcext.DoOnceOnSuccess(
		func(ctx *zero.Ctx) bool {
			// 1) 拉取公共数据：从 tiangou.db 改为 tiangou.txt
			_, err := en.GetLazyData("tiangou.txt", false)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return false
			}

			// 2) 读取本地数据文件
			fp := filepath.Join(en.DataFolder(), "tiangou.txt")
			list, err := loadTxt(fp)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return false
			}

			mu.Lock()
			entries = list
			mu.Unlock()

			logrus.Infoln("[tiangou]加载", len(list), "条舔狗日记（TXT）")
			return true
		},
	)).SetBlock(true).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		mu.RLock()
		n := len(entries)
		if n == 0 {
			mu.RUnlock()
			ctx.SendChain(message.Text("ERROR: tiangou.txt 未加载或为空"))
			return
		}
		i := rng.Intn(n)
		text := entries[i]
		mu.RUnlock()

		ctx.SendChain(message.Text(text))
	})
}
