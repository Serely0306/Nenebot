package help

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
)

var (
	colorBgTop         = color.RGBA{20, 24, 43, 255}
	colorBgBottom      = color.RGBA{15, 33, 56, 255}
	colorCardBg        = color.RGBA{18, 30, 55, 232}
	colorCardBorder    = color.RGBA{122, 147, 196, 90}
	colorTitle         = color.RGBA{248, 250, 255, 255}
	colorAccent        = color.RGBA{150, 196, 255, 255}
	colorText          = color.RGBA{239, 243, 250, 255}
	colorDimText       = color.RGBA{198, 208, 226, 255}
	colorIntroText     = color.RGBA{232, 239, 249, 255}
	colorServiceBg     = color.RGBA{255, 255, 255, 0}
	colorServiceName   = color.RGBA{248, 251, 255, 255}
	colorServiceBorder = color.RGBA{146, 170, 220, 88}
	colorSectionTitle  = color.RGBA{255, 220, 150, 255}
	colorAccentBar     = color.RGBA{118, 168, 255, 220}
)

const (
	canvasWidth   = 940
	paddingX      = 44
	paddingTop    = 42
	paddingBottom = 42
	contentWidth  = canvasWidth - paddingX*2

	fontSizeTitle   = 46
	fontSizeIntro   = 28
	fontSizeSection = 36
	fontSizeSummary = 30
	fontSizeService = 26

	cardPaddingX = 26
	cardPaddingY = 22
	cardRadius   = 14.0
	cardGap      = 22

	commandColGap = 28

	serviceCols   = 2
	serviceColGap = 18
	serviceRowGap = 12
	servicePadX   = 16
	servicePadY   = 10
	serviceRadius = 8.0
	serviceMinH   = fontSizeService + servicePadY*2
)

type fontSet struct {
	title   font.Face
	intro   font.Face
	section font.Face
	summary font.Face
	service font.Face
}

func (m *Module) GenerateImage() error {
	data, err := GenerateImage(m.cfg, m.paths.FontFile)
	if err != nil {
		return err
	}
	return os.WriteFile(m.paths.HelpImage, data, 0o644)
}

func GenerateImage(cfg Config, fontPath string) ([]byte, error) {
	fonts, err := loadFontSet(fontPath)
	if err != nil {
		return nil, err
	}

	totalHeight := measureTotalHeight(cfg, fonts)
	dc := gg.NewContext(canvasWidth, totalHeight)

	drawGradientBackground(dc, totalHeight)
	drawHelpContent(dc, cfg, fonts)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, fmt.Errorf("编码 PNG 失败: %w", err)
	}

	return buf.Bytes(), nil
}

func SaveImage(cfg Config, fontPath, outputPath string) error {
	data, err := GenerateImage(cfg, fontPath)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0o644)
}

func loadFontSet(fontPath string) (*fontSet, error) {
	mkFace := func(size float64) (font.Face, error) {
		return gg.LoadFontFace(fontPath, size)
	}

	title, err := mkFace(fontSizeTitle)
	if err != nil {
		return nil, fmt.Errorf("加载标题字体失败: %w", err)
	}
	intro, err := mkFace(fontSizeIntro)
	if err != nil {
		return nil, fmt.Errorf("加载说明字体失败: %w", err)
	}
	section, err := mkFace(fontSizeSection)
	if err != nil {
		return nil, fmt.Errorf("加载分区字体失败: %w", err)
	}
	summary, err := mkFace(fontSizeSummary)
	if err != nil {
		return nil, fmt.Errorf("加载正文字体失败: %w", err)
	}
	service, err := mkFace(fontSizeService)
	if err != nil {
		return nil, fmt.Errorf("加载服务字体失败: %w", err)
	}

	return &fontSet{title, intro, section, summary, service}, nil
}

func measureTotalHeight(cfg Config, fonts *fontSet) int {
	y := float64(paddingTop)
	y += fontSizeTitle + 12

	for range cfg.IntroLines {
		y += fontSizeIntro + 10
	}
	y += 22

	for _, section := range cfg.Sections {
		y += measureSectionHeight(section, fonts) + cardGap
	}

	y += paddingBottom
	return int(math.Ceil(y))
}

func measureSectionHeight(section Section, fonts *fontSet) float64 {
	h := float64(cardPaddingY)
	innerW := float64(contentWidth - cardPaddingX*2)

	h += fontSizeSection + 18

	for _, line := range section.Summary {
		h += measureWrappedTextHeight(line, fonts.summary, innerW) + 4
	}

	if len(section.Commands) > 0 {
		h += fontSizeSummary + 12
		h += measureCommandListHeight(section.Commands, fonts.summary, innerW)
	}

	if len(section.Services) > 0 {
		h += fontSizeSummary + 12
		h += measureServiceGridHeight(section.Services, fonts.service, innerW)
	}

	if len(section.Notes) > 0 {
		h += 10
		for _, note := range section.Notes {
			h += measureWrappedTextHeight(note, fonts.summary, innerW) + 4
		}
	}

	h += cardPaddingY
	return h
}

