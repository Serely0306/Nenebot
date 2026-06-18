package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"haruki-sekai-api/client"
	"haruki-sekai-api/utils"

	"github.com/go-resty/resty/v2"
	"github.com/gofiber/fiber/v3"
)

const (
	cnWishRankingBaseURL            = "https://act.nvsgames.cn"
	cnWishRankingNewsPath           = "/site/api/v2/news/search"
	cnWishRankingExecPath           = "/act/5236/process/exec/v2"
	cnWishRankingAppID              = 5236
	cnWishRankingWebsiteID          = 148
	cnWishRankingDefaultLanguage    = "zh-CN"
	cnWishRankingServerID           = "60001"
	cnWishRankingDefaultPageSize    = 50
	cnWishRankingTotalPageCount     = 10
	cnWishRankingTopLadderProcessID = "query_top_ladder"
)

type cnWishRankingNewsItem struct {
	Keyword string `json:"keyword"`
}

type cnWishRankingNewsResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		PageNews []cnWishRankingNewsItem `json:"page_news"`
	} `json:"data"`
}

type cnWishRankingFrontParams struct {
	PageSize int `json:"page_size"`
	Page     int `json:"page"`
}

type cnWishRankingExecResponse struct {
	Code    int                           `json:"code"`
	Message string                        `json:"message"`
	Data    cnWishRankingExecResponseData `json:"data"`
}

type cnWishRankingExecResponseData struct {
	AtDataSourceOutput cnWishRankingExecAtDataSourceOutput `json:"at_data_source_output"`
	ProcessID          string                              `json:"process_id"`
}

type cnWishRankingExecAtDataSourceOutput struct {
	Value cnWishRankingExecValue `json:"value"`
}

type cnWishRankingExecValue struct {
	Ladder []map[string]any `json:"ladder"`
	TopN   []map[string]any `json:"topN"`
}

type cnWishRankingPeriodInfo struct {
	PeriodIndex int   `json:"period_index"`
	PeriodTotal int   `json:"period_total"`
	StartAt     int64 `json:"start_at"`
	EndAt       int64 `json:"end_at"`
}

func getCNWishRankingTopLadder(c fiber.Ctx) error {
	region, mgr, err := getMgr(c)
	if err != nil {
		return err
	}
	if region != utils.HarukiSekaiServerRegionCN {
		return fiber.NewError(fiber.StatusBadRequest, "wish ranking only supports cn server")
	}

	accessToken, err := getNuverseAccessToken(mgr)
	if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}

	activityID := c.Query("activity_id")
	var newsItems []cnWishRankingNewsItem
	if activityID == "" {
		ctx, cancel := context.WithTimeout(c.RequestCtx(), 20*time.Second)
		defer cancel()

		newsItems, err = fetchCNWishRankingNewsItems(ctx, mgr.Proxy)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("fetch wish ranking news failed: %v", err))
		}
		activityID, err = pickCNWishRankingActivityID(newsItems, time.Now().In(loadCNWishRankingLocation()))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("resolve activity_id failed: %v", err))
		}
	} else {
		ctx, cancel := context.WithTimeout(c.RequestCtx(), 20*time.Second)
		defer cancel()

		newsItems, err = fetchCNWishRankingNewsItems(ctx, mgr.Proxy)
		if err != nil {
			newsItems = nil
		}
	}

	ctx, cancel := context.WithTimeout(c.RequestCtx(), 20*time.Second)
	defer cancel()

	pages, err := fetchCNWishRankingPages(ctx, mgr.Proxy, accessToken, activityID)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("fetch wish ranking failed: %v", err))
	}
	var periodInfo *cnWishRankingPeriodInfo
	if len(newsItems) > 0 {
		periodInfo, _ = resolveCNWishRankingPeriodInfo(activityID, newsItems)
	}
	result, err := buildCNWishRankingResult(activityID, pages, periodInfo)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("parse wish ranking failed: %v", err))
	}

	c.Set("X-Haruki-Resolved-Activity-Id", activityID)
	return c.Status(http.StatusOK).JSON(result)
}

