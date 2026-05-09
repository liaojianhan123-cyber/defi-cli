// Package pendle implements a read-only adapter for Pendle Finance.
//
// Pendle is a yield-tokenisation protocol that splits yield-bearing tokens into
// Principal Tokens (PT — fixed yield locked to maturity) and Yield Tokens
// (YT — leveraged variable yield).  The adapter exposes:
//
//   - YieldOpportunities: one "fixed" entry per active PT market + one "lp" entry
//     for the corresponding AMM pool (instant withdrawal).
//   - YieldPositions: a user's non-zero PT, YT, and LP balances across all
//     active markets on a chain.
//
// Data source: Pendle public REST API v2 (no API key required).
// Docs: https://api-v2.pendle.finance/core/docs
package pendle

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	clierr "github.com/ggonzalez94/defi-cli/internal/errors"
	"github.com/ggonzalez94/defi-cli/internal/httpx"
	"github.com/ggonzalez94/defi-cli/internal/id"
	"github.com/ggonzalez94/defi-cli/internal/model"
	"github.com/ggonzalez94/defi-cli/internal/providers"
)

const defaultBaseURL = "https://api-v2.pendle.finance/core/v1"

// supportedChainIDs lists EVM chains where Pendle has live markets.
var supportedChainIDs = map[int64]bool{
	1:     true, // Ethereum
	42161: true, // Arbitrum
	8453:  true, // Base
	10:    true, // Optimism
	56:    true, // BNB Chain
	5000:  true, // Mantle
}

// Client is the Pendle Finance adapter.
type Client struct {
	http    *httpx.Client
	baseURL string
	now     func() time.Time
}

// New creates a Pendle Client using the shared httpx transport.
func New(httpClient *httpx.Client) *Client {
	return &Client{
		http:    httpClient,
		baseURL: defaultBaseURL,
		now:     time.Now,
	}
}

// Info satisfies providers.Provider.
func (c *Client) Info() model.ProviderInfo {
	return model.ProviderInfo{
		Name:        "pendle",
		Type:        "yield",
		RequiresKey: false,
		Capabilities: []string{
			"yield.opportunities",
			"yield.positions",
		},
	}
}

// SupportedChainIDs returns a copy of the set of EVM chain IDs where Pendle is
// deployed.  Used by runner.go to pre-filter chains before querying.
func SupportedChainIDs() map[int64]bool {
	out := make(map[int64]bool, len(supportedChainIDs))
	for k, v := range supportedChainIDs {
		out[k] = v
	}
	return out
}

// ─── Pendle API response types ────────────────────────────────────────────────

type pendlePrice struct {
	USD float64 `json:"usd"`
}

type pendleToken struct {
	Address  string      `json:"address"`
	Symbol   string      `json:"symbol"`
	Decimals int         `json:"decimals"`
	Price    pendlePrice `json:"price"`
}

type pendleMarketDetails struct {
	TVL           pendlePrice `json:"tvl"`
	Liquidity     pendlePrice `json:"liquidity"`
	ImpliedAPY    float64     `json:"impliedApy"`
	UnderlyingAPY float64     `json:"underlyingApy"`
	YTFloatingAPY float64     `json:"ytFloatingApy"`
	LPRewardAPY   float64     `json:"lpRewardApy"`
	AggregatedAPY float64     `json:"aggregatedApy"`
	MaxBoostedAPY float64     `json:"maxBoostedApy"`
}

type pendleMarket struct {
	ID              string              `json:"id"`
	Address         string              `json:"address"`
	Expiry          string              `json:"expiry"`
	PT              pendleToken         `json:"pt"`
	YT              pendleToken         `json:"yt"`
	SY              pendleToken         `json:"sy"`
	UnderlyingAsset pendleToken         `json:"underlyingAsset"`
	AccountingAsset pendleToken         `json:"accountingAsset"`
	Details         pendleMarketDetails `json:"details"`
}

type pendleMarketsResponse struct {
	Total   int            `json:"total"`
	Results []pendleMarket `json:"results"`
}

type pendlePositionAmount struct {
	Amount    string      `json:"amount"`   // base-unit string (may be "0" or absent)
	Decimals  int         `json:"decimals"` // token decimals
	Valuation pendlePrice `json:"valuation"`
}

type pendlePositionItem struct {
	PT     pendlePositionAmount `json:"pt"`
	YT     pendlePositionAmount `json:"yt"`
	LP     pendlePositionAmount `json:"lp"`
	Market pendleMarket         `json:"market"`
}

type pendlePositionsResponse struct {
	Total   int                  `json:"total"`
	Results []pendlePositionItem `json:"results"`
}

// ─── HTTP fetch helpers ───────────────────────────────────────────────────────

