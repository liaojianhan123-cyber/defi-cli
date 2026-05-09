package compoundv3

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ggonzalez94/defi-cli/internal/id"
	"github.com/ggonzalez94/defi-cli/internal/model"
	"github.com/ggonzalez94/defi-cli/internal/providers"
)

// ── Test fixtures ───────────────────────────────────────────────────────
//
// Scroll only has a single canonical Comet deployment in the registry
// (cUSDCv3 at 0xB2f97...CE44), which keeps the mock dispatch tractable.
// All on-chain values below are illustrative — they do not need to match
// real mainnet state.
const scrollChainID = int64(534352)

var (
	scrollComet     = common.HexToAddress("0xB2f97c1Bd3bf02f5e74d13f02E3e26F93D77CE44")
	scrollUSDC      = common.HexToAddress("0x06eFdBFf2a14a7c8E15944D1F4A48F9F95F663A4")
	scrollPriceFeed = common.HexToAddress("0x0000000000000000000000000000000000000F1A")

	// Collateral asset registered on the Comet (illustrative).
	scrollWETH      = common.HexToAddress("0x5300000000000000000000000000000000000004")
	scrollWETHFeed  = common.HexToAddress("0x0000000000000000000000000000000000000E72")

	testAccount = common.HexToAddress("0x000000000000000000000000000000000000dEaD")
)

// ── JSON-RPC plumbing ───────────────────────────────────────────────────

type jsonRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      interface{}   `json:"id"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  string      `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

func selectorHex(a abi.ABI, method string) string {
	m, ok := a.Methods[method]
	if !ok {
		return ""
	}
	return hex.EncodeToString(m.ID)
}

func packOutput(sig string, vals ...interface{}) string {
	a, _ := abi.JSON(strings.NewReader(sig))
	out, _ := a.Methods["f"].Outputs.Pack(vals...)
	return "0x" + hex.EncodeToString(out)
}

func encodeAddress(addr common.Address) string {
	return packOutput(`[{"name":"f","type":"function","outputs":[{"type":"address"}]}]`, addr)
}

func encodeUint8(v uint8) string {
	return packOutput(`[{"name":"f","type":"function","outputs":[{"type":"uint8"}]}]`, v)
}

func encodeUint64(v uint64) string {
	return packOutput(`[{"name":"f","type":"function","outputs":[{"type":"uint64"}]}]`, v)
}

func encodeUint256(v *big.Int) string {
	return packOutput(`[{"name":"f","type":"function","outputs":[{"type":"uint256"}]}]`, v)
}

func encodeString(s string) string {
	return packOutput(`[{"name":"f","type":"function","outputs":[{"type":"string"}]}]`, s)
}

// encodeUserCollateral encodes (uint128 balance, uint128 _reserved).
func encodeUserCollateral(balance *big.Int) string {
	return packOutput(
		`[{"name":"f","type":"function","outputs":[{"type":"uint128"},{"type":"uint128"}]}]`,
		balance, big.NewInt(0),
	)
}

// encodeAssetInfo encodes the AssetInfo tuple returned by getAssetInfo(uint8).
func encodeAssetInfo(asset, priceFeed common.Address) string {
	return packOutput(
		`[{"name":"f","type":"function","outputs":[{"name":"info","type":"tuple","components":[{"name":"offset","type":"uint8"},{"name":"asset","type":"address"},{"name":"priceFeed","type":"address"},{"name":"scale","type":"uint64"},{"name":"borrowCollateralFactor","type":"uint64"},{"name":"liquidateCollateralFactor","type":"uint64"},{"name":"liquidationFactor","type":"uint64"},{"name":"supplyCap","type":"uint128"}]}]}]`,
		struct {
			Offset                    uint8
			Asset                     common.Address
			PriceFeed                 common.Address
			Scale                     uint64
			BorrowCollateralFactor    uint64
			LiquidateCollateralFactor uint64
			LiquidationFactor         uint64
			SupplyCap                 *big.Int
		}{
			Offset:                    0,
			Asset:                     asset,
			PriceFeed:                 priceFeed,
			Scale:                     1e18,
			BorrowCollateralFactor:    8e17,
			LiquidateCollateralFactor: 9e17,
			LiquidationFactor:         95e16,
			SupplyCap:                 new(big.Int).Mul(big.NewInt(1e9), big.NewInt(1e18)),
		},
	)
}

