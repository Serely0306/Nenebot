package stats

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"math"

	"github.com/fogleman/gg"
)

type RankRow struct {
	Index   int
	Name    string
	Count   int64
	Percent float64
}

type RenderRankInput struct {
	Title       string
	SessionName string
	RangeLabel  string
	TotalCount  int64
	Rows        []RankRow
}

type RenderStatsInput struct {
	Title             string
	SessionName       string
	RangeLabel        string
	RecvCount         int64
	SendCount         int64
	BotSendCount      int64
	InternalSendCount int64
	Rows              []RankRow
}

func RenderRankImage(input RenderRankInput) ([]byte, error) {
	return renderCardImage(
		input.Title,
		input.SessionName,
		input.RangeLabel,
		fmt.Sprintf("总消息数：%d", input.TotalCount),
		input.Rows,
	)
}

func RenderStatsImage(input RenderStatsInput) ([]byte, error) {
	subtitle := fmt.Sprintf("接收 %d / 发送 %d / bot %d / 内部 %d", input.RecvCount, input.SendCount, input.BotSendCount, input.InternalSendCount)
	return renderCardImage(input.Title, input.SessionName, input.RangeLabel, subtitle, input.Rows)
}

func renderCardImage(title, sessionName, rangeLabel, subtitle string, rows []RankRow) ([]byte, error) {
	const (
		width     = 1080
		headerH   = 180
		rowHeight = 76
		padding   = 40
	)

	height := headerH + padding + maxInt(1, len(rows))*rowHeight + padding
	dc := gg.NewContext(width, height)

	bgTop := color.RGBA{18, 24, 43, 255}
	bgBottom := color.RGBA{12, 39, 68, 255}
	for y := 0; y < height; y++ {
		t := float64(y) / float64(height)
		r := lerp(float64(bgTop.R), float64(bgBottom.R), t)
		g := lerp(float64(bgTop.G), float64(bgBottom.G), t)
		b := lerp(float64(bgTop.B), float64(bgBottom.B), t)
		dc.SetRGB255(int(r), int(g), int(b))
		dc.DrawLine(0, float64(y), width, float64(y))
		dc.Stroke()
	}

	dc.SetRGB255(245, 248, 255)
	if err := dc.LoadFontFace("C:/Windows/Fonts/msyh.ttc", 36); err == nil {
		dc.DrawStringAnchored(title, padding, 52, 0, 0.5)
	}
	if err := dc.LoadFontFace("C:/Windows/Fonts/msyh.ttc", 20); err == nil {
		dc.SetRGB255(210, 220, 240)
		dc.DrawStringAnchored(sessionName, padding, 96, 0, 0.5)
		dc.DrawStringAnchored(rangeLabel, padding, 126, 0, 0.5)
		dc.DrawStringAnchored(subtitle, padding, 156, 0, 0.5)
	}

	startY := float64(headerH)
	barX := float64(280)
	barW := float64(width - 420)

	for i, row := range rows {
		top := startY + float64(i*rowHeight)
		dc.SetRGBA255(255, 255, 255, 22)
		dc.DrawRoundedRectangle(float64(padding), top, float64(width-padding*2), rowHeight-12, 12)
		dc.Fill()

		if err := dc.LoadFontFace("C:/Windows/Fonts/msyh.ttc", 20); err == nil {
			dc.SetRGB255(255, 255, 255)
			dc.DrawStringAnchored(fmt.Sprintf("#%d", row.Index), float64(padding+24), top+30, 0.5, 0.5)
			dc.DrawStringAnchored(row.Name, float64(padding+90), top+30, 0, 0.5)
			dc.SetRGB255(205, 216, 238)
			dc.DrawStringAnchored(fmt.Sprintf("%d", row.Count), float64(width-180), top+30, 1, 0.5)
			dc.DrawStringAnchored(fmt.Sprintf("%.1f%%", row.Percent), float64(width-80), top+30, 1, 0.5)
		}

		progress := math.Max(0, math.Min(1, row.Percent/100))
		dc.SetRGBA255(120, 166, 255, 180)
		dc.DrawRoundedRectangle(barX, top+40, barW*progress, 12, 6)
		dc.Fill()
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
