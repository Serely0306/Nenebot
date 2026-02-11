package shadiao

import (
	"io"
	"net/http"
	"time"

	"github.com/FloatTech/zbputils/ctxext"
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

func init() {
	// 简单的请求函数
	fetchText := func(url string) (string, error) {
		client := &http.Client{
			Timeout: 15 * time.Second,
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return "", err
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		// 直接解析JSON
		// 这里需要根据实际的API响应结构来解析
		// 假设API返回的是 {"returnObj": {"content": "文本内容"}}
		content := gjson.Get(string(body), "returnObj.content").String()
		return content, nil
	}

	engine.OnFullMatch("来碗绿茶").SetBlock(true).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		text, err := fetchText(chayiURL)
		if err != nil {
			ctx.SendChain(message.Text("获取失败: ", err))
			return
		}
		ctx.SendChain(message.Reply(ctx.Event.MessageID), message.Text(text))
	})

	engine.OnFullMatch("渣我").SetBlock(true).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		text, err := fetchText(ganhaiURL)
		if err != nil {
			ctx.SendChain(message.Text("获取失败: ", err))
			return
		}
		ctx.SendChain(message.Reply(ctx.Event.MessageID), message.Text(text))
	})
}
