package data

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"
)

const (
	defaultHSQueryAPIURL = "https://www.singlewindow.cn/access/ui/1776225848566/Param002?cw2qEsfh=0_ANsxalqWkpLsAEgamx5Yf7_OktVhbTEB3iaFDccSc1jBtIsQzH876YTScp_EGQXeJLc6dz.vPczj3ElcKWsl7JZuoZmBqd2.0FlHnMGvxHAr82gJ76UAA"
	hsQueryRowsPerPage   = 200
)

type HsQueryAPIRepo struct {
	queryAPIURL string
	httpClient  *http.Client
}

func NewHsQueryAPIRepo(c *conf.Bootstrap) *HsQueryAPIRepo {
	apiURL := defaultHSQueryAPIURL
	if c != nil {
		if v := strings.TrimSpace(c.GetHsQueryApiUrl()); v != "" {
			apiURL = v
		}
	}
	if v := strings.TrimSpace(os.Getenv("CAICHIP_HS_QUERY_API_URL")); v != "" {
		apiURL = v
	}
	timeout := 15 * time.Second
	if c != nil && c.GetHsResolveSyncTimeoutMs() > 0 {
		timeout = time.Duration(c.GetHsResolveSyncTimeoutMs()) * time.Millisecond
	}
	return &HsQueryAPIRepo{
		queryAPIURL: apiURL,
		httpClient:  &http.Client{Timeout: timeout},
	}
}

type hsQueryAPIResponse struct {
	Status string `json:"status"`
	Data   struct {
		PageNo       int                      `json:"pageNo"`
		PageSumCount int                      `json:"pageSumCount"`
		Rows         []map[string]interface{} `json:"data"`
	} `json:"data"`
}

func (r *HsQueryAPIRepo) FetchAllByCoreHS6(ctx context.Context, coreHS6 string) ([]biz.HsItemRecord, error) {
	pageNo := 1
	all := make([]biz.HsItemRecord, 0, 128)
	for {
		resp, err := r.fetchOnePage(ctx, coreHS6, pageNo)
		if err != nil {
			return nil, err
		}
		for i := range resp.Data.Rows {
			record, mapErr := mapHSItemRecord(resp.Data.Rows[i], coreHS6)
			if mapErr != nil {
				return nil, mapErr
			}
			all = append(all, record)
		}
		if resp.Data.PageSumCount <= 0 || pageNo >= resp.Data.PageSumCount {
			break
		}
		pageNo++
	}
	return all, nil
}

func (r *HsQueryAPIRepo) fetchOnePage(ctx context.Context, coreHS6 string, pageNo int) (*hsQueryAPIResponse, error) {
	payloadData, err := json.Marshal(map[string]any{
		"pageNo":      pageNo,
		"rowsPerPage": hsQueryRowsPerPage,
		"paramName":   "CusComplex",
		"filterField": "CODE_TS",
		"filterValue": coreHS6,
	})
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"Head": map[string]any{},
		"Data": string(payloadData),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.queryAPIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := r.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hs_query_api http status=%d body=%s", resp.StatusCode, string(body))
	}
	var out hsQueryAPIResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if strings.ToLower(strings.TrimSpace(out.Status)) != "success" {
		return nil, fmt.Errorf("hs_query_api status=%s", out.Status)
	}
	return &out, nil
}

func mapHSItemRecord(row map[string]interface{}, coreHS6 string) (biz.HsItemRecord, error) {
	codeTS := strings.TrimSpace(anyToString(row["CODE_TS"]))
	if codeTS == "" {
		return biz.HsItemRecord{}, errors.New("hs_query_api 返回空 CODE_TS")
	}
	raw, err := json.Marshal(row)
	if err != nil {
		return biz.HsItemRecord{}, err
	}
	return biz.HsItemRecord{
		CodeTS:        codeTS,
		GName:         strings.TrimSpace(anyToString(row["G_NAME"])),
		Unit1:         strings.TrimSpace(anyToString(row["UNIT_1"])),
		Unit2:         strings.TrimSpace(anyToString(row["UNIT_2"])),
		ControlMark:   strings.TrimSpace(anyToString(row["CONTROL_MARK"])),
		SourceCoreHS6: strings.TrimSpace(coreHS6),
		RawJSON:       raw,
	}, nil
}

func anyToString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return fmt.Sprintf("%.0f", t)
	default:
		return fmt.Sprint(v)
	}
}

var _ biz.HsQueryAPIRepo = (*HsQueryAPIRepo)(nil)