func measureWrappedTextHeight(text string, face font.Face, maxWidth float64) float64 {
	dc := gg.NewContext(1, 1)
	dc.SetFontFace(face)
	lines := dc.WordWrap(text, maxWidth)
	if len(lines) == 0 {
		return 0
	}
	lineH := float64(face.Metrics().Height.Round())
	return float64(len(lines)) * (lineH + 2)
}

func commandCols(commands []string) int {
	if len(commands) >= 4 {
		return 2
	}
	return 1
}

func measureCommandListHeight(commands []string, face font.Face, totalW float64) float64 {
	if len(commands) == 0 {
		return 0
	}

	cols := commandCols(commands)
	colGap := 0.0
	if cols > 1 {
		colGap = commandColGap
	}
	colW := (totalW - colGap*float64(cols-1)) / float64(cols)
	total := 0.0

	for rowStart := 0; rowStart < len(commands); rowStart += cols {
		rowH := 0.0
		for offset := 0; offset < cols && rowStart+offset < len(commands); offset++ {
			item := "- " + strings.TrimSpace(commands[rowStart+offset])
			itemH := measureWrappedTextHeight(item, face, colW)
			if itemH > rowH {
				rowH = itemH
			}
		}
		total += rowH
		if rowStart+cols < len(commands) {
			total += 4
		}
	}

	return total
}

func drawGradientBackground(dc *gg.Context, height int) {
	for y := 0; y < height; y++ {
		t := float64(y) / float64(height)
		r := helpLerp(float64(colorBgTop.R), float64(colorBgBottom.R), t)
		g := helpLerp(float64(colorBgTop.G), float64(colorBgBottom.G), t)
		b := helpLerp(float64(colorBgTop.B), float64(colorBgBottom.B), t)
		dc.SetColor(color.RGBA{uint8(r), uint8(g), uint8(b), 255})
		dc.DrawLine(0, float64(y), float64(dc.Width()), float64(y))
		dc.Stroke()
	}
}

func helpLerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

func drawHelpContent(dc *gg.Context, cfg Config, fonts *fontSet) {
	y := float64(paddingTop)
	x := float64(paddingX)

	dc.SetFontFace(fonts.title)
	dc.SetColor(colorTitle)
	dc.DrawStringAnchored(cfg.Title, float64(canvasWidth)/2, y, 0.5, 1)
	y += fontSizeTitle + 6

	lineW := 144.0
	dc.SetColor(colorAccentBar)
	dc.SetLineWidth(2)
	dc.DrawLine(float64(canvasWidth)/2-lineW/2, y, float64(canvasWidth)/2+lineW/2, y)
	dc.Stroke()
	y += 18

	dc.SetFontFace(fonts.intro)
	dc.SetColor(colorIntroText)
	for _, line := range cfg.IntroLines {
		dc.DrawStringAnchored(line, float64(canvasWidth)/2, y, 0.5, 1)
		y += fontSizeIntro + 10
	}
	y += 12

	for _, section := range cfg.Sections {
		sectionH := measureSectionHeight(section, fonts)
		drawSectionCard(dc, x, y, float64(contentWidth), sectionH, section, fonts)
		y += sectionH + cardGap
	}
}

func drawSectionCard(dc *gg.Context, x, y, w, h float64, section Section, fonts *fontSet) {
	drawRoundedRect(dc, x, y, w, h, cardRadius, colorCardBg)

	dc.SetColor(colorCardBorder)
	dc.SetLineWidth(1)
	drawRoundedRectStroke(dc, x, y, w, h, cardRadius)

	innerX := x + cardPaddingX
	innerW := w - cardPaddingX*2
	cy := y + float64(cardPaddingY)

	barW := 4.0
	barH := float64(fontSizeSection) + 4
	drawRoundedRect(dc, innerX, cy-2, barW, barH, 2, colorAccentBar)

	dc.SetFontFace(fonts.section)
	dc.SetColor(colorSectionTitle)
	dc.DrawString(section.Title, innerX+barW+12, cy+float64(fontSizeSection)-4)
	cy += fontSizeSection + 18

	for _, line := range section.Summary {
		cy = drawWrappedText(dc, line, fonts.summary, innerX, cy, innerW, colorText)
		cy += 4
	}

	if len(section.Commands) > 0 {
		dc.SetFontFace(fonts.summary)
		dc.SetColor(colorAccent)
		dc.DrawString("常用指令示例：", innerX, cy+fontSizeSummary)
		cy += fontSizeSummary + 12
		cy += drawCommandList(dc, innerX, cy, innerW, section.Commands, fonts.summary, colorText)
	}

	if len(section.Services) > 0 {
		dc.SetFontFace(fonts.summary)
		dc.SetColor(colorAccent)
		dc.DrawString("服务列表：", innerX, cy+fontSizeSummary)
		cy += fontSizeSummary + 12
		cy += drawServiceGrid(dc, innerX, cy, innerW, section.Services, fonts)
	}

	if len(section.Notes) > 0 {
		cy += 6
		for _, note := range section.Notes {
			cy = drawWrappedText(dc, note, fonts.summary, innerX, cy, innerW, colorDimText)
			cy += 4
		}
	}
}

