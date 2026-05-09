package pendle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ggonzalez94/defi-cli/internal/httpx"
	"github.com/ggonzalez94/defi-cli/internal/id"
	"github.com/ggonzalez94/defi-cli/internal/providers"
)

// ─── Fixture JSON ─────────────────────────────────────────────────────────────

// Two active markets on Ethereum (chainID 1):
//
//	market1 (weETH): impliedAPY=4.5, underlyingAPY=3.2, lpRewardAPY=1.1, aggregatedAPY=4.3, tvl=50M
//	market2 (USDC):  impliedAPY=6.0, underlyingAPY=5.0, lpRewardAPY=0.8, aggregatedAPY=5.8, tvl=20M
var fakeMarketsJSON = `{
  "total": 2,
  "results": [
    {
      "id":      "0xmarket1",
      "address": "0xmarket1",
      "expiry":  "2099-12-26T00:00:00.000Z",
      "pt":  {"address":"0xpt1",  "symbol":"PT-weETH-DEC2099","decimals":18,"price":{"usd":3400}},
      "yt":  {"address":"0xyt1",  "symbol":"YT-weETH-DEC2099","decimals":18,"price":{"usd":50}},
      "sy":  {"address":"0xsy1",  "symbol":"SY-weETH",         "decimals":18,"price":{"usd":3450}},
      "underlyingAsset":{"address":"0xweeth","symbol":"weETH","decimals":18,"price":{"usd":3450}},
      "accountingAsset": {"address":"0xweth", "symbol":"WETH", "decimals":18,"price":{"usd":3000}},
      "details":{
        "tvl":{"usd":50000000},"liquidity":{"usd":25000000},
        "impliedApy":4.5,"underlyingApy":3.2,"ytFloatingApy":12.0,
        "lpRewardApy":1.1,"aggregatedApy":4.3
      }
    },
    {
      "id":      "0xmarket2",
      "address": "0xmarket2",
      "expiry":  "2099-06-30T00:00:00.000Z",
      "pt":  {"address":"0xpt2","symbol":"PT-USDC-JUN2099","decimals":6,"price":{"usd":0.97}},
      "yt":  {"address":"0xyt2","symbol":"YT-USDC-JUN2099","decimals":6,"price":{"usd":0.03}},
      "sy":  {"address":"0xsy2","symbol":"SY-USDC",         "decimals":6,"price":{"usd":1.0}},
      "underlyingAsset":{"address":"0xusdc","symbol":"USDC","decimals":6,"price":{"usd":1.0}},
      "accountingAsset": {"address":"0xusdc","symbol":"USDC","decimals":6,"price":{"usd":1.0}},
      "details":{
        "tvl":{"usd":20000000},"liquidity":{"usd":10000000},
        "impliedApy":6.0,"underlyingApy":5.0,"ytFloatingApy":18.0,
        "lpRewardApy":0.8,"aggregatedApy":5.8
      }
    }
  ]
}`