func getNuverseAccessToken(mgr *client.SekaiClientManager) (string, error) {
	for _, cli := range mgr.Clients {
		if cli == nil || cli.Account == nil {
			continue
		}
		if token := strings.TrimSpace(cli.Account.GetToken()); token != "" {
			return token, nil
		}
	}
	return "", fmt.Errorf("no nuverse access token available")
}

func buildCNWishRankingPayload(accessToken, activityID string, page int) (map[string]any, error) {
	accessToken = strings.TrimSpace(accessToken)
	activityID = strings.TrimSpace(activityID)
	if accessToken == "" {
		return nil, fmt.Errorf("access_token is required")
	}
	if activityID == "" {
		return nil, fmt.Errorf("activity_id is required")
	}

	payload := map[string]any{
		"login_type":   "gsdk",
		"access_token": accessToken,
		"process_id":   cnWishRankingTopLadderProcessID,
		"activity_id":  activityID,
		"server_id":    cnWishRankingServerID,
	}
	if page <= 0 {
		return nil, fmt.Errorf("page must be a positive integer")
	}

	frontParams, err := json.Marshal(cnWishRankingFrontParams{
		PageSize: cnWishRankingDefaultPageSize,
		Page:     page,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal front_params failed: %w", err)
	}
	payload["front_params"] = string(frontParams)
	return payload, nil
}

func fetchCNWishRankingPages(ctx context.Context, proxy, accessToken, activityID string) ([]cnWishRankingExecResponse, error) {
	pages := make([]cnWishRankingExecResponse, 0, cnWishRankingTotalPageCount)
	for page := 1; page <= cnWishRankingTotalPageCount; page++ {
		payload, err := buildCNWishRankingPayload(accessToken, activityID, page)
		if err != nil {
			return nil, err
		}
		result, _, err := postCNWishRankingTopLadder(ctx, proxy, payload)
		if err != nil {
			return nil, err
		}
		pages = append(pages, result)
	}
	return pages, nil
}

func buildCNWishRankingResult(activityID string, pages []cnWishRankingExecResponse, periodInfo *cnWishRankingPeriodInfo) (map[string]any, error) {
	if len(pages) == 0 {
		return nil, fmt.Errorf("no ranking pages fetched")
	}

	var ladder []map[string]any
	topN := make([]map[string]any, 0, len(pages)*cnWishRankingDefaultPageSize)
	for _, page := range pages {
		value := page.Data.AtDataSourceOutput.Value
		if len(ladder) == 0 && len(value.Ladder) > 0 {
			ladder = value.Ladder
		}
		topN = append(topN, value.TopN...)
	}

	return map[string]any{
		"activity_id": activityID,
		"process_id":  cnWishRankingTopLadderProcessID,
		"page_size":   cnWishRankingDefaultPageSize,
		"page_count":  len(pages),
		"total_count": len(topN),
		"period_info": periodInfo,
		"ladder":      ladder,
		"topN":        topN,
	}, nil
}

func fetchCNWishRankingActivityID(ctx context.Context, proxy string) (string, error) {
	items, err := fetchCNWishRankingNewsItems(ctx, proxy)
	if err != nil {
		return "", err
	}
	return pickCNWishRankingActivityID(items, time.Now().In(loadCNWishRankingLocation()))
}

func fetchCNWishRankingNewsItems(ctx context.Context, proxy string) ([]cnWishRankingNewsItem, error) {
	client := newCNWishRankingHTTPClient(proxy)
	resp, err := client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"app_id":    strconv.Itoa(cnWishRankingAppID),
			"language":  cnWishRankingDefaultLanguage,
			"website":   strconv.Itoa(cnWishRankingWebsiteID),
			"page":      "1",
			"block":     "1",
			"channel":   "1",
			"page_size": "1000",
			"top_flag":  "false",
		}).
		Get(cnWishRankingBaseURL + cnWishRankingNewsPath)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("news search returned status %d", resp.StatusCode())
	}

	var result cnWishRankingNewsResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("decode news search response failed: %w", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("news search failed: %s", result.Message)
	}
	return result.Data.PageNews, nil
}

