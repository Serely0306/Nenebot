package qqwife

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/FloatTech/gg"
	imgfactory "github.com/FloatTech/gg/factory"
)

// ==================== 星空渐变背景 ====================

// 缓存不同尺寸的渐变背景底图（不含星星）
var (
	bgCache   = make(map[[2]int]*image.RGBA)
	bgCacheMu sync.RWMutex
)

// getGradientBase 获取或生成指定尺寸的渐变底图（缓存）
func getGradientBase(w, h int) *image.RGBA {
	key := [2]int{w, h}
	bgCacheMu.RLock()
	if bg, ok := bgCache[key]; ok {
		bgCacheMu.RUnlock()
		return bg
	}
	bgCacheMu.RUnlock()

	// 生成渐变底图
	bg := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h) // 0.0(顶) → 1.0(底)
		// 顶部深蓝(15,15,50) → 底部深紫黑(10,5,30)
		r := uint8(15 - t*5)
		g := uint8(15 - t*10)
		b := uint8(50 - t*20)
		for x := 0; x < w; x++ {
			bg.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}

	bgCacheMu.Lock()
	bgCache[key] = bg
	bgCacheMu.Unlock()
	return bg
}

// drawStarryBackground 在画布上绘制星空渐变背景
// 先使用缓存的渐变底图，再随机叠加星星
func drawStarryBackground(canvas *gg.Context) {
	w := canvas.W()
	h := canvas.H()

	// 1. 绘制缓存的渐变底图
	base := getGradientBase(w, h)
	canvas.DrawImage(base, 0, 0)

	// 2. 随机绘制星星
	starCount := w * h / 8000 // 根据面积动态调整星星数量
	if starCount < 50 {
		starCount = 50
	}
	if starCount > 300 {
		starCount = 300
	}
	for i := 0; i < starCount; i++ {
		x := rand.Float64() * float64(w)
		y := rand.Float64() * float64(h)
		radius := 0.3 + rand.Float64()*1.5 // 0.3 ~ 1.8
		alpha := 0.3 + rand.Float64()*0.7  // 0.3 ~ 1.0
		canvas.SetRGBA(1, 1, 1, alpha)
		canvas.DrawCircle(x, y, radius)
		canvas.Fill()
	}

	// 3. 半透明叠加层增强文字可读性
	canvas.SetRGBA(0, 0, 0, 0.15)
	canvas.DrawRectangle(0, 0, float64(w), float64(h))
	canvas.Fill()
}

// ==================== 头像工具 ====================

// fetchAvatar 获取 QQ 头像，缩放并裁剪为圆形，失败返回 nil
func fetchAvatar(qq int64, size int) image.Image {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://q4.qlogo.cn/g?b=qq&nk=%d&s=100", qq))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	avatar, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil
	}
	// 缩放到目标尺寸
	scaled := imgfactory.Size(avatar, size, size).Image()
	// 在子画布上裁剪为圆形
	c := gg.NewContext(size, size)
	c.DrawCircle(float64(size)/2, float64(size)/2, float64(size)/2)
	c.Clip()
	c.DrawImage(scaled, 0, 0)
	return c.Image()
}

// fetchAvatarsConcurrent 并发下载多个头像
func fetchAvatarsConcurrent(qqIDs []int64, size int) []image.Image {
	avatars := make([]image.Image, len(qqIDs))
	var wg sync.WaitGroup
	for i, qq := range qqIDs {
		wg.Add(1)
		go func(idx int, qqID int64) {
			defer wg.Done()
			avatars[idx] = fetchAvatar(qqID, size)
		}(i, qq)
	}
	wg.Wait()
	return avatars
}

// truncateName 截断名字使其宽度不超过 maxW，超出部分用 "..." 替代
func truncateName(name string, maxW float64, canvas *gg.Context) string {
	w, _ := canvas.MeasureString(name)
	if w <= maxW {
		return name
	}
	runes := []rune(name)
	for i := len(runes) - 1; i > 0; i-- {
		sub := string(runes[:i]) + "..."
		sw, _ := canvas.MeasureString(sub)
		if sw <= maxW {
			return sub
		}
	}
	return "..."
}