// Wallet 0xalice holds PT in market1 and LP in market2; YT balances are zero.
var fakePosJSON = `{
  "total": 2,
  "results": [
    {
      "pt": {"amount":"1000000000000000000","decimals":18,"valuation":{"usd":3400}},
      "yt": {"amount":"0",                  "decimals":18,"valuation":{"usd":0}},
      "lp": {"amount":"0",                  "decimals":18,"valuation":{"usd":0}},
      "market":{
        "id":"0xmarket1","address":"0xmarket1","expiry":"2099-12-26T00:00:00.000Z",
        "pt":{"address":"0xpt1","symbol":"PT-weETH-DEC2099","decimals":18,"price":{"usd":3400}},
        "yt":{"address":"0xyt1","symbol":"YT-weETH-DEC2099","decimals":18,"price":{"usd":50}},
        "sy":{"address":"0xsy1","symbol":"SY-weETH","decimals":18,"price":{"usd":3450}},
        "underlyingAsset":{"address":"0xweeth","symbol":"weETH","decimals":18,"price":{"usd":3450}},
        "accountingAsset": {"address":"0xweth","symbol":"WETH","decimals":18,"price":{"usd":3000}},
        "details":{
          "tvl":{"usd":50000000},"liquidity":{"usd":25000000},
          "impliedApy":4.5,"underlyingApy":3.2,"ytFloatingApy":12.0,
          "lpRewardApy":1.1,"aggregatedApy":4.3
        }
      }
    },
    {
      "pt": {"amount":"0",                   "decimals":6, "valuation":{"usd":0}},
      "yt": {"amount":"0",                   "decimals":6, "valuation":{"usd":0}},
      "lp": {"amount":"500000000000000000",  "decimals":18,"valuation":{"usd":1500}},
      "market":{
        "id":"0xmarket2","address":"0xmarket2","expiry":"2099-06-30T00:00:00.000Z",
        "pt":{"address":"0xpt2","symbol":"PT-USDC-JUN2099","decimals":6,"price":{"usd":0.97}},
        "yt":{"address":"0xyt2","symbol":"YT-USDC-JUN2099","decimals":6,"price":{"usd":0.03}},
        "sy":{"address":"0xsy2","symbol":"SY-USDC","decimals":6,"price":{"usd":1.0}},
        "underlyingAsset":{"address":"0xusdc","symbol":"USDC","decimals":6,"price":{"usd":1.0}},
        "accountingAsset": {"address":"0xusdc","symbol":"USDC","decimals":6,"price":{"usd":1.0}},
        "details":{
          "tvl":{"usd":20000000},"liquidity":{"usd":10000000},
          "impliedApy":6.0,"underlyingApy":5.0,"ytFloatingApy":18.0,
          "lpRewardApy":0.8,"aggregatedApy":5.8
        }
      }
    }
  ]
}`

// ─── Test server ──────────────────────────────────────────────────────────────

func newTestServer(t *testing.T) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/markets/active"):
			fmt.Fprint(w, fakeMarketsJSON)
		case strings.Contains(r.URL.Path, "/positions/"):
			fmt.Fprint(w, fakePosJSON)
		default:
			http.NotFound(w, r)
		}
	}))

	c := New(httpx.New(5*time.Second, 0))
	c.baseURL = srv.URL
	c.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	return srv, c
}

func mustChain(caip2 string) id.Chain {
	ch, err := id.ParseChain(caip2)
	if err != nil {
		panic(fmt.Sprintf("mustChain(%q): %v", caip2, err))
	}
	return ch
}

// ─── Info ─────────────────────────────────────────────────────────────────────

func TestInfo(t *testing.T) {
	c := New(httpx.New(5*time.Second, 0))
	info := c.Info()
	if info.Name != "pendle" {
		t.Fatalf("name: want pendle, got %q", info.Name)
	}
	if info.RequiresKey {
		t.Fatal("pendle should not require an API key")
	}
	caps := make(map[string]bool)
	for _, cap := range info.Capabilities {
		caps[cap] = true
	}
	for _, want := range []string{"yield.opportunities", "yield.positions"} {
		if !caps[want] {
			t.Errorf("missing capability %q", want)
		}
	}
}

// ─── YieldOpportunities ───────────────────────────────────────────────────────