func pickCNWishRankingActivityID(items []cnWishRankingNewsItem, now time.Time) (string, error) {
	type candidate struct {
		activityID string
		start      time.Time
		end        time.Time
	}

	var (
		firstCandidate  *candidate
		activeCandidate *candidate
		latestCandidate *candidate
	)

	for _, item := range items {
		parts := parseCNWishRankingKeyword(item.Keyword)
		activityID := strings.TrimSpace(parts["activityId"])
		if activityID == "" {
			continue
		}

		start := parseCNWishRankingTime(parts["startTime"])
		end := parseCNWishRankingTime(parts["endTime"])
		current := &candidate{
			activityID: activityID,
			start:      start,
			end:        end,
		}
		if firstCandidate == nil {
			firstCandidate = current
		}
		if latestCandidate == nil || (!current.start.IsZero() && current.start.After(latestCandidate.start)) {
			latestCandidate = current
		}
		if !start.IsZero() && !end.IsZero() && !now.Before(start) && !now.After(end) {
			if activeCandidate == nil || current.start.After(activeCandidate.start) {
				activeCandidate = current
			}
		}
	}

	switch {
	case activeCandidate != nil:
		return activeCandidate.activityID, nil
	case latestCandidate != nil:
		return latestCandidate.activityID, nil
	case firstCandidate != nil:
		return firstCandidate.activityID, nil
	default:
		return "", fmt.Errorf("no activity_id found in news list")
	}
}

func resolveCNWishRankingPeriodInfo(activityID string, items []cnWishRankingNewsItem) (*cnWishRankingPeriodInfo, error) {
	type candidate struct {
		activityID string
		start      time.Time
		end        time.Time
	}

	candidates := make([]candidate, 0, len(items))
	for _, item := range items {
		parts := parseCNWishRankingKeyword(item.Keyword)
		curActivityID := strings.TrimSpace(parts["activityId"])
		if curActivityID == "" {
			continue
		}
		start := parseCNWishRankingTime(parts["startTime"])
		end := parseCNWishRankingTime(parts["endTime"])
		if start.IsZero() || end.IsZero() {
			continue
		}
		candidates = append(candidates, candidate{
			activityID: curActivityID,
			start:      start,
			end:        end,
		})
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no valid period info found")
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].start.Before(candidates[j].start)
	})

	for i, candidate := range candidates {
		if candidate.activityID != activityID {
			continue
		}
		return &cnWishRankingPeriodInfo{
			PeriodIndex: i + 1,
			PeriodTotal: len(candidates),
			StartAt:     candidate.start.Unix(),
			EndAt:       candidate.end.Unix(),
		}, nil
	}
	return nil, fmt.Errorf("activity_id %s not found in period list", activityID)
}

func parseCNWishRankingKeyword(keyword string) map[string]string {
	result := make(map[string]string)
	for _, item := range strings.Split(keyword, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result
}

func parseCNWishRankingTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}

	loc := loadCNWishRankingLocation()
	for _, layout := range []string{
		"2006/1/2 15:04",
		"2006/01/02 15:04",
		"2006/1/2",
		"2006/01/02",
	} {
		if ts, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return ts
		}
	}
	return time.Time{}
}

func loadCNWishRankingLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}

func postCNWishRankingTopLadder(ctx context.Context, proxy string, payload map[string]any) (cnWishRankingExecResponse, int, error) {
	client := newCNWishRankingHTTPClient(proxy)
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json;charset=UTF-8").
		SetBody(payload).
		Post(cnWishRankingBaseURL + cnWishRankingExecPath)
	if err != nil {
		return cnWishRankingExecResponse{}, 0, err
	}

	var result cnWishRankingExecResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return cnWishRankingExecResponse{}, resp.StatusCode(), fmt.Errorf("decode exec response failed: %w", err)
	}
	if result.Code != 0 {
		return cnWishRankingExecResponse{}, resp.StatusCode(), fmt.Errorf("exec failed: %s", result.Message)
	}
	return result, resp.StatusCode(), nil
}

func newCNWishRankingHTTPClient(proxy string) *resty.Client {
	client := resty.New().SetTimeout(20 * time.Second)
	if proxy == "" {
		return client
	}
	if _, err := url.Parse(proxy); err == nil {
		client.SetProxy(proxy)
	}
	return client
}