type fixture struct {
	supplyRate   uint64
	borrowRate   uint64
	utilization  *big.Int
	totalSupply  *big.Int
	totalBorrow  *big.Int
	basePrice    *big.Int // 8-decimal USD
	wethPrice    *big.Int
	supplyBal    *big.Int
	borrowBal    *big.Int
	wethCollBal  *big.Int
	numAssets    uint8
}

func defaultFixture() fixture {
	return fixture{
		supplyRate:  1_000_000_000,                                  // ~3.15% APR
		borrowRate:  1_500_000_000,                                  // ~4.7% APR
		utilization: new(big.Int).Mul(big.NewInt(7), big.NewInt(1e17)), // 0.7
		totalSupply: new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1e6)), // 1M USDC
		totalBorrow: new(big.Int).Mul(big.NewInt(700_000), big.NewInt(1e6)),   // 700k USDC
		basePrice:   new(big.Int).Mul(big.NewInt(1), big.NewInt(1e8)),         // $1.00
		wethPrice:   new(big.Int).Mul(big.NewInt(3000), big.NewInt(1e8)),      // $3000
		supplyBal:   new(big.Int).Mul(big.NewInt(10_000), big.NewInt(1e6)),    // 10k USDC supplied
		borrowBal:   new(big.Int).Mul(big.NewInt(2_000), big.NewInt(1e6)),     // 2k USDC borrowed
		wethCollBal: new(big.Int).Mul(big.NewInt(5), big.NewInt(1e18)),        // 5 WETH collateral
		numAssets:   1,
	}
}

// dispatchSingleCall resolves one eth_call against the canonical Comet/ERC20
// surface defined above. Returns "0x" if nothing matches (caller may treat
// that as Success=false in multicall results).
func dispatchSingleCall(to string, dataHex string, sels selectors, fx fixture) string {
	selector := ""
	if len(dataHex) >= 8 {
		selector = dataHex[:8]
	}
	to = strings.ToLower(to)

	switch {
	case to == strings.ToLower(scrollComet.Hex()):
		switch selector {
		case sels.comet["baseToken"]:
			return encodeAddress(scrollUSDC)
		case sels.comet["baseTokenPriceFeed"]:
			return encodeAddress(scrollPriceFeed)
		case sels.comet["getUtilization"]:
			return encodeUint256(fx.utilization)
		case sels.comet["getSupplyRate"]:
			return encodeUint64(fx.supplyRate)
		case sels.comet["getBorrowRate"]:
			return encodeUint64(fx.borrowRate)
		case sels.comet["totalSupply"]:
			return encodeUint256(fx.totalSupply)
		case sels.comet["totalBorrow"]:
			return encodeUint256(fx.totalBorrow)
		case sels.comet["balanceOf"]:
			return encodeUint256(fx.supplyBal)
		case sels.comet["borrowBalanceOf"]:
			return encodeUint256(fx.borrowBal)
		case sels.comet["numAssets"]:
			return encodeUint8(fx.numAssets)
		case sels.comet["getAssetInfo"]:
			return encodeAssetInfo(scrollWETH, scrollWETHFeed)
		case sels.comet["userCollateral"]:
			return encodeUserCollateral(fx.wethCollBal)
		case sels.comet["getPrice"]:
			// Distinguish base price feed vs WETH price feed by inspecting the
			// argument (last 20 bytes of the 32-byte address slot).
			if len(dataHex) >= 8+64 {
				arg := dataHex[8 : 8+64]
				addr := "0x" + arg[24:]
				if strings.EqualFold(addr, scrollWETHFeed.Hex()) {
					return encodeUint256(fx.wethPrice)
				}
			}
			return encodeUint256(fx.basePrice)
		}
	case to == strings.ToLower(scrollUSDC.Hex()):
		switch selector {
		case sels.erc20["symbol"]:
			return encodeString("USDC")
		case sels.erc20["decimals"]:
			return encodeUint8(6)
		}
	case to == strings.ToLower(scrollWETH.Hex()):
		switch selector {
		case sels.erc20["symbol"]:
			return encodeString("WETH")
		case sels.erc20["decimals"]:
			return encodeUint8(18)
		}
	}
	return "0x"
}

type selectors struct {
	comet map[string]string
	erc20 map[string]string
	mc3   string
}