func TestYieldOpportunitiesReturnsBothTypesPerMarket(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	opps, err := c.YieldOpportunities(context.Background(), providers.YieldRequest{
		Chain: mustChain("eip155:1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// 2 markets × 2 types (pt + lp) = 4 opportunities.
	if len(opps) != 4 {
		t.Fatalf("expected 4 opportunities, got %d: %s", len(opps), mustMarshal(opps))
	}
}

func TestYieldOpportunitiesSortedByAPYDesc(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	opps, err := c.YieldOpportunities(context.Background(), providers.YieldRequest{
		Chain: mustChain("eip155:1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(opps); i++ {
		if opps[i].APYTotal > opps[i-1].APYTotal {
			t.Errorf("not sorted desc: [%d]=%.2f > [%d]=%.2f",
				i, opps[i].APYTotal, i-1, opps[i-1].APYTotal)
		}
	}
}

func TestYieldOpportunitiesPTFields(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	opps, err := c.YieldOpportunities(context.Background(), providers.YieldRequest{
		Chain: mustChain("eip155:1"),
	})
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, o := range opps {
		if o.Type == "fixed" && strings.Contains(strings.ToLower(o.ProviderNativeID), "market2") {
			// USDC PT: impliedAPY=6.0
			if o.APYTotal != 6.0 {
				t.Errorf("USDC PT APYTotal: want 6.0, got %.2f", o.APYTotal)
			}
			if o.LockupDays <= 0 {
				t.Errorf("PT should have positive lockup days, got %.2f", o.LockupDays)
			}
			if o.WithdrawalTerms != "fixed-maturity" {
				t.Errorf("PT withdrawal_terms: want fixed-maturity, got %q", o.WithdrawalTerms)
			}
			found = true
		}
	}
	if !found {
		t.Error("USDC PT opportunity not found")
	}
}

func TestYieldOpportunitiesLPFields(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	opps, err := c.YieldOpportunities(context.Background(), providers.YieldRequest{
		Chain: mustChain("eip155:1"),
	})
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, o := range opps {
		if o.Type == "lp" && strings.Contains(strings.ToLower(o.ProviderNativeID), "market1") {
			// weETH LP: aggregatedAPY=4.3
			if o.APYTotal != 4.3 {
				t.Errorf("weETH LP APYTotal: want 4.3, got %.2f", o.APYTotal)
			}
			if o.LockupDays != 0 {
				t.Errorf("LP lockup should be 0, got %.2f", o.LockupDays)
			}
			if o.WithdrawalTerms != "instant" {
				t.Errorf("LP withdrawal_terms: want instant, got %q", o.WithdrawalTerms)
			}
			found = true
		}
	}
	if !found {
		t.Error("weETH LP opportunity not found")
	}
}

func TestYieldOpportunitiesAssetFilterBySymbol(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	opps, err := c.YieldOpportunities(context.Background(), providers.YieldRequest{
		Chain: mustChain("eip155:1"),
		Asset: id.Asset{Symbol: "USDC"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Only the USDC market → PT + LP = 2.
	if len(opps) != 2 {
		t.Fatalf("expected 2 USDC opportunities, got %d: %s", len(opps), mustMarshal(opps))
	}
	for _, o := range opps {
		if !strings.Contains(o.AssetID, "0xusdc") {
			t.Errorf("unexpected assetID %q in USDC filter result", o.AssetID)
		}
	}
}

func TestYieldOpportunitiesMinTVLFilter(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	// Only weETH (50M) passes; USDC (20M) is filtered out.
	opps, err := c.YieldOpportunities(context.Background(), providers.YieldRequest{
		Chain:     mustChain("eip155:1"),
		MinTVLUSD: 30_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(opps) != 2 {
		t.Fatalf("expected 2 (weETH PT+LP), got %d", len(opps))
	}
}

func TestYieldOpportunitiesLimit(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	opps, err := c.YieldOpportunities(context.Background(), providers.YieldRequest{
		Chain: mustChain("eip155:1"),
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(opps) != 1 {
		t.Fatalf("expected 1, got %d", len(opps))
	}
}

func TestYieldOpportunitiesNonEVMRejected(t *testing.T) {
	c := New(httpx.New(5*time.Second, 0))
	sol, _ := id.ParseChain("solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp")
	_, err := c.YieldOpportunities(context.Background(), providers.YieldRequest{Chain: sol})
	if err == nil {
		t.Fatal("expected error for non-EVM chain")
	}
}

func TestYieldOpportunitiesUnsupportedChainRejected(t *testing.T) {
	c := New(httpx.New(5*time.Second, 0))
	_, err := c.YieldOpportunities(context.Background(), providers.YieldRequest{
		Chain: mustChain("eip155:999999"),
	})
	if err == nil {
		t.Fatal("expected error for unsupported chain")
	}
}

// ─── OpportunityID determinism ────────────────────────────────────────────────

func TestOpportunityIDDeterministic(t *testing.T) {
	caip2 := "eip155:1"
	addr := "0xABCD"
	id1 := opportunityID(caip2, addr, "pt")
	id2 := opportunityID(caip2, addr, "pt")
	if id1 != id2 {
		t.Errorf("non-deterministic: %q vs %q", id1, id2)
	}
	if !strings.HasPrefix(id1, "pendle:eip155:1:") {
		t.Errorf("unexpected prefix in %q", id1)
	}
}

// ─── YieldPositions ───────────────────────────────────────────────────────────

func TestYieldPositionsNonZeroOnly(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	pos, err := c.YieldPositions(context.Background(), providers.YieldPositionsRequest{
		Chain:   mustChain("eip155:1"),
		Account: "0xalice",
	})
	if err != nil {
		t.Fatal(err)
	}
	// market1: PT only (YT=0, LP=0) → 1
	// market2: LP only (PT=0, YT=0) → 1
	if len(pos) != 2 {
		t.Fatalf("expected 2 positions, got %d: %s", len(pos), mustMarshal(pos))
	}
}

func TestYieldPositionsPTFields(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	pos, err := c.YieldPositions(context.Background(), providers.YieldPositionsRequest{
		Chain:   mustChain("eip155:1"),
		Account: "0xalice",
	})
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, p := range pos {
		if p.PositionType == "fixed" {
			if p.AmountUSD != 3400 {
				t.Errorf("PT AmountUSD: want 3400, got %.2f", p.AmountUSD)
			}
			if p.Amount.AmountBaseUnits != "1000000000000000000" {
				t.Errorf("PT base units: got %q", p.Amount.AmountBaseUnits)
			}
			if p.Amount.Decimals != 18 {
				t.Errorf("PT decimals: want 18, got %d", p.Amount.Decimals)
			}
			if p.APYTotal != 4.5 {
				t.Errorf("PT APY: want 4.5, got %.2f", p.APYTotal)
			}
			found = true
		}
	}
	if !found {
		t.Error("PT position not found")
	}
}

func TestYieldPositionsLPFields(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	pos, err := c.YieldPositions(context.Background(), providers.YieldPositionsRequest{
		Chain:   mustChain("eip155:1"),
		Account: "0xalice",
	})
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, p := range pos {
		if p.PositionType == "lp" {
			if p.AmountUSD != 1500 {
				t.Errorf("LP AmountUSD: want 1500, got %.2f", p.AmountUSD)
			}
			if p.APYTotal != 5.8 {
				t.Errorf("LP APY: want 5.8, got %.2f", p.APYTotal)
			}
			found = true
		}
	}
	if !found {
		t.Error("LP position not found")
	}
}

func TestYieldPositionsSortedByUSDDesc(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	pos, err := c.YieldPositions(context.Background(), providers.YieldPositionsRequest{
		Chain:   mustChain("eip155:1"),
		Account: "0xalice",
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(pos); i++ {
		if pos[i].AmountUSD > pos[i-1].AmountUSD {
			t.Errorf("not sorted desc: [%d]=%.2f > [%d]=%.2f",
				i, pos[i].AmountUSD, i-1, pos[i-1].AmountUSD)
		}
	}
}

func TestYieldPositionsMissingAddressError(t *testing.T) {
	c := New(httpx.New(5*time.Second, 0))
	_, err := c.YieldPositions(context.Background(), providers.YieldPositionsRequest{
		Chain: mustChain("eip155:1"),
		// Account intentionally omitted
	})
	if err == nil {
		t.Fatal("expected error when account address is empty")
	}
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func TestBaseToDecimal(t *testing.T) {
	cases := []struct {
		in  string
		dec int
		out string
	}{
		{"1000000000000000000", 18, "1"},
		{"500000000000000000", 18, "0.5"},
		{"1500000000000000000", 18, "1.5"},
		{"1000000", 6, "1"},
		{"500000", 6, "0.5"},
		{"0", 18, "0"},
		{"", 18, "0"},
	}
	for _, tc := range cases {
		got := baseToDecimal(tc.in, tc.dec)
		if got != tc.out {
			t.Errorf("baseToDecimal(%q,%d) = %q, want %q", tc.in, tc.dec, got, tc.out)
		}
	}
}

func TestNonZeroAmount(t *testing.T) {
	if nonZeroAmount("0") {
		t.Error("0 should be zero")
	}
	if nonZeroAmount("") {
		t.Error("empty should be zero")
	}
	if !nonZeroAmount("1") {
		t.Error("1 should be non-zero")
	}
	if !nonZeroAmount("1000000000000000000") {
		t.Error("1e18 should be non-zero")
	}
}

// ─── Helper ───────────────────────────────────────────────────────────────────

func mustMarshal(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
