package stats

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	rand "math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/fogleman/gg"
	gemoji "github.com/gogpu/gg/text/emoji"
	"golang.org/x/image/font"
)

type RankRow struct {
	Index   int
	UserID  int64
	Name    string
	Count   int64
	Percent float64
	Avatar  []byte
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
	SessionAvatar     []byte
	RecvCount         int64
	SendCount         int64
	BotSendCount      int64
	InternalSendCount int64
	Rows              []RankRow
}

type GlobalStatsRow struct {
	SessionName string
	TotalCount  int64
	BotCounts   []int64
	Avatar      []byte
}

type RenderGlobalStatsInput struct {
	Title             string
	SessionName       string
	RangeLabel        string
	RecvCount         int64
	SendCount         int64
	BotSendCount      int64
	InternalSendCount int64
	BotNames          []string
	BotSummary        []int64
	Rows              []GlobalStatsRow
}

type fontPair struct {
	primary  font.Face
	size     float64
	emojiDir string
}

type fontBundle struct {
	title fontPair
	body  fontPair
	meta  fontPair
}

func RenderRankImage(fontPath string, input RenderRankInput) ([]byte, error) {
	return renderRankCardImage(fontPath, input)
}

func RenderStatsImage(fontPath string, input RenderStatsInput) ([]byte, error) {
	return renderStatsCardImage(fontPath, input)
}