func buildSelectors() selectors {
	return selectors{
		comet: map[string]string{
			"baseToken":          selectorHex(cometABI, "baseToken"),
			"baseTokenPriceFeed": selectorHex(cometABI, "baseTokenPriceFeed"),
			"getUtilization":     selectorHex(cometABI, "getUtilization"),
			"getSupplyRate":      selectorHex(cometABI, "getSupplyRate"),
			"getBorrowRate":      selectorHex(cometABI, "getBorrowRate"),
			"totalSupply":        selectorHex(cometABI, "totalSupply"),
			"totalBorrow":        selectorHex(cometABI, "totalBorrow"),
			"balanceOf":          selectorHex(cometABI, "balanceOf"),
			"borrowBalanceOf":    selectorHex(cometABI, "borrowBalanceOf"),
			"numAssets":          selectorHex(cometABI, "numAssets"),
			"getAssetInfo":       selectorHex(cometABI, "getAssetInfo"),
			"userCollateral":     selectorHex(cometABI, "userCollateral"),
			"getPrice":           selectorHex(cometABI, "getPrice"),
		},
		erc20: map[string]string{
			"symbol":   selectorHex(erc20ABI, "symbol"),
			"decimals": selectorHex(erc20ABI, "decimals"),
		},
		mc3: selectorHex(mc3ABI, "aggregate3"),
	}
}

func newTestRPCServer(t *testing.T, fx fixture) *httptest.Server {
	t.Helper()
	sels := buildSelectors()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if req.Method != "eth_call" {
			_ = json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: "0x"})
			return
		}

		params, ok := req.Params[0].(map[string]interface{})
		if !ok {
			_ = json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: "0x"})
			return
		}
		dataHex, _ := params["data"].(string)
		if dataHex == "" {
			dataHex, _ = params["input"].(string)
		}
		toHex, _ := params["to"].(string)
		dataHex = strings.TrimPrefix(dataHex, "0x")
		selector := ""
		if len(dataHex) >= 8 {
			selector = dataHex[:8]
		}
		to := strings.ToLower(toHex)

		// Multicall3.aggregate3: decode Call3[], dispatch each, re-encode Result[].
		if to == strings.ToLower(multicall3Addr.Hex()) && selector == sels.mc3 {
			rawData, _ := hex.DecodeString(dataHex)
			decoded, err := mc3ABI.Methods["aggregate3"].Inputs.Unpack(rawData[4:])
			if err != nil {
				t.Logf("aggregate3 unpack error: %v", err)
				_ = json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: "0x"})
				return
			}
			calls := decoded[0].([]struct {
				Target       common.Address `json:"target"`
				AllowFailure bool           `json:"allowFailure"`
				CallData     []byte         `json:"callData"`
			})

			type mc3Result struct {
				Success    bool
				ReturnData []byte
			}
			results := make([]mc3Result, len(calls))
			for i, call := range calls {
				subData := hex.EncodeToString(call.CallData)
				subResult := dispatchSingleCall(call.Target.Hex(), subData, sels, fx)
				subBytes, _ := hex.DecodeString(strings.TrimPrefix(subResult, "0x"))
				results[i] = mc3Result{Success: subResult != "0x", ReturnData: subBytes}
			}

			encoded, err := mc3ABI.Methods["aggregate3"].Outputs.Pack(results)
			if err != nil {
				t.Logf("aggregate3 pack error: %v", err)
				_ = json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: "0x"})
				return
			}
			_ = json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: "0x" + hex.EncodeToString(encoded)})
			return
		}

		// Direct (non-multicall) calls.
		result := dispatchSingleCall(to, dataHex, sels, fx)
		_ = json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
	}))
}

// ── Tests ───────────────────────────────────────────────────────────────

func newClient(rpc string) *Client {
	c := New()
	c.SetRPCOverride(rpc)
	c.now = func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }
	return c
}

func scrollChain() id.Chain {
	return id.Chain{Name: "Scroll", Slug: "scroll", CAIP2: "eip155:534352", EVMChainID: scrollChainID}
}

func TestInfoSurfacesCapabilities(t *testing.T) {
	info := New().Info()
	if info.Name != "compoundv3" {
		t.Fatalf("expected provider name compoundv3, got %q", info.Name)
	}
	if info.RequiresKey {
		t.Fatalf("compoundv3 should not require an API key")
	}
	wantCaps := map[string]bool{
		"lend.markets":        true,
		"lend.rates":          true,
		"lend.positions":      true,
		"yield.opportunities": true,
		"yield.positions":     true,
	}
	for _, c := range info.Capabilities {
		delete(wantCaps, c)
	}
	if len(wantCaps) != 0 {
		t.Fatalf("missing expected capabilities: %v", wantCaps)
	}
}

