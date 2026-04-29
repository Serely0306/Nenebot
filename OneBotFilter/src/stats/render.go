package stats

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/fogleman/gg"
	gemoji "github.com/gogpu/gg/text/emoji"
	"golang.org/x/image/font"
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

type fontPair struct {
	primary  font.Face
	fallback font.Face
	size     float64
	emojiDir string
}

type fontBundle struct {
	title fontPair
	body  fontPair
	meta  fontPair
}

func RenderRankImage(fontPath string, input RenderRankInput) ([]byte, error) {
	return renderCardImage(
		fontPath,
		input.Title,
		input.SessionName,
		input.RangeLabel,
		fmt.Sprintf("总消息数：%d", input.TotalCount),
		input.Rows,
	)
}

func RenderStatsImage(fontPath string, input RenderStatsInput) ([]byte, error) {
	subtitle := fmt.Sprintf("接收 %d / 发送 %d / bot %d / 内部 %d", input.RecvCount, input.SendCount, input.BotSendCount, input.InternalSendCount)
	return renderCardImage(fontPath, input.Title, input.SessionName, input.RangeLabel, subtitle, input.Rows)
}

func renderCardImage(fontPath, title, sessionName, rangeLabel, subtitle string, rows []RankRow) ([]byte, error) {
	const (
		width      = 1080
		headerH    = 190
		rowHeight  = 112
		padding    = 40
		cardRadius = 14
	)

	fonts, err := loadFonts(fontPath)
	if err != nil {
		return nil, err
	}

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

	setColor(dc, 245, 248, 255)
	drawTextLeft(dc, fonts.title, title, float64(padding), 52)
	setColor(dc, 210, 220, 240)
	drawTextLeft(dc, fonts.body, sessionName, float64(padding), 96)
	drawTextLeft(dc, fonts.meta, rangeLabel, float64(padding), 126)
	drawTextLeft(dc, fonts.meta, subtitle, float64(padding), 156)

	startY := float64(headerH)
	cardWidth := float64(width - padding*2)
	barX := float64(padding + 90)
	barW := float64(width - padding*2 - 180)
	nameMaxWidth := float64(width - padding*2 - 320)

	for i, row := range rows {
		top := startY + float64(i*rowHeight)
		dc.SetRGBA255(255, 255, 255, 22)
		dc.DrawRoundedRectangle(float64(padding), top, cardWidth, rowHeight-14, cardRadius)
		dc.Fill()

		indexY := top + 30
		nameY := top + 32
		barY := top + 66

		setColor(dc, 255, 255, 255)
		drawTextLeft(dc, fonts.body, fmt.Sprintf("#%d", row.Index), float64(padding+24), indexY)
		nameText := ellipsizeText(dc, fonts.body, row.Name, nameMaxWidth)
		drawTextLeft(dc, fonts.body, nameText, float64(padding+90), nameY)

		setColor(dc, 205, 216, 238)
		drawTextRight(dc, fonts.meta, fmt.Sprintf("%d", row.Count), float64(width-180), nameY)
		drawTextRight(dc, fonts.meta, fmt.Sprintf("%.1f%%", row.Percent), float64(width-80), nameY)

		dc.SetRGBA255(255, 255, 255, 26)
		dc.DrawRoundedRectangle(barX, barY, barW, 12, 6)
		dc.Fill()

		progress := math.Max(0, math.Min(1, row.Percent/100))
		dc.SetRGBA255(120, 166, 255, 210)
		dc.DrawRoundedRectangle(barX, barY, math.Max(10, barW*progress), 12, 6)
		dc.Fill()
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func loadFonts(fontPath string) (fontBundle, error) {
	title, err := loadFontPair(fontPath, 36)
	if err != nil {
		return fontBundle{}, fmt.Errorf("加载统计标题字体失败: %w", err)
	}
	body, err := loadFontPair(fontPath, 22)
	if err != nil {
		return fontBundle{}, fmt.Errorf("加载统计正文字体失败: %w", err)
	}
	meta, err := loadFontPair(fontPath, 18)
	if err != nil {
		return fontBundle{}, fmt.Errorf("加载统计说明字体失败: %w", err)
	}
	return fontBundle{title: title, body: body, meta: meta}, nil
}

func loadFontPair(primaryPath string, size float64) (fontPair, error) {
	primary, err := gg.LoadFontFace(primaryPath, size)
	if err != nil {
		return fontPair{}, err
	}

	var fallback font.Face
	if fallbackPath := findFallbackEmojiFont(primaryPath); fallbackPath != "" {
		fallback, _ = gg.LoadFontFace(fallbackPath, size)
	}

	return fontPair{
		primary:  primary,
		fallback: fallback,
		size:     size,
		emojiDir: filepath.Join(filepath.Dir(filepath.Dir(primaryPath)), "emojis", "72x72"),
	}, nil
}

func findFallbackEmojiFont(primaryPath string) string {
	candidates := []string{
		filepath.Join(filepath.Dir(primaryPath), "NotoEmoji-Regular.ttf"),
		filepath.Join(filepath.Dir(primaryPath), "NotoColorEmoji.ttf"),
		filepath.Join(filepath.Dir(primaryPath), "seguiemj.ttf"),
		`C:\Windows\Fonts\seguiemj.ttf`,
		`C:\Windows\Fonts\SegoeUIEmoji.ttf`,
		`/usr/share/fonts/truetype/noto/NotoEmoji-Regular.ttf`,
		`/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf`,
		`/usr/share/fonts/noto/NotoEmoji-Regular.ttf`,
		`/usr/share/fonts/noto/NotoColorEmoji.ttf`,
	}
	for _, candidate := range candidates {
		if candidate == "" || samePath(primaryPath, candidate) {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func drawTextLeft(dc *gg.Context, pair fontPair, text string, x, y float64) {
	drawText(dc, pair, text, x, y)
}

func drawTextRight(dc *gg.Context, pair fontPair, text string, x, y float64) {
	width := measureText(dc, pair, text)
	drawText(dc, pair, text, x-width, y)
}

func drawText(dc *gg.Context, pair fontPair, text string, x, y float64) {
	segments := segmentTextRuns(text)
	if pair.fallback == nil || !hasEmojiRun(segments) {
		dc.SetFontFace(pair.primary)
		dc.DrawString(text, x, y)
		return
	}

	cursor := x
	for _, segment := range segments {
		if segment.emoji {
			if img, ok := loadEmojiImage(pair.emojiDir, segment.text); ok {
				drawEmojiImage(dc, img, cursor, y, pair.size)
				cursor += pair.size
				continue
			}
		}
		face := pair.primary
		if segment.emoji && pair.fallback != nil {
			face = pair.fallback
		}
		dc.SetFontFace(face)
		dc.DrawString(segment.text, cursor, y)
		w, _ := dc.MeasureString(segment.text)
		cursor += w
	}
}

func measureText(dc *gg.Context, pair fontPair, text string) float64 {
	segments := segmentTextRuns(text)
	if pair.fallback == nil || !hasEmojiRun(segments) {
		dc.SetFontFace(pair.primary)
		w, _ := dc.MeasureString(text)
		return w
	}

	width := 0.0
	for _, segment := range segments {
		if segment.emoji {
			if _, ok := loadEmojiImage(pair.emojiDir, segment.text); ok {
				width += pair.size
				continue
			}
		}
		face := pair.primary
		if segment.emoji && pair.fallback != nil {
			face = pair.fallback
		}
		dc.SetFontFace(face)
		w, _ := dc.MeasureString(segment.text)
		width += w
	}
	return width
}

func ellipsizeText(dc *gg.Context, pair fontPair, text string, maxWidth float64) string {
	if measureText(dc, pair, text) <= maxWidth {
		return text
	}

	runes := []rune(text)
	ellipsis := "..."
	for len(runes) > 0 {
		candidate := string(runes) + ellipsis
		if measureText(dc, pair, candidate) <= maxWidth {
			return candidate
		}
		runes = runes[:len(runes)-1]
	}
	return ellipsis
}

type textSegment struct {
	text  string
	emoji bool
}

func segmentTextRuns(text string) []textSegment {
	runs := gemoji.Segment(text)
	if len(runs) == 0 {
		return nil
	}

	segments := make([]textSegment, 0, len(runs))
	for _, run := range runs {
		if run.Text == "" {
			continue
		}
		segments = append(segments, textSegment{
			text:  run.Text,
			emoji: run.IsEmoji,
		})
	}
	return segments
}

func hasEmojiRun(segments []textSegment) bool {
	for _, segment := range segments {
		if segment.emoji {
			return true
		}
	}
	return false
}

func loadEmojiImage(assetDir, text string) (image.Image, bool) {
	for _, candidate := range emojiFilenameCandidates(text) {
		path := filepath.Join(assetDir, candidate+".png")
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		img, err := png.Decode(file)
		_ = file.Close()
		if err != nil {
			continue
		}
		return img, true
	}
	return nil, false
}

func drawEmojiImage(dc *gg.Context, img image.Image, x, y, size float64) {
	w := float64(img.Bounds().Dx())
	h := float64(img.Bounds().Dy())
	if w <= 0 || h <= 0 {
		return
	}
	scale := size / math.Max(w, h)
	dc.Push()
	dc.Translate(x, y-size*0.9)
	dc.Scale(scale, scale)
	dc.DrawImage(img, 0, 0)
	dc.Pop()
}

func emojiFilenameCandidates(text string) []string {
	full := emojiFilename(text, false)
	withoutVS16 := emojiFilename(text, true)
	if full == withoutVS16 {
		return []string{full}
	}
	return []string{full, withoutVS16}
}

func emojiFilename(text string, stripVS16 bool) string {
	parts := make([]string, 0, len(text))
	for _, r := range text {
		if stripVS16 && r == 0xFE0F {
			continue
		}
		parts = append(parts, fmt.Sprintf("%x", r))
	}
	return strings.Join(parts, "-")
}

func setColor(dc *gg.Context, r, g, b int) {
	dc.SetRGB255(r, g, b)
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