func RenderGlobalStatsImage(fontPath string, input RenderGlobalStatsInput) ([]byte, error) {
	fonts, err := loadFonts(fontPath)
	if err != nil {
		return nil, err
	}

	const (
		width       = 1320
		headerH     = 250
		padding     = 32
		rowHeight   = 68
		headerColor = 32
	)

	colCount := 2 + len(input.BotNames)
	if colCount < 2 {
		colCount = 2
	}
	sessionColW := 340.0
	otherColW := float64(width-padding*2) - sessionColW
	otherColW = otherColW / float64(colCount-1)

	height := int(float64(headerH+padding+padding) + rowHeight*float64(maxInt(2, len(input.Rows)+1)))
	dc := gg.NewContext(width, height)
	drawStarryBackground(dc)

	dc.SetRGBA255(255, 255, 255, 18)
	dc.DrawRoundedRectangle(float64(padding), 28, float64(width-padding*2), 170, 24)
	dc.Fill()
	setColor(dc, 245, 248, 255)
	drawTextLeft(dc, fonts.title, input.Title, float64(padding+24), 62)
	setColor(dc, 232, 238, 250)
	drawTextLeft(dc, fonts.body, input.SessionName, float64(padding+24), 104)
	setColor(dc, 222, 230, 246)
	drawTextLeft(dc, fonts.meta, input.RangeLabel, float64(padding+24), 134)
	setColor(dc, 236, 242, 252)
	drawTextLeft(dc, fonts.meta, fmt.Sprintf("接收 %d / 发送 %d / bot %d / 内部 %d", input.RecvCount, input.SendCount, input.BotSendCount, input.InternalSendCount), float64(padding+24), 164)
	setColor(dc, 228, 236, 250)
	drawTextLeft(dc, fonts.meta, buildBotSummaryLine(input.BotNames, input.BotSummary), float64(padding+24), 190)

	tableTop := float64(headerH)
	tableWidth := float64(width - padding*2)
	dc.SetRGBA255(255, 255, 255, headerColor)
	dc.DrawRoundedRectangle(float64(padding), tableTop, tableWidth, rowHeight, 12)
	dc.Fill()

	x := float64(padding)
	setColor(dc, 234, 240, 252)
	drawTableCell(dc, fonts.body, "会话", x+16, tableTop+43, sessionColW-32, false)
	x += sessionColW
	drawTableCell(dc, fonts.body, "总计", x+16, tableTop+43, otherColW-32, true)
	x += otherColW
	for _, botName := range input.BotNames {
		drawTableCell(dc, fonts.body, botName, x+16, tableTop+43, otherColW-32, true)
		x += otherColW
	}

	for i, row := range input.Rows {
		top := tableTop + rowHeight*float64(i+1)
		alpha := 18
		if i%2 == 1 {
			alpha = 24
		}
		dc.SetRGBA255(255, 255, 255, alpha)
		dc.DrawRoundedRectangle(float64(padding), top, tableWidth, rowHeight-8, 10)
		dc.Fill()

		x = float64(padding)
		setColor(dc, 255, 255, 255)
		if row.Avatar != nil {
			dc.SetFontFace(fonts.body.primary)
			drawCircularAvatar(dc, row.Avatar, x+12, top+8, 42, sessionAvatarLabel(row.SessionName))
			drawTableCell(dc, fonts.body, row.SessionName, x+62, top+42, sessionColW-78, false)
		} else {
			drawTableCell(dc, fonts.body, row.SessionName, x+16, top+42, sessionColW-32, false)
		}
		x += sessionColW
		drawTableCell(dc, fonts.body, fmt.Sprintf("%d", row.TotalCount), x+16, top+42, otherColW-32, true)
		x += otherColW
		for _, count := range row.BotCounts {
			drawTableCell(dc, fonts.body, fmt.Sprintf("%d", count), x+16, top+42, otherColW-32, true)
			x += otherColW
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderRankCardImage(fontPath string, input RenderRankInput) ([]byte, error) {
	const (
		width      = 1080
		headerH    = 210
		rowHeight  = 132
		padding    = 40
		cardRadius = 14
		avatarSize = 78
	)

	fonts, err := loadFonts(fontPath)
	if err != nil {
		return nil, err
	}

	height := headerH + padding + maxInt(1, len(input.Rows))*rowHeight + padding
	dc := gg.NewContext(width, height)
	drawStarryBackground(dc)

	dc.SetRGBA255(255, 255, 255, 20)
	dc.DrawRoundedRectangle(float64(padding), 28, float64(width-padding*2), 138, 22)
	dc.Fill()
	setColor(dc, 245, 248, 255)
	drawTextLeft(dc, fonts.title, input.Title, float64(padding+24), 70)
	setColor(dc, 210, 220, 240)
	drawTextLeft(dc, fonts.body, input.SessionName, float64(padding+24), 112)
	drawTextLeft(dc, fonts.meta, input.RangeLabel, float64(padding+24), 144)
	drawMetricPill(dc, fonts.meta, fmt.Sprintf("总消息 %d", input.TotalCount), float64(width-238), 54, 170, 40)

	startY := float64(headerH)
	cardWidth := float64(width - padding*2)
	barX := float64(padding + 126)
	barW := float64(width - padding*2 - 286)
	nameMaxWidth := float64(width - padding*2 - 420)

	if len(input.Rows) == 0 {
		setColor(dc, 220, 226, 240)
		drawTextLeft(dc, fonts.body, "暂无数据", float64(padding+24), startY+58)
	}

	for i, row := range input.Rows {
		top := startY + float64(i*rowHeight)
		alpha := 24
		if i < 3 {
			alpha = 34
		}
		dc.SetRGBA255(255, 255, 255, alpha)
		dc.DrawRoundedRectangle(float64(padding), top, cardWidth, rowHeight-14, cardRadius)
		dc.Fill()

		avatarX := float64(padding + 22)
		avatarY := top + 20
		dc.SetFontFace(fonts.body.primary)
		drawCircularAvatar(dc, row.Avatar, avatarX, avatarY, avatarSize, rankAvatarLabel(row))

		nameY := top + 38
		qqY := top + 70
		barY := top + 91
		indexX := float64(width - 210)

		setRankColor(dc, row.Index)
		drawTextLeft(dc, fonts.body, fmt.Sprintf("#%d", row.Index), indexX, nameY)
		setColor(dc, 255, 255, 255)
		nameText := ellipsizeText(dc, fonts.body, row.Name, nameMaxWidth)
		drawTextLeft(dc, fonts.body, nameText, float64(padding+126), nameY)
		setColor(dc, 178, 190, 215)
		qqText := "QQ " + formatID(row.UserID)
		drawTextLeft(dc, fonts.meta, qqText, float64(padding+126), qqY)

		setColor(dc, 205, 216, 238)
		drawTextRight(dc, fonts.body, fmt.Sprintf("%d", row.Count), float64(width-78), nameY)
		drawTextRight(dc, fonts.meta, fmt.Sprintf("%.1f%%", row.Percent), float64(width-78), qqY)

		dc.SetRGBA255(255, 255, 255, 26)
		dc.DrawRoundedRectangle(barX, barY, barW, 12, 6)
		dc.Fill()

		progress := math.Max(0, math.Min(1, row.Percent/100))
		setProgressColor(dc, row.Index)
		dc.DrawRoundedRectangle(barX, barY, math.Max(10, barW*progress), 12, 6)
		dc.Fill()
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderStatsCardImage(fontPath string, input RenderStatsInput) ([]byte, error) {
	const (
		width      = 1080
		headerH    = 250
		rowHeight  = 112
		padding    = 40
		avatarSize = 92
	)

	fonts, err := loadFonts(fontPath)
	if err != nil {
		return nil, err
	}

	height := headerH + padding + maxInt(1, len(input.Rows))*rowHeight + padding
	dc := gg.NewContext(width, height)
	drawStarryBackground(dc)

	dc.SetRGBA255(255, 255, 255, 22)
	dc.DrawRoundedRectangle(float64(padding), 28, float64(width-padding*2), 176, 24)
	dc.Fill()
	dc.SetFontFace(fonts.body.primary)
	drawCircularAvatar(dc, input.SessionAvatar, float64(padding+24), 56, avatarSize, "群")

	textX := float64(padding + 136)
	setColor(dc, 245, 248, 255)
	drawTextLeft(dc, fonts.title, input.Title, textX, 72)
	setColor(dc, 232, 238, 250)
	drawTextLeft(dc, fonts.body, input.SessionName, textX, 116)
	setColor(dc, 222, 230, 246)
	drawTextLeft(dc, fonts.meta, input.RangeLabel, textX, 148)

	metricTop := 164.0
	metricW := 168.0
	drawMetricPill(dc, fonts.meta, fmt.Sprintf("接收 %d", input.RecvCount), textX, metricTop, metricW, 36)
	drawMetricPill(dc, fonts.meta, fmt.Sprintf("发送 %d", input.SendCount), textX+metricW+12, metricTop, metricW, 36)
	drawMetricPill(dc, fonts.meta, fmt.Sprintf("bot %d", input.BotSendCount), textX+(metricW+12)*2, metricTop, metricW, 36)
	drawMetricPill(dc, fonts.meta, fmt.Sprintf("内部 %d", input.InternalSendCount), textX+(metricW+12)*3, metricTop, metricW, 36)

	startY := float64(headerH)
	cardWidth := float64(width - padding*2)
	maxCount := int64(1)
	for _, row := range input.Rows {
		if row.Count > maxCount {
			maxCount = row.Count
		}
	}
	if len(input.Rows) == 0 {
		setColor(dc, 220, 226, 240)
		drawTextLeft(dc, fonts.body, "暂无 bot 发送记录", float64(padding+24), startY+58)
	}
	for i, row := range input.Rows {
		top := startY + float64(i*rowHeight)
		dc.SetRGBA255(255, 255, 255, 24)
		dc.DrawRoundedRectangle(float64(padding), top, cardWidth, rowHeight-14, 16)
		dc.Fill()

		textLeft := float64(padding + 28)
		nameText := ellipsizeText(dc, fonts.body, row.Name, 520)
		setColor(dc, 248, 250, 255)
		drawTextLeft(dc, fonts.body, nameText, textLeft, top+40)
		setColor(dc, 220, 228, 244)
		drawTextLeft(dc, fonts.meta, "下游 bot-app 发送", textLeft, top+72)
		setColor(dc, 240, 245, 255)
		drawTextRight(dc, fonts.body, fmt.Sprintf("%d", row.Count), float64(width-78), top+42)
		setColor(dc, 228, 236, 250)
		drawTextRight(dc, fonts.meta, fmt.Sprintf("%.1f%%", row.Percent), float64(width-78), top+74)

		barX := textLeft
		barY := top + 88
		barW := float64(width - padding*2 - 174)
		dc.SetRGBA255(255, 255, 255, 26)
		dc.DrawRoundedRectangle(barX, barY, barW, 10, 5)
		dc.Fill()
		progress := float64(row.Count) / float64(maxCount)
		setProgressColor(dc, row.Index)
		dc.DrawRoundedRectangle(barX, barY, math.Max(10, barW*progress), 10, 5)
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

	return fontPair{
		primary:  primary,
		size:     size,
		emojiDir: filepath.Join(filepath.Dir(filepath.Dir(primaryPath)), "emojis", "72x72"),
	}, nil
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
	if !hasEmojiRun(segments) {
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
			dc.SetFontFace(pair.primary)
			placeholder := missingEmojiPlaceholder()
			dc.DrawString(placeholder, cursor, y)
			w, _ := dc.MeasureString(placeholder)
			cursor += w
			continue
		}
		dc.SetFontFace(pair.primary)
		dc.DrawString(segment.text, cursor, y)
		w, _ := dc.MeasureString(segment.text)
		cursor += w
	}
}

func measureText(dc *gg.Context, pair fontPair, text string) float64 {
	segments := segmentTextRuns(text)
	if !hasEmojiRun(segments) {
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
			dc.SetFontFace(pair.primary)
			placeholder := missingEmojiPlaceholder()
			w, _ := dc.MeasureString(placeholder)
			width += w
			continue
		}
		dc.SetFontFace(pair.primary)
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

func drawStarryBackground(dc *gg.Context) {
	width := dc.Width()
	height := dc.Height()
	bgTop := color.RGBA{16, 18, 48, 255}
	bgBottom := color.RGBA{11, 32, 62, 255}
	for y := 0; y < height; y++ {
		t := float64(y) / float64(maxInt(1, height))
		r := lerp(float64(bgTop.R), float64(bgBottom.R), t)
		g := lerp(float64(bgTop.G), float64(bgBottom.G), t)
		b := lerp(float64(bgTop.B), float64(bgBottom.B), t)
		dc.SetRGB255(int(r), int(g), int(b))
		dc.DrawLine(0, float64(y), float64(width), float64(y))
		dc.Stroke()
	}

	seed := int64(width*73856093 ^ height*19349663)
	rng := rand.New(rand.NewSource(seed))
	starCount := width * height / 9000
	if starCount < 60 {
		starCount = 60
	}
	if starCount > 260 {
		starCount = 260
	}
	for i := 0; i < starCount; i++ {
		x := rng.Float64() * float64(width)
		y := rng.Float64() * float64(height)
		radius := 0.35 + rng.Float64()*1.3
		alpha := 0.22 + rng.Float64()*0.45
		dc.SetRGBA(1, 1, 1, alpha)
		dc.DrawCircle(x, y, radius)
		dc.Fill()
	}

	dc.SetRGBA(0, 0, 0, 0.14)
	dc.DrawRectangle(0, 0, float64(width), float64(height))
	dc.Fill()
}

func drawCircularAvatar(dc *gg.Context, data []byte, x, y float64, size int, fallback string) {
	dc.Push()
	defer dc.Pop()
	if img := decodeImageBytes(data); img != nil {
		drawCircularImage(dc, img, x, y, float64(size))
		return
	}
	drawAvatarPlaceholder(dc, x, y, float64(size), fallback)
}

func decodeImageBytes(data []byte) image.Image {
	if len(data) == 0 {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}

func drawCircularImage(dc *gg.Context, img image.Image, x, y, size float64) {
	bounds := img.Bounds()
	w := float64(bounds.Dx())
	h := float64(bounds.Dy())
	if w <= 0 || h <= 0 {
		return
	}

	scale := size / math.Min(w, h)
	drawW := w * scale
	drawH := h * scale
	dc.Push()
	dc.DrawCircle(x+size/2, y+size/2, size/2)
	dc.Clip()
	dc.Translate(x-(drawW-size)/2, y-(drawH-size)/2)
	dc.Scale(scale, scale)
	dc.DrawImage(img, -bounds.Min.X, -bounds.Min.Y)
	dc.ResetClip()
	dc.Pop()

	dc.SetRGBA255(255, 255, 255, 60)
	dc.SetLineWidth(2)
	dc.DrawCircle(x+size/2, y+size/2, size/2-1)
	dc.Stroke()
}

func drawAvatarPlaceholder(dc *gg.Context, x, y, size float64, fallback string) {
	dc.SetRGBA255(255, 255, 255, 28)
	dc.DrawCircle(x+size/2, y+size/2, size/2)
	dc.Fill()
	dc.SetRGBA255(255, 255, 255, 70)
	dc.SetLineWidth(2)
	dc.DrawCircle(x+size/2, y+size/2, size/2-1)
	dc.Stroke()

	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		fallback = "?"
	}
	runes := []rune(fallback)
	if len(runes) > 1 {
		fallback = string(runes[:1])
	}
	dc.SetRGB255(230, 236, 250)
	dc.DrawStringAnchored(fallback, x+size/2, y+size/2+size*0.16, 0.5, 0.5)
}

func drawMetricPill(dc *gg.Context, pair fontPair, text string, x, y, w, h float64) {
	dc.SetRGBA255(255, 255, 255, 26)
	dc.DrawRoundedRectangle(x, y, w, h, h/2)
	dc.Fill()
	setColor(dc, 236, 242, 252)
	drawTextLeft(dc, pair, ellipsizeText(dc, pair, text, w-28), x+14, y+h/2+pair.size*0.34)
}

func setRankColor(dc *gg.Context, index int) {
	switch index {
	case 1:
		dc.SetRGB255(255, 220, 128)
	case 2:
		dc.SetRGB255(210, 225, 245)
	case 3:
		dc.SetRGB255(230, 176, 128)
	default:
		dc.SetRGB255(205, 216, 238)
	}
}

func setProgressColor(dc *gg.Context, index int) {
	switch index {
	case 1:
		dc.SetRGBA255(255, 198, 98, 225)
	case 2:
		dc.SetRGBA255(138, 185, 255, 218)
	case 3:
		dc.SetRGBA255(190, 133, 255, 212)
	default:
		dc.SetRGBA255(120, 166, 255, 205)
	}
}

func rankAvatarLabel(row RankRow) string {
	if strings.TrimSpace(row.Name) != "" {
		return row.Name
	}
	return formatID(row.UserID)
}

func sessionAvatarLabel(sessionName string) string {
	if strings.Contains(sessionName, "私聊") {
		return "私"
	}
	return "群"
}

func formatID(id int64) string {
	if id <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d", id)
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

func missingEmojiPlaceholder() string {
	return "□"
}

func setColor(dc *gg.Context, r, g, b int) {
	dc.SetRGB255(r, g, b)
}

func drawTableCell(dc *gg.Context, pair fontPair, text string, x, y, maxWidth float64, alignRight bool) {
	text = ellipsizeText(dc, pair, text, maxWidth)
	if alignRight {
		drawTextRight(dc, pair, text, x+maxWidth, y)
		return
	}
	drawTextLeft(dc, pair, text, x, y)
}

func buildBotSummaryLine(botNames []string, counts []int64) string {
	if len(botNames) == 0 || len(counts) == 0 {
		return "bot 明细：无"
	}
	parts := make([]string, 0, minInt(len(botNames), len(counts)))
	for i := 0; i < len(botNames) && i < len(counts); i++ {
		parts = append(parts, fmt.Sprintf("%s %d", botNames[i], counts[i]))
	}
	return "bot 明细：" + strings.Join(parts, " / ")
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