func TestLendMarketsReturnsBaseAsset(t *testing.T) {
	srv := newTestRPCServer(t, defaultFixture())
	defer srv.Close()

	markets, err := newClient(srv.URL).LendMarkets(context.Background(), "compoundv3", scrollChain(), id.Asset{Symbol: "USDC", ChainID: "eip155:534352"})
	if err != nil {
		t.Fatalf("LendMarkets failed: %v", err)
	}
	if len(markets) != 1 {
		t.Fatalf("expected 1 market, got %d", len(markets))
	}
	m := markets[0]
	if m.Provider != "compoundv3" || m.Protocol != "compoundv3" {
		t.Fatalf("expected compoundv3 protocol/provider, got %+v", m)
	}
	if m.ChainID != "eip155:534352" {
		t.Fatalf("expected scroll chain id, got %q", m.ChainID)
	}
	if m.AssetID != "eip155:534352/erc20:"+strings.ToLower(scrollUSDC.Hex()) {
		t.Fatalf("unexpected asset id %q", m.AssetID)
	}
	if m.ProviderNativeIDKind != model.NativeIDKindCompositeMarketAsset {
		t.Fatalf("expected composite_market_asset native id kind, got %q", m.ProviderNativeIDKind)
	}
	if m.SupplyAPY <= 0 || m.BorrowAPY <= 0 {
		t.Fatalf("expected positive APYs, got supply=%v borrow=%v", m.SupplyAPY, m.BorrowAPY)
	}
	if m.SupplyAPY >= m.BorrowAPY {
		t.Fatalf("expected supply APY < borrow APY, got supply=%v borrow=%v", m.SupplyAPY, m.BorrowAPY)
	}
	// TVL = 1M USDC * $1 = $1M; liquidity = (1M - 700k) * $1 = $300k.
	if m.TVLUSD < 999_000 || m.TVLUSD > 1_001_000 {
		t.Fatalf("expected TVL around $1M, got %v", m.TVLUSD)
	}
	if m.LiquidityUSD < 299_000 || m.LiquidityUSD > 301_000 {
		t.Fatalf("expected liquidity around $300k, got %v", m.LiquidityUSD)
	}
}

func TestLendRatesReportsUtilization(t *testing.T) {
	srv := newTestRPCServer(t, defaultFixture())
	defer srv.Close()

	rates, err := newClient(srv.URL).LendRates(context.Background(), "compoundv3", scrollChain(), id.Asset{})
	if err != nil {
		t.Fatalf("LendRates failed: %v", err)
	}
	if len(rates) != 1 {
		t.Fatalf("expected 1 rate row, got %d", len(rates))
	}
	r := rates[0]
	if r.Utilization < 0.69 || r.Utilization > 0.71 {
		t.Fatalf("expected utilization ~0.7, got %v", r.Utilization)
	}
}

