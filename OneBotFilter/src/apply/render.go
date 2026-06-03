package apply

import (
	"bytes"
	"fmt"
	"image/png"
	"time"

	"github.com/fogleman/gg"
)

type ReviewRow struct {
	Index       int
	ID          string
	GroupID     string
	GroupName   string
	MemberCount int
	Applicant   string
	Nickname    string
	Purpose     string
	CreatedAt   string
}

type RenderReviewInput struct {
	Title string
	Rows  []ReviewRow
}

func RenderReviewList(fontPath string, input RenderReviewInput) ([]byte, error) {
	const (
		width   = 860
		rowH    = 64
		padX    = 20
		padY    = 20
		headerH = 70
	)

	rows := input.Rows
	height := headerH + padY + len(rows)*rowH + padY
	if len(rows) == 0 {
		height = headerH + 120
	}

	dc := gg.NewContext(width, height)

	// Background
	dc.SetHexColor("#faf8f3")
	dc.Clear()

	// Header background
	dc.SetHexColor("#2d2050")
	dc.DrawRectangle(0, 0, width, headerH)
	dc.Fill()

	// Load font
	titleFace, err := gg.LoadFontFace(fontPath, 24)
	if err != nil {
		return nil, fmt.Errorf("加载字体失败: %w", err)
	}
	bodyFace, err := gg.LoadFontFace(fontPath, 14)
	if err != nil {
		return nil, fmt.Errorf("加载字体失败: %w", err)
	}
	smallFace, err := gg.LoadFontFace(fontPath, 12)
	if err != nil {
		return nil, fmt.Errorf("加载字体失败: %w", err)
	}

	// Header title
	dc.SetFontFace(titleFace)
	dc.SetHexColor("#ffffff")
	dc.DrawString(input.Title, padX, 44)

	// Empty state
	if len(rows) == 0 {
		dc.SetFontFace(bodyFace)
		dc.SetHexColor("#9b8eb8")
		dc.DrawStringAnchored("暂无待审核申请", width/2, headerH+60, 0.5, 0.5)

		var buf bytes.Buffer
		if err := png.Encode(&buf, dc.Image()); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	// Rows
	for i, r := range rows {
		y := float64(headerH + padY + i*rowH)

		// Alternating row background
		if i%2 == 1 {
			dc.SetHexColor("#f5f0fa")
		} else {
			dc.SetHexColor("#ffffff")
		}
		dc.DrawRectangle(0, y, width, rowH)
		dc.Fill()

		// Left: ID + group
		dc.SetFontFace(bodyFace)
		dc.SetHexColor("#2d2050")

		groupDisplay := r.GroupID
		if r.GroupName != "" {
			groupDisplay = r.GroupName + " (" + r.GroupID + ")"
		}
		rowTitle := fmt.Sprintf("#%d [%s] %s · %d人", r.Index, r.ID, groupDisplay, r.MemberCount)
		if len(rowTitle) > 55 {
			rowTitle = rowTitle[:52] + "..."
		}
		dc.DrawString(rowTitle, padX, y+24)

		// Applicant
		applicantDisplay := r.Applicant
		if r.Nickname != "" {
			applicantDisplay = r.Nickname + " (" + r.Applicant + ")"
		}
		dc.SetFontFace(smallFace)
		dc.SetHexColor("#6b5e8a")
		dc.DrawString(applicantDisplay, padX, y+46)

		// Purpose (truncated)
		purpose := r.Purpose
		if len(purpose) > 30 {
			purpose = purpose[:28] + "..."
		}
		dc.DrawString(purpose, padX+280, y+24)

		// Time
		t, err := time.Parse(time.RFC3339, r.CreatedAt)
		timeStr := ""
		if err == nil {
			timeStr = t.In(time.Local).Format("01-02 15:04")
		}
		dc.SetHexColor("#9b8eb8")
		dc.DrawStringAnchored(timeStr, width-padX, y+24, 1, 0)

		// Divider line
		if i < len(rows)-1 {
			dc.SetHexColor("#e8e4f0")
			dc.SetLineWidth(0.5)
			dc.DrawLine(padX, y+rowH, width-padX, y+rowH)
			dc.Stroke()
		}
	}

	// Footer
	footerY := float64(height - 10)
	dc.SetFontFace(smallFace)
	dc.SetHexColor("#9b8eb8")
	msg := fmt.Sprintf("使用 /审核通过 <ID> 或 /审核拒绝 <ID> 处理 · %s", time.Now().Format("15:04:05"))
	dc.DrawStringAnchored(msg, width/2, footerY, 0.5, 1)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

