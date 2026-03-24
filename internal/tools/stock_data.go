package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// StockDataTool queries Vietnam stock market data via a vnstock-api sidecar.
// Supports: realtime prices, OHLCV history, intraday, financials, screening.
type StockDataTool struct {
	baseURL    string
	httpClient *http.Client
}

// NewStockDataTool creates a stock data tool pointing at the given vnstock-api base URL.
func NewStockDataTool(baseURL string) *StockDataTool {
	if baseURL == "" {
		baseURL = "http://localhost:11235"
	}
	return &StockDataTool{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *StockDataTool) Name() string { return "stock_data" }

func (t *StockDataTool) Description() string {
	return `Query Vietnam stock market data (HOSE/HNX/UPCOM). Actions:
- price: Get realtime price, bid/ask for a stock symbol
- history: Get OHLCV historical data (daily/weekly/monthly)
- intraday: Get today's intraday price movements
- financials: Get balance sheet, income statement, or cash flow
- screening: Search/filter stocks by exchange, industry, volume`
}

func (t *StockDataTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"price", "history", "intraday", "financials", "screening"},
				"description": "The action to perform",
			},
			"symbol": map[string]any{
				"type":        "string",
				"description": "Stock ticker symbol (e.g. VNM, FPT, VIC). Required for all actions except screening.",
			},
			"start": map[string]any{
				"type":        "string",
				"description": "Start date YYYY-MM-DD (history action only)",
			},
			"end": map[string]any{
				"type":        "string",
				"description": "End date YYYY-MM-DD (history action only)",
			},
			"interval": map[string]any{
				"type":        "string",
				"enum":        []string{"1D", "1W", "1M"},
				"description": "Data interval for history action (default 1D)",
			},
			"financial_type": map[string]any{
				"type":        "string",
				"enum":        []string{"balance_sheet", "income_statement", "cash_flow"},
				"description": "Financial statement type (financials action only)",
			},
			"period": map[string]any{
				"type":        "string",
				"enum":        []string{"quarter", "annual"},
				"description": "Reporting period (financials action only, default quarter)",
			},
			"exchange": map[string]any{
				"type":        "string",
				"enum":        []string{"HOSE", "HNX", "UPCOM"},
				"description": "Stock exchange filter (screening action only)",
			},
			"industry": map[string]any{
				"type":        "string",
				"description": "Industry filter (screening action only)",
			},
			"min_volume": map[string]any{
				"type":        "integer",
				"description": "Minimum trading volume filter (screening action only)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *StockDataTool) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)
	symbol := strings.ToUpper(strings.TrimSpace(getString(args, "symbol")))

	if action == "" {
		return ErrorResult("action is required")
	}
	if action != "screening" && symbol == "" {
		return ErrorResult("symbol is required for action: " + action)
	}

	var endpoint string
	params := url.Values{}

	switch action {
	case "price":
		endpoint = "/price/" + symbol
	case "history":
		endpoint = "/history/" + symbol
		if v := getString(args, "start"); v != "" {
			params.Set("start", v)
		}
		if v := getString(args, "end"); v != "" {
			params.Set("end", v)
		}
		if v := getString(args, "interval"); v != "" {
			params.Set("interval", v)
		}
	case "intraday":
		endpoint = "/intraday/" + symbol
	case "financials":
		endpoint = "/financials/" + symbol
		if v := getString(args, "financial_type"); v != "" {
			params.Set("type", v)
		}
		if v := getString(args, "period"); v != "" {
			params.Set("period", v)
		}
	case "screening":
		endpoint = "/screening"
		if v := getString(args, "exchange"); v != "" {
			params.Set("exchange", v)
		}
		if v := getString(args, "industry"); v != "" {
			params.Set("industry", v)
		}
		if v, ok := args["min_volume"].(float64); ok {
			params.Set("min_volume", fmt.Sprintf("%.0f", v))
		}
	default:
		return ErrorResult("unknown action: " + action)
	}

	reqURL := t.baseURL + endpoint
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return ErrorResult("failed to build request: " + err.Error())
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return ErrorResult("vnstock-api unreachable: " + err.Error())
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512KB max

	if resp.StatusCode != http.StatusOK {
		return ErrorResult(fmt.Sprintf("vnstock-api error (%d): %s", resp.StatusCode, string(body)))
	}

	// Pretty-print JSON for LLM readability.
	var parsed any
	if json.Unmarshal(body, &parsed) == nil {
		pretty, _ := json.MarshalIndent(parsed, "", "  ")
		return NewResult(string(pretty))
	}
	return NewResult(string(body))
}

// getString extracts a string value from the args map.
func getString(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}