func TestYieldOpportunitiesReturnsBaseAsset(t *testing.T) {
	srv := newTestRPCServer(t, defaultFixture())
	defer srv.Close()

	opps, err := newClient(srv.URL).YieldOpportunities(context.Background(), providers.YieldRequest{
		Chain: scrollChain(),
		Asset: id.Asset{Symbol: "USDC", ChainID: "eip155:534352"},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("YieldOpportunities failed: %v", err)
	}
	if len(opps) != 1 {
		t.Fatalf("expected 1 opportunity, got %d", len(opps))
	}
	o := opps[0]
	if o.Type != "lend" {
		t.Fatalf("expected type=lend, got %q", o.Type)
	}
	if o.APYTotal <= 0 || o.APYTotal != o.APYBase {
		t.Fatalf("expected APYTotal == APYBase > 0, got total=%v base=%v", o.APYTotal, o.APYBase)
	}
	if len(o.BackingAssets) != 1 || o.BackingAssets[0].Symbol != "USDC" {
		t.Fatalf("expected USDC backing, got %+v", o.BackingAssets)
	}
}

func TestLendPositionsSurfacesSupplyBorrowCollateral(t *testing.T) {
	srv := newTestRPCServer(t, defaultFixture())
	defer srv.Close()

	positions, err := newClient(srv.URL).LendPositions(context.Background(), providers.LendPositionsRequest{
		Chain:        scrollChain(),
		Account:      strings.ToLower(testAccount.Hex()),
		PositionType: providers.LendPositionTypeAll,
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("LendPositions failed: %v", err)
	}
	if len(positions) != 3 {
		t.Fatalf("expected 3 positions (supply+borrow+collateral), got %d: %+v", len(positions), positions)
	}

	seen := map[string]model.LendPosition{}
	for _, p := range positions {
		seen[p.PositionType] = p
	}

	supply, ok := seen["supply"]
	if !ok {
		t.Fatalf("missing supply position; got %+v", positions)
	}
	if supply.AmountUSD < 9_900 || supply.AmountUSD > 10_100 {
		t.Fatalf("expected supply ~ $10k, got %v", supply.AmountUSD)
	}
	if supply.APY <= 0 {
		t.Fatalf("expected supply APY > 0, got %v", supply.APY)
	}

	borrow, ok := seen["borrow"]
	if !ok {
		t.Fatalf("missing borrow position; got %+v", positions)
	}
	if borrow.AmountUSD < 1_900 || borrow.AmountUSD > 2_100 {
		t.Fatalf("expected borrow ~ $2k, got %v", borrow.AmountUSD)
	}
	if borrow.APY <= supply.APY {
		t.Fatalf("expected borrow APY > supply APY, got borrow=%v supply=%v", borrow.APY, supply.APY)
	}

	collateral, ok := seen["collateral"]
	if !ok {
		t.Fatalf("missing collateral position; got %+v", positions)
	}
	// 5 WETH * $3000 = $15,000.
	if collateral.AmountUSD < 14_900 || collateral.AmountUSD > 15_100 {
		t.Fatalf("expected collateral ~ $15k, got %v", collateral.AmountUSD)
	}
	if collateral.APY != 0 {
		t.Fatalf("collateral should not earn yield in compound v3, got APY=%v", collateral.APY)
	}
	if !strings.Contains(strings.ToLower(collateral.AssetID), strings.ToLower(scrollWETH.Hex()[2:])) {
		t.Fatalf("expected WETH collateral asset, got %q", collateral.AssetID)
	}
}

func TestLendPositionsFilterType(t *testing.T) {
	srv := newTestRPCServer(t, defaultFixture())
	defer srv.Close()

	positions, err := newClient(srv.URL).LendPositions(context.Background(), providers.LendPositionsRequest{
		Chain:        scrollChain(),
		Account:      strings.ToLower(testAccount.Hex()),
		PositionType: providers.LendPositionTypeBorrow,
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("LendPositions failed: %v", err)
	}
	if len(positions) != 1 || positions[0].PositionType != "borrow" {
		t.Fatalf("expected single borrow position, got %+v", positions)
	}
}

func TestYieldPositionsOnlyReturnsSupply(t *testing.T) {
	srv := newTestRPCServer(t, defaultFixture())
	defer srv.Close()

	rows, err := newClient(srv.URL).YieldPositions(context.Background(), providers.YieldPositionsRequest{
		Chain:   scrollChain(),
		Account: strings.ToLower(testAccount.Hex()),
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("YieldPositions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 yield position, got %d", len(rows))
	}
	if rows[0].PositionType != "deposit" {
		t.Fatalf("expected position_type=deposit, got %q", rows[0].PositionType)
	}
	if rows[0].AmountUSD < 9_900 || rows[0].AmountUSD > 10_100 {
		t.Fatalf("expected ~$10k yield position, got %v", rows[0].AmountUSD)
	}
}

func TestLendMarketsRejectsNonEVM(t *testing.T) {
	_, err := newClient("").LendMarkets(context.Background(), "compoundv3", id.Chain{CAIP2: "solana:5eykt", Name: "Solana"}, id.Asset{})
	if err == nil {
		t.Fatalf("expected unsupported error for non-EVM chain")
	}
}

func TestLendMarketsUnsupportedChain(t *testing.T) {
	// Chain ID 12345 is not registered in CompoundV3Markets.
	_, err := newClient("").LendMarkets(context.Background(), "compoundv3", id.Chain{CAIP2: "eip155:12345", EVMChainID: 12345}, id.Asset{})
	if err == nil {
		t.Fatalf("expected unsupported error for unregistered chain")
	}
}