func drawCommandList(dc *gg.Context, x, y, totalW float64, commands []string, face font.Face, clr color.Color) float64 {
	if len(commands) == 0 {
		return 0
	}

	cols := commandCols(commands)
	colGap := 0.0
	if cols > 1 {
		colGap = commandColGap
	}
	colW := (totalW - colGap*float64(cols-1)) / float64(cols)
	offsetY := 0.0

	for rowStart := 0; rowStart < len(commands); rowStart += cols {
		rowH := 0.0
		for offset := 0; offset < cols && rowStart+offset < len(commands); offset++ {
			item := "- " + strings.TrimSpace(commands[rowStart+offset])
			itemH := measureWrappedTextHeight(item, face, colW)
			if itemH > rowH {
				rowH = itemH
			}
		}

		for offset := 0; offset < cols && rowStart+offset < len(commands); offset++ {
			item := "- " + strings.TrimSpace(commands[rowStart+offset])
			sx := x + float64(offset)*(colW+colGap)
			_ = drawWrappedText(dc, item, face, sx, y+offsetY, colW, clr)
		}

		offsetY += rowH
		if rowStart+cols < len(commands) {
			offsetY += 4
		}
	}

	return offsetY
}

func serviceLabel(svc Service) string {
	if strings.TrimSpace(svc.Desc) == "" {
		return strings.TrimSpace(svc.Name)
	}
	return strings.TrimSpace(svc.Name) + " - " + strings.TrimSpace(svc.Desc)
}

func measureServiceGridHeight(services []Service, face font.Face, totalW float64) float64 {
	if len(services) == 0 {
		return 0
	}

	colW := (totalW - float64(serviceColGap*(serviceCols-1))) / float64(serviceCols)
	total := 0.0

	for rowStart := 0; rowStart < len(services); rowStart += serviceCols {
		rowH := float64(serviceMinH)
		for offset := 0; offset < serviceCols && rowStart+offset < len(services); offset++ {
			label := serviceLabel(services[rowStart+offset])
			itemH := measureWrappedTextHeight(label, face, colW-float64(servicePadX*2)) + float64(servicePadY*2)
			if itemH < float64(serviceMinH) {
				itemH = float64(serviceMinH)
			}
			if itemH > rowH {
				rowH = itemH
			}
		}
		total += rowH
		if rowStart+serviceCols < len(services) {
			total += float64(serviceRowGap)
		}
	}

	return total
}

func drawServiceGrid(dc *gg.Context, x, y, totalW float64, services []Service, fonts *fontSet) float64 {
	if len(services) == 0 {
		return 0
	}

	colW := (totalW - float64(serviceColGap*(serviceCols-1))) / float64(serviceCols)
	offsetY := 0.0

	for rowStart := 0; rowStart < len(services); rowStart += serviceCols {
		rowH := float64(serviceMinH)
		for offset := 0; offset < serviceCols && rowStart+offset < len(services); offset++ {
			label := serviceLabel(services[rowStart+offset])
			itemH := measureWrappedTextHeight(label, fonts.service, colW-float64(servicePadX*2)) + float64(servicePadY*2)
			if itemH < float64(serviceMinH) {
				itemH = float64(serviceMinH)
			}
			if itemH > rowH {
				rowH = itemH
			}
		}

		for offset := 0; offset < serviceCols && rowStart+offset < len(services); offset++ {
			svc := services[rowStart+offset]
			sx := x + float64(offset)*(colW+float64(serviceColGap))
			sy := y + offsetY

			drawRoundedRect(dc, sx, sy, colW, rowH, serviceRadius, colorServiceBg)
			dc.SetColor(colorServiceBorder)
			dc.SetLineWidth(1)
			drawRoundedRectStroke(dc, sx, sy, colW, rowH, serviceRadius)
			drawWrappedText(dc, serviceLabel(svc), fonts.service, sx+float64(servicePadX), sy+float64(servicePadY), colW-float64(servicePadX*2), colorServiceName)
		}

		offsetY += rowH
		if rowStart+serviceCols < len(services) {
			offsetY += float64(serviceRowGap)
		}
	}

	return offsetY
}

func drawWrappedText(dc *gg.Context, text string, face font.Face, x, y, maxW float64, clr color.Color) float64 {
	dc.SetFontFace(face)
	lines := dc.WordWrap(text, maxW)
	if len(lines) == 0 {
		lines = []string{text}
	}

	lineH := float64(face.Metrics().Height.Round())
	dc.SetColor(clr)
	for _, line := range lines {
		dc.DrawString(strings.TrimSpace(line), x, y+lineH*0.8)
		y += lineH + 2
	}
	return y
}

func drawRoundedRect(dc *gg.Context, x, y, w, h, r float64, clr color.Color) {
	dc.SetColor(clr)
	dc.DrawRoundedRectangle(x, y, w, h, r)
	dc.Fill()
}

func drawRoundedRectStroke(dc *gg.Context, x, y, w, h, r float64) {
	dc.DrawRoundedRectangle(x, y, w, h, r)
	dc.Stroke()
}

var _ image.Image