// fetchActiveMarkets retrieves all non-expired markets for an EVM chain, ordered
// by TVL descending.
func (c *Client) fetchActiveMarkets(ctx context.Context, chainID int64) ([]pendleMarket, error) {
	url := fmt.Sprintf("%s/%d/markets/active?order_by=tvl:desc&limit=200", c.baseURL, chainID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "pendle: build markets request", err)
	}

	var result pendleMarketsResponse
	if _, err := c.http.DoJSON(ctx, req, &result); err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "pendle: fetch markets", err)
	}
	return result.Results, nil
}

// fetchPositions retrieves all positions for a wallet on an EVM chain.
func (c *Client) fetchPositions(ctx context.Context, chainID int64, address string) ([]pendlePositionItem, error) {
	url := fmt.Sprintf("%s/%d/positions/%s?limit=200", c.baseURL, chainID, strings.ToLower(address))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "pendle: build positions request", err)
	}

	var result pendlePositionsResponse
	if _, err := c.http.DoJSON(ctx, req, &result); err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "pendle: fetch positions", err)
	}
	return result.Results, nil
}

// ─── YieldProvider ────────────────────────────────────────────────────────────

// YieldOpportunities implements providers.YieldProvider.
// It returns two entries per active market: one "fixed" (PT) and one "lp" (AMM pool).
func (c *Client) YieldOpportunities(ctx context.Context, req providers.YieldRequest) ([]model.YieldOpportunity, error) {
	if !req.Chain.IsEVM() {
		return nil, clierr.New(clierr.CodeUnsupported, "pendle only supports EVM chains")
	}
	if !supportedChainIDs[req.Chain.EVMChainID] {
		return nil, clierr.New(clierr.CodeUnsupported,
			fmt.Sprintf("pendle does not support chain %d; supported: 1,10,42161,8453,56,5000", req.Chain.EVMChainID))
	}

	markets, err := c.fetchActiveMarkets(ctx, req.Chain.EVMChainID)
	if err != nil {
		return nil, err
	}

	now := c.now()
	fetchedAt := now.UTC().Format(time.RFC3339)
	caip2 := req.Chain.CAIP2

	var out []model.YieldOpportunity

	for _, m := range markets {
		// Skip markets whose expiry has already passed.
		expiry, expiryOK := parseExpiry(m.Expiry)
		if expiryOK && !expiry.After(now) {
			continue
		}

		// Underlying asset CAIP-19.
		assetID := caip2AssetID(caip2, m.UnderlyingAsset.Address)

		// Optional asset filter.
		if (req.Asset.Address != "" || req.Asset.Symbol != "") && !assetMatchesPendle(req.Asset, m.UnderlyingAsset, m.AccountingAsset) {
			continue
		}

		tvl := m.Details.TVL.USD

		// Min-TVL filter.
		if req.MinTVLUSD > 0 && tvl < req.MinTVLUSD {
			continue
		}

		backing := []model.YieldBackingAsset{{
			AssetID:  assetID,
			Symbol:   m.UnderlyingAsset.Symbol,
			SharePct: 100,
		}}

		// ── PT opportunity (fixed yield to maturity) ───────────────────────────
		ptAPY := m.Details.ImpliedAPY
		lockupDays := 0.0
		withdrawalTerms := "fixed-maturity"
		if expiryOK {
			lockupDays = math.Max(0, expiry.Sub(now).Hours()/24)
		}

		if req.MinAPY <= 0 || ptAPY >= req.MinAPY {
			out = append(out, model.YieldOpportunity{
				OpportunityID:        opportunityID(caip2, m.Address, "pt"),
				Provider:             "pendle",
				Protocol:             "pendle",
				ChainID:              caip2,
				AssetID:              assetID,
				ProviderNativeID:     m.Address,
				ProviderNativeIDKind: model.NativeIDKindPoolID,
				Type:                 "fixed",
				APYBase:              ptAPY,
				APYReward:            0,
				APYTotal:             ptAPY,
				TVLUSD:               tvl,
				LiquidityUSD:         m.Details.Liquidity.USD,
				LockupDays:           lockupDays,
				WithdrawalTerms:      withdrawalTerms,
				BackingAssets:        backing,
				SourceURL:            fmt.Sprintf("https://app.pendle.finance/trade/markets?search=%s", m.Address),
				FetchedAt:            fetchedAt,
			})
		}

		// ── LP opportunity (variable yield from AMM + incentives) ─────────────
		// AggregatedAPY = underlying base + PENDLE incentive rewards.
		// Fall back to lpRewardAPY + underlyingAPY when aggregatedAPY is zero.
		lpAPY := m.Details.AggregatedAPY
		if lpAPY == 0 {
			lpAPY = m.Details.LPRewardAPY + m.Details.UnderlyingAPY
		}

		if req.MinAPY <= 0 || lpAPY >= req.MinAPY {
			out = append(out, model.YieldOpportunity{
				OpportunityID:        opportunityID(caip2, m.Address, "lp"),
				Provider:             "pendle",
				Protocol:             "pendle",
				ChainID:              caip2,
				AssetID:              assetID,
				ProviderNativeID:     m.Address,
				ProviderNativeIDKind: model.NativeIDKindPoolID,
				Type:                 "lp",
				APYBase:              m.Details.UnderlyingAPY,
				APYReward:            m.Details.LPRewardAPY,
				APYTotal:             lpAPY,
				TVLUSD:               tvl,
				LiquidityUSD:         m.Details.Liquidity.USD,
				LockupDays:           0,
				WithdrawalTerms:      "instant",
				BackingAssets:        backing,
				SourceURL:            fmt.Sprintf("https://app.pendle.finance/trade/pools?search=%s", m.Address),
				FetchedAt:            fetchedAt,
			})
		}
	}

	// Sort descending by total APY; break ties by TVL.
	sort.Slice(out, func(i, j int) bool {
		if out[i].APYTotal != out[j].APYTotal {
			return out[i].APYTotal > out[j].APYTotal
		}
		return out[i].TVLUSD > out[j].TVLUSD
	})

	if req.Limit > 0 && len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

// ─── YieldPositionsProvider ───────────────────────────────────────────────────

// YieldPositions implements providers.YieldPositionsProvider.
// It emits one model.YieldPosition for each non-zero balance (PT, YT, LP).
func (c *Client) YieldPositions(ctx context.Context, req providers.YieldPositionsRequest) ([]model.YieldPosition, error) {
	if !req.Chain.IsEVM() {
		return nil, clierr.New(clierr.CodeUnsupported, "pendle only supports EVM chains")
	}
	if !supportedChainIDs[req.Chain.EVMChainID] {
		return nil, clierr.New(clierr.CodeUnsupported,
			fmt.Sprintf("pendle does not support chain %d", req.Chain.EVMChainID))
	}
	if req.Account == "" {
		return nil, clierr.New(clierr.CodeUsage, "pendle: --address is required for yield positions")
	}

	items, err := c.fetchPositions(ctx, req.Chain.EVMChainID, req.Account)
	if err != nil {
		return nil, err
	}

	now := c.now()
	fetchedAt := now.UTC().Format(time.RFC3339)
	caip2 := req.Chain.CAIP2
	account := strings.ToLower(req.Account)

	var out []model.YieldPosition

	for _, item := range items {
		m := item.Market
		assetID := caip2AssetID(caip2, m.UnderlyingAsset.Address)

		// Optional asset filter.
		if (req.Asset.Address != "" || req.Asset.Symbol != "") && !assetMatchesPendle(req.Asset, m.UnderlyingAsset, m.AccountingAsset) {
			continue
		}

		lpAPY := m.Details.AggregatedAPY
		if lpAPY == 0 {
			lpAPY = m.Details.LPRewardAPY + m.Details.UnderlyingAPY
		}

		// ── PT position ───────────────────────────────────────────────────────
		if item.PT.Valuation.USD > 0 || nonZeroAmount(item.PT.Amount) {
			ptDecimals := item.PT.Decimals
			if ptDecimals == 0 {
				ptDecimals = m.PT.Decimals
			}
			if ptDecimals == 0 {
				ptDecimals = 18
			}
			out = append(out, model.YieldPosition{
				Protocol:             "pendle",
				Provider:             "pendle",
				ChainID:              caip2,
				AccountAddress:       account,
				PositionType:         "fixed",
				OpportunityID:        opportunityID(caip2, m.Address, "pt"),
				AssetID:              caip2AssetID(caip2, m.PT.Address), // the PT token itself
				ProviderNativeID:     m.Address,
				ProviderNativeIDKind: model.NativeIDKindPoolID,
				Amount: model.AmountInfo{
					AmountBaseUnits: normaliseAmount(item.PT.Amount),
					AmountDecimal:   baseToDecimal(item.PT.Amount, ptDecimals),
					Decimals:        ptDecimals,
				},
				AmountUSD: item.PT.Valuation.USD,
				APYTotal:  m.Details.ImpliedAPY,
				SourceURL: fmt.Sprintf("https://app.pendle.finance/trade/markets?search=%s", m.Address),
				FetchedAt: fetchedAt,
			})
		}

		// ── YT position ───────────────────────────────────────────────────────
		if item.YT.Valuation.USD > 0 || nonZeroAmount(item.YT.Amount) {
			ytDecimals := item.YT.Decimals
			if ytDecimals == 0 {
				ytDecimals = m.YT.Decimals
			}
			if ytDecimals == 0 {
				ytDecimals = 18
			}
			out = append(out, model.YieldPosition{
				Protocol:             "pendle",
				Provider:             "pendle",
				ChainID:              caip2,
				AccountAddress:       account,
				PositionType:         "variable",
				OpportunityID:        opportunityID(caip2, m.Address, "yt"),
				AssetID:              caip2AssetID(caip2, m.YT.Address), // the YT token itself
				ProviderNativeID:     m.Address,
				ProviderNativeIDKind: model.NativeIDKindPoolID,
				Amount: model.AmountInfo{
					AmountBaseUnits: normaliseAmount(item.YT.Amount),
					AmountDecimal:   baseToDecimal(item.YT.Amount, ytDecimals),
					Decimals:        ytDecimals,
				},
				AmountUSD: item.YT.Valuation.USD,
				APYTotal:  m.Details.YTFloatingAPY,
				SourceURL: fmt.Sprintf("https://app.pendle.finance/trade/markets?search=%s", m.Address),
				FetchedAt: fetchedAt,
			})
		}

		// ── LP position ───────────────────────────────────────────────────────
		if item.LP.Valuation.USD > 0 || nonZeroAmount(item.LP.Amount) {
			lpDecimals := item.LP.Decimals
			if lpDecimals == 0 {
				lpDecimals = 18 // Pendle LP tokens are always 18-decimal
			}
			out = append(out, model.YieldPosition{
				Protocol:             "pendle",
				Provider:             "pendle",
				ChainID:              caip2,
				AccountAddress:       account,
				PositionType:         "lp",
				OpportunityID:        opportunityID(caip2, m.Address, "lp"),
				AssetID:              assetID, // underlying asset of the pool
				ProviderNativeID:     m.Address,
				ProviderNativeIDKind: model.NativeIDKindPoolID,
				Amount: model.AmountInfo{
					AmountBaseUnits: normaliseAmount(item.LP.Amount),
					AmountDecimal:   baseToDecimal(item.LP.Amount, lpDecimals),
					Decimals:        lpDecimals,
				},
				AmountUSD: item.LP.Valuation.USD,
				APYTotal:  lpAPY,
				SourceURL: fmt.Sprintf("https://app.pendle.finance/trade/pools?search=%s", m.Address),
				FetchedAt: fetchedAt,
			})
		}
	}

	// Sort by USD value descending.
	sort.Slice(out, func(i, j int) bool {
		return out[i].AmountUSD > out[j].AmountUSD
	})

	if req.Limit > 0 && len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// opportunityID builds a deterministic ID: "pendle:{caip2}:{marketAddr}:{kind}".
func opportunityID(caip2, marketAddr, kind string) string {
	return fmt.Sprintf("pendle:%s:%s:%s", caip2, strings.ToLower(marketAddr), kind)
}

// caip2AssetID builds a CAIP-19 ERC-20 identifier.
func caip2AssetID(caip2, tokenAddr string) string {
	return fmt.Sprintf("%s/erc20:%s", caip2, strings.ToLower(tokenAddr))
}

// parseExpiry tries multiple ISO 8601 layouts used by the Pendle API.
func parseExpiry(s string) (time.Time, bool) {
	for _, l := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
	} {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// assetMatchesPendle returns true when the user-supplied id.Asset refers to the
// underlying or accounting asset of a Pendle market.
func assetMatchesPendle(asset id.Asset, underlying, accounting pendleToken) bool {
	if asset.Address != "" {
		return strings.EqualFold(asset.Address, underlying.Address) ||
			strings.EqualFold(asset.Address, accounting.Address)
	}
	if asset.Symbol != "" {
		sym := strings.ToUpper(asset.Symbol)
		return strings.EqualFold(sym, underlying.Symbol) ||
			strings.EqualFold(sym, accounting.Symbol)
	}
	return true
}

// nonZeroAmount reports whether a base-unit string represents a non-zero value.
func nonZeroAmount(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return false
	}
	for _, ch := range s {
		if ch != '0' {
			return true
		}
	}
	return false
}

// normaliseAmount returns "0" for empty strings, otherwise the raw string.
func normaliseAmount(s string) string {
	if strings.TrimSpace(s) == "" {
		return "0"
	}
	return s
}

// baseToDecimal converts a base-unit integer string to a human-readable decimal.
// Returns "0" on empty input.
func baseToDecimal(baseUnits string, decimals int) string {
	s := strings.TrimSpace(baseUnits)
	if s == "" || s == "0" {
		return "0"
	}
	// Pad with leading zeros until len > decimals.
	for len(s) <= decimals {
		s = "0" + s
	}
	intPart := s[:len(s)-decimals]
	fracPart := strings.TrimRight(s[len(s)-decimals:], "0")
	if fracPart == "" {
		return intPart
	}
	return intPart + "." + fracPart
}
