// Package compoundv3 implements a read-only adapter for Compound V3 (Comet)
// lending markets, rates, and positions.
//
// Compound V3 differs from Aave/Compound V2 in that each Comet contract
// manages exactly one base asset: that asset is the only one that can be
// supplied for yield or borrowed. All other assets in the same Comet serve
// only as collateral (no yield, no borrow).
//
// This adapter therefore models each Comet as a single market keyed by its
// base asset. Collateral-only positions are still surfaced via
// LendPositions when an account holds collateral.
package compoundv3

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	clierr "github.com/ggonzalez94/defi-cli/internal/errors"
	"github.com/ggonzalez94/defi-cli/internal/id"
	"github.com/ggonzalez94/defi-cli/internal/model"
	"github.com/ggonzalez94/defi-cli/internal/providers"
	"github.com/ggonzalez94/defi-cli/internal/providers/yieldutil"
	"github.com/ggonzalez94/defi-cli/internal/registry"
)

const (
	secondsPerYear = 365.25 * 24 * 3600
	// rateScale is the per-second interest-rate denominator used by Comet
	// (FACTOR_SCALE = 1e18).
	rateScale = 1e18
	// priceScale is the USD price denominator used by Comet's getPrice
	// (Chainlink-style price feeds, 1e8).
	priceScale = 1e8
)

// multicall3Addr is the canonical Multicall3 deployment, present at the same
// address on every supported EVM chain.
var multicall3Addr = common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")

type multicall3Call struct {
	Target       common.Address
	AllowFailure bool
	CallData     []byte
}

type multicall3Result struct {
	Success    bool
	ReturnData []byte
}

// Client is a read-only Compound V3 (Comet) adapter.
type Client struct {
	now         func() time.Time
	rpcOverride string // used in tests to point at a mock RPC server
}

func New() *Client {
	return &Client{now: time.Now}
}

// SetRPCOverride sets the RPC URL used for on-chain reads. Pass "" to revert
// to the default chain RPC. Intended for tests.
func (c *Client) SetRPCOverride(url string) { c.rpcOverride = url }

func (c *Client) Info() model.ProviderInfo {
	return model.ProviderInfo{
		Name:        "compoundv3",
		Type:        "lending+yield",
		RequiresKey: false,
		Capabilities: []string{
			"lend.markets",
			"lend.rates",
			"lend.positions",
			"yield.opportunities",
			"yield.positions",
			"lend.plan",
			"lend.execute",
			"yield.plan",
			"yield.execute",
		},
	}
}

// ── internal market struct ──────────────────────────────────────────────

type cometMarket struct {
	Comet              string
	Label              string
	BaseAsset          string // underlying address (lowercased)
	BaseSymbol         string
	BaseDecimals       int
	BasePriceUSD       float64
	SupplyAPY          float64 // percentage points
	BorrowAPY          float64
	Utilization        float64 // 0..1
	TotalSupplyBase    *big.Int
	TotalBorrowBase    *big.Int
	TVLUSD             float64
	TotalBorrowsUSD    float64
	LiquidityUSD       float64
}

// ── LendingProvider ─────────────────────────────────────────────────────

func (c *Client) LendMarkets(ctx context.Context, _ string, chain id.Chain, asset id.Asset) ([]model.LendMarket, error) {
	markets, err := c.fetchMarkets(ctx, chain)
	if err != nil {
		return nil, err
	}

	out := make([]model.LendMarket, 0, len(markets))
	for _, m := range markets {
		if !matchesAsset(m.BaseAsset, m.BaseSymbol, asset) {
			continue
		}
		assetID := canonicalAssetIDForChain(chain.CAIP2, m.BaseAsset)
		if assetID == "" {
			continue
		}
		out = append(out, model.LendMarket{
			Protocol:             "compoundv3",
			Provider:             "compoundv3",
			ChainID:              chain.CAIP2,
			AssetID:              assetID,
			ProviderNativeID:     providerNativeID(chain.CAIP2, m.Comet, m.BaseAsset),
			ProviderNativeIDKind: model.NativeIDKindCompositeMarketAsset,
			SupplyAPY:            m.SupplyAPY,
			BorrowAPY:            m.BorrowAPY,
			TVLUSD:               m.TVLUSD,
			LiquidityUSD:         m.LiquidityUSD,
			SourceURL:            "https://app.compound.finance",
			FetchedAt:            c.now().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].TVLUSD != out[j].TVLUSD {
			return out[i].TVLUSD > out[j].TVLUSD
		}
		return out[i].AssetID < out[j].AssetID
	})
	return out, nil
}

func (c *Client) LendRates(ctx context.Context, _ string, chain id.Chain, asset id.Asset) ([]model.LendRate, error) {
	markets, err := c.fetchMarkets(ctx, chain)
	if err != nil {
		return nil, err
	}

	out := make([]model.LendRate, 0, len(markets))
	for _, m := range markets {
		if !matchesAsset(m.BaseAsset, m.BaseSymbol, asset) {
			continue
		}
		assetID := canonicalAssetIDForChain(chain.CAIP2, m.BaseAsset)
		if assetID == "" {
			continue
		}
		out = append(out, model.LendRate{
			Protocol:             "compoundv3",
			Provider:             "compoundv3",
			ChainID:              chain.CAIP2,
			AssetID:              assetID,
			ProviderNativeID:     providerNativeID(chain.CAIP2, m.Comet, m.BaseAsset),
			ProviderNativeIDKind: model.NativeIDKindCompositeMarketAsset,
			SupplyAPY:            m.SupplyAPY,
			BorrowAPY:            m.BorrowAPY,
			Utilization:          m.Utilization,
			SourceURL:            "https://app.compound.finance",
			FetchedAt:            c.now().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].SupplyAPY != out[j].SupplyAPY {
			return out[i].SupplyAPY > out[j].SupplyAPY
		}
		return out[i].AssetID < out[j].AssetID
	})
	return out, nil
}

// ── YieldProvider ───────────────────────────────────────────────────────

func (c *Client) YieldOpportunities(ctx context.Context, req providers.YieldRequest) ([]model.YieldOpportunity, error) {
	markets, err := c.fetchMarkets(ctx, req.Chain)
	if err != nil {
		return nil, err
	}

	out := make([]model.YieldOpportunity, 0, len(markets))
	for _, m := range markets {
		if !matchesAsset(m.BaseAsset, m.BaseSymbol, req.Asset) {
			continue
		}
		if (m.SupplyAPY == 0 || m.TVLUSD == 0) && !req.IncludeIncomplete {
			continue
		}
		if m.SupplyAPY < req.MinAPY {
			continue
		}
		if m.TVLUSD < req.MinTVLUSD {
			continue
		}
		assetID := canonicalAssetIDForChain(req.Chain.CAIP2, m.BaseAsset)
		if assetID == "" {
			continue
		}
		nativeID := providerNativeID(req.Chain.CAIP2, m.Comet, m.BaseAsset)
		opportunityID := hashOpportunity("compoundv3", req.Chain.CAIP2, nativeID, assetID)

		out = append(out, model.YieldOpportunity{
			OpportunityID:        opportunityID,
			Provider:             "compoundv3",
			Protocol:             "compoundv3",
			ChainID:              req.Chain.CAIP2,
			AssetID:              assetID,
			ProviderNativeID:     nativeID,
			ProviderNativeIDKind: model.NativeIDKindCompositeMarketAsset,
			Type:                 "lend",
			APYBase:              m.SupplyAPY,
			APYReward:            0,
			APYTotal:             m.SupplyAPY,
			TVLUSD:               m.TVLUSD,
			LiquidityUSD:         m.LiquidityUSD,
			LockupDays:           0,
			WithdrawalTerms:      "variable",
			BackingAssets: []model.YieldBackingAsset{{
				AssetID:  assetID,
				Symbol:   m.BaseSymbol,
				SharePct: 100,
			}},
			SourceURL: "https://app.compound.finance",
			FetchedAt: c.now().UTC().Format(time.RFC3339),
		})
	}

	if len(out) == 0 {
		return nil, clierr.New(clierr.CodeUnavailable, "no compoundv3 yield opportunities for requested chain/asset")
	}
	yieldutil.Sort(out, req.SortBy)
	if req.Limit <= 0 || req.Limit > len(out) {
		req.Limit = len(out)
	}
	return out[:req.Limit], nil
}

// ── LendingPositionsProvider ────────────────────────────────────────────

func (c *Client) LendPositions(ctx context.Context, req providers.LendPositionsRequest) ([]model.LendPosition, error) {
	if !req.Chain.IsEVM() {
		return nil, clierr.New(clierr.CodeUnsupported, "compoundv3 supports only EVM chains")
	}
	account := normalizeEVMAddress(req.Account)
	if account == "" {
		return nil, clierr.New(clierr.CodeUsage, "lend positions requires a valid EVM address")
	}

	rpcURL, err := c.resolveRPC(req.Chain.EVMChainID, req.RPCURL)
	if err != nil {
		return nil, err
	}
	deployments, ok := registry.CompoundV3Markets(req.Chain.EVMChainID)
	if !ok {
		return nil, clierr.New(clierr.CodeUnsupported, "compoundv3 is not supported on this chain")
	}

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "connect rpc", err)
	}
	defer client.Close()

	accountAddr := common.HexToAddress(account)

	// Phase 1: per Comet — baseToken, baseTokenPriceFeed, balanceOf, borrowBalanceOf, numAssets.
	const callsPerCometPhase1 = 5
	phase1Calls := make([]multicall3Call, 0, len(deployments)*callsPerCometPhase1)
	baseTokenCD, _ := cometABI.Pack("baseToken")
	priceFeedCD, _ := cometABI.Pack("baseTokenPriceFeed")
	balanceOfCD, _ := cometABI.Pack("balanceOf", accountAddr)
	borrowOfCD, _ := cometABI.Pack("borrowBalanceOf", accountAddr)
	numAssetsCD, _ := cometABI.Pack("numAssets")

	for _, d := range deployments {
		comet := common.HexToAddress(d.Comet)
		phase1Calls = append(phase1Calls,
			multicall3Call{Target: comet, AllowFailure: true, CallData: baseTokenCD},
			multicall3Call{Target: comet, AllowFailure: true, CallData: priceFeedCD},
			multicall3Call{Target: comet, AllowFailure: true, CallData: balanceOfCD},
			multicall3Call{Target: comet, AllowFailure: true, CallData: borrowOfCD},
			multicall3Call{Target: comet, AllowFailure: true, CallData: numAssetsCD},
		)
	}
	p1, err := execMulticall3(ctx, client, phase1Calls)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "multicall positions phase1", err)
	}

	type cometCtx struct {
		comet      common.Address
		label      string
		baseToken  common.Address
		priceFeed  common.Address
		supplyBal  *big.Int
		borrowBal  *big.Int
		numAssets  uint8
	}

	ctxs := make([]cometCtx, 0, len(deployments))
	for i, d := range deployments {
		base := i * callsPerCometPhase1
		r := p1[base : base+callsPerCometPhase1]

		baseToken, ok := decodeAddress(r[0], cometABI, "baseToken")
		if !ok {
			continue
		}
		priceFeed, _ := decodeAddress(r[1], cometABI, "baseTokenPriceFeed")
		supplyBal := decodeUint256(r[2], cometABI, "balanceOf")
		borrowBal := decodeUint256(r[3], cometABI, "borrowBalanceOf")
		numAssets := decodeUint8(r[4], cometABI, "numAssets")

		// Skip Comets where the account has no exposure of any kind.
		if supplyBal.Sign() == 0 && borrowBal.Sign() == 0 && numAssets == 0 {
			continue
		}

		ctxs = append(ctxs, cometCtx{
			comet:     common.HexToAddress(d.Comet),
			label:     d.Label,
			baseToken: baseToken,
			priceFeed: priceFeed,
			supplyBal: supplyBal,
			borrowBal: borrowBal,
			numAssets: numAssets,
		})
	}

	if len(ctxs) == 0 {
		return []model.LendPosition{}, nil
	}

	// Phase 2: per Comet — getPrice(baseTokenPriceFeed) + getAssetInfo(0..n-1).
	phase2Calls := make([]multicall3Call, 0)
	type p2Layout struct {
		priceIdx     int
		assetInfoIdx []int
	}
	layouts := make([]p2Layout, len(ctxs))

	for i, cc := range ctxs {
		layout := p2Layout{}
		priceCD, _ := cometABI.Pack("getPrice", cc.priceFeed)
		layout.priceIdx = len(phase2Calls)
		phase2Calls = append(phase2Calls, multicall3Call{Target: cc.comet, AllowFailure: true, CallData: priceCD})
		for ai := uint8(0); ai < cc.numAssets; ai++ {
			cd, _ := cometABI.Pack("getAssetInfo", ai)
			layout.assetInfoIdx = append(layout.assetInfoIdx, len(phase2Calls))
			phase2Calls = append(phase2Calls, multicall3Call{Target: cc.comet, AllowFailure: true, CallData: cd})
		}
		layouts[i] = layout
	}

	p2, err := execMulticall3(ctx, client, phase2Calls)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "multicall positions phase2", err)
	}

	cometCollaterals := make([][]collateralAsset, len(ctxs))
	basePricesUSD := make([]float64, len(ctxs))

	for i := range ctxs {
		layout := layouts[i]
		basePricesUSD[i] = decodeUSDPrice(p2[layout.priceIdx], cometABI, "getPrice")
		assets := make([]collateralAsset, 0, len(layout.assetInfoIdx))
		for _, idx := range layout.assetInfoIdx {
			info, ok := decodeAssetInfo(p2[idx])
			if !ok {
				continue
			}
			assets = append(assets, info)
		}
		cometCollaterals[i] = assets
	}

	// Phase 3: per (Comet, collateralAsset) — userCollateral + getPrice(asset.priceFeed).
	type p3Slot struct {
		userCollIdx int
		priceIdx    int
	}
	phase3Calls := make([]multicall3Call, 0)
	cometSlots := make([][]p3Slot, len(ctxs))

	for i, cc := range ctxs {
		slots := make([]p3Slot, 0, len(cometCollaterals[i]))
		for _, asset := range cometCollaterals[i] {
			ucCD, _ := cometABI.Pack("userCollateral", accountAddr, asset.asset)
			pcCD, _ := cometABI.Pack("getPrice", asset.priceFeed)
			slot := p3Slot{
				userCollIdx: len(phase3Calls),
				priceIdx:    len(phase3Calls) + 1,
			}
			phase3Calls = append(phase3Calls,
				multicall3Call{Target: cc.comet, AllowFailure: true, CallData: ucCD},
				multicall3Call{Target: cc.comet, AllowFailure: true, CallData: pcCD},
			)
			slots = append(slots, slot)
		}
		cometSlots[i] = slots
	}

	var p3 []multicall3Result
	if len(phase3Calls) > 0 {
		p3, err = execMulticall3(ctx, client, phase3Calls)
		if err != nil {
			return nil, clierr.Wrap(clierr.CodeUnavailable, "multicall positions phase3", err)
		}
	}

	// Phase 4: per unique token (base + collateral with non-zero exposure) — symbol + decimals.
	uniqueTokens := make([]common.Address, 0)
	tokenIdx := make(map[common.Address]int)
	pushToken := func(addr common.Address) {
		if _, ok := tokenIdx[addr]; ok {
			return
		}
		tokenIdx[addr] = len(uniqueTokens)
		uniqueTokens = append(uniqueTokens, addr)
	}
	for i, cc := range ctxs {
		// We always need symbol/decimals for the base token if there is any base-side exposure.
		if cc.supplyBal.Sign() > 0 || cc.borrowBal.Sign() > 0 {
			pushToken(cc.baseToken)
		}
		for ai, asset := range cometCollaterals[i] {
			slot := cometSlots[i][ai]
			ucBalance := decodeUserCollateralBalance(p3[slot.userCollIdx])
			if ucBalance.Sign() > 0 {
				pushToken(asset.asset)
			}
		}
	}

	symbols := make(map[common.Address]string, len(uniqueTokens))
	decimals := make(map[common.Address]int, len(uniqueTokens))
	if len(uniqueTokens) > 0 {
		symbolCD, _ := erc20ABI.Pack("symbol")
		decimalsCD, _ := erc20ABI.Pack("decimals")
		phase4Calls := make([]multicall3Call, 0, len(uniqueTokens)*2)
		for _, t := range uniqueTokens {
			phase4Calls = append(phase4Calls,
				multicall3Call{Target: t, AllowFailure: true, CallData: symbolCD},
				multicall3Call{Target: t, AllowFailure: true, CallData: decimalsCD},
			)
		}
		p4, err := execMulticall3(ctx, client, phase4Calls)
		if err != nil {
			return nil, clierr.Wrap(clierr.CodeUnavailable, "multicall positions phase4", err)
		}
		for i, t := range uniqueTokens {
			base := i * 2
			symbols[t] = decodeString(p4[base], erc20ABI, "symbol")
			decimals[t] = int(decodeUint8(p4[base+1], erc20ABI, "decimals"))
		}
	}

	// Compute supply and borrow rates per Comet for APY enrichment on supply/borrow rows.
	// (Not strictly required, but useful so positions surface APY.)
	rateCalls := make([]multicall3Call, 0, len(ctxs)*3)
	for _, cc := range ctxs {
		utilCD, _ := cometABI.Pack("getUtilization")
		rateCalls = append(rateCalls, multicall3Call{Target: cc.comet, AllowFailure: true, CallData: utilCD})
	}
	utilResults, err := execMulticall3(ctx, client, rateCalls)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "multicall positions utilization", err)
	}
	supplyAPYs := make([]float64, len(ctxs))
	borrowAPYs := make([]float64, len(ctxs))
	rateCalls2 := make([]multicall3Call, 0, len(ctxs)*2)
	for i, cc := range ctxs {
		util := decodeUint256(utilResults[i], cometABI, "getUtilization")
		sCD, _ := cometABI.Pack("getSupplyRate", util)
		bCD, _ := cometABI.Pack("getBorrowRate", util)
		rateCalls2 = append(rateCalls2,
			multicall3Call{Target: cc.comet, AllowFailure: true, CallData: sCD},
			multicall3Call{Target: cc.comet, AllowFailure: true, CallData: bCD},
		)
	}
	rateResults, err := execMulticall3(ctx, client, rateCalls2)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "multicall positions rates", err)
	}
	for i := range ctxs {
		base := i * 2
		supplyAPYs[i] = ratePerSecondToAPY(decodeUint64(rateResults[base], cometABI, "getSupplyRate"))
		borrowAPYs[i] = ratePerSecondToAPY(decodeUint64(rateResults[base+1], cometABI, "getBorrowRate"))
	}

	// Assemble positions.
	filterType := providers.LendPositionType(strings.ToLower(strings.TrimSpace(string(req.PositionType))))
	out := make([]model.LendPosition, 0)
	now := c.now().UTC().Format(time.RFC3339)

	for i, cc := range ctxs {
		baseSymbol := symbols[cc.baseToken]
		baseDecimals := decimals[cc.baseToken]
		basePriceUSD := basePricesUSD[i]
		nativeID := providerNativeID(req.Chain.CAIP2, strings.ToLower(cc.comet.Hex()), strings.ToLower(cc.baseToken.Hex()))

		// Supply position (base asset).
		if cc.supplyBal.Sign() > 0 && matchesAsset(strings.ToLower(cc.baseToken.Hex()), baseSymbol, req.Asset) && matchesPositionType(filterType, providers.LendPositionTypeSupply) {
			if assetID := canonicalAssetIDForChain(req.Chain.CAIP2, strings.ToLower(cc.baseToken.Hex())); assetID != "" && baseDecimals > 0 {
				out = append(out, model.LendPosition{
					Protocol:             "compoundv3",
					Provider:             "compoundv3",
					ChainID:              req.Chain.CAIP2,
					AccountAddress:       account,
					PositionType:         string(providers.LendPositionTypeSupply),
					AssetID:              assetID,
					ProviderNativeID:     nativeID,
					ProviderNativeIDKind: model.NativeIDKindCompositeMarketAsset,
					Amount:               amountInfoFromBigInt(cc.supplyBal, baseDecimals),
					AmountUSD:            bigIntToFloat(cc.supplyBal, baseDecimals) * basePriceUSD,
					APY:                  supplyAPYs[i],
					SourceURL:            "https://app.compound.finance",
					FetchedAt:            now,
				})
			}
		}

		// Borrow position (base asset).
		if cc.borrowBal.Sign() > 0 && matchesAsset(strings.ToLower(cc.baseToken.Hex()), baseSymbol, req.Asset) && matchesPositionType(filterType, providers.LendPositionTypeBorrow) {
			if assetID := canonicalAssetIDForChain(req.Chain.CAIP2, strings.ToLower(cc.baseToken.Hex())); assetID != "" && baseDecimals > 0 {
				out = append(out, model.LendPosition{
					Protocol:             "compoundv3",
					Provider:             "compoundv3",
					ChainID:              req.Chain.CAIP2,
					AccountAddress:       account,
					PositionType:         string(providers.LendPositionTypeBorrow),
					AssetID:              assetID,
					ProviderNativeID:     nativeID,
					ProviderNativeIDKind: model.NativeIDKindCompositeMarketAsset,
					Amount:               amountInfoFromBigInt(cc.borrowBal, baseDecimals),
					AmountUSD:            bigIntToFloat(cc.borrowBal, baseDecimals) * basePriceUSD,
					APY:                  borrowAPYs[i],
					SourceURL:            "https://app.compound.finance",
					FetchedAt:            now,
				})
			}
		}

		// Collateral positions (other assets in the same Comet).
		if !matchesPositionType(filterType, providers.LendPositionTypeCollateral) {
			continue
		}
		for ai, asset := range cometCollaterals[i] {
			slot := cometSlots[i][ai]
			balance := decodeUserCollateralBalance(p3[slot.userCollIdx])
			if balance.Sign() == 0 {
				continue
			}
			collSymbol := symbols[asset.asset]
			collDecimals := decimals[asset.asset]
			collAddrLower := strings.ToLower(asset.asset.Hex())
			if !matchesAsset(collAddrLower, collSymbol, req.Asset) {
				continue
			}
			assetID := canonicalAssetIDForChain(req.Chain.CAIP2, collAddrLower)
			if assetID == "" || collDecimals == 0 {
				continue
			}
			collateralPriceUSD := decodeUSDPrice(p3[slot.priceIdx], cometABI, "getPrice")

			out = append(out, model.LendPosition{
				Protocol:             "compoundv3",
				Provider:             "compoundv3",
				ChainID:              req.Chain.CAIP2,
				AccountAddress:       account,
				PositionType:         string(providers.LendPositionTypeCollateral),
				AssetID:              assetID,
				ProviderNativeID:     providerNativeID(req.Chain.CAIP2, strings.ToLower(cc.comet.Hex()), collAddrLower),
				ProviderNativeIDKind: model.NativeIDKindCompositeMarketAsset,
				Amount:               amountInfoFromBigInt(balance, collDecimals),
				AmountUSD:            bigIntToFloat(balance, collDecimals) * collateralPriceUSD,
				APY:                  0, // collateral does not earn yield in Compound V3
				SourceURL:            "https://app.compound.finance",
				FetchedAt:            now,
			})
		}
	}

	sortLendPositions(out)
	if req.Limit > 0 && len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

// ── YieldPositionsProvider ──────────────────────────────────────────────

func (c *Client) YieldPositions(ctx context.Context, req providers.YieldPositionsRequest) ([]model.YieldPosition, error) {
	rows, err := c.LendPositions(ctx, providers.LendPositionsRequest{
		Chain:        req.Chain,
		Account:      req.Account,
		Asset:        req.Asset,
		PositionType: providers.LendPositionTypeAll,
		Limit:        req.Limit,
		RPCURL:       req.RPCURL,
	})
	if err != nil {
		return nil, err
	}

	out := make([]model.YieldPosition, 0, len(rows))
	for _, row := range rows {
		// Only base-asset supply earns yield in Compound V3.
		if row.PositionType != string(providers.LendPositionTypeSupply) {
			continue
		}
		opportunityID := ""
		if strings.TrimSpace(row.ProviderNativeID) != "" {
			opportunityID = hashOpportunity("compoundv3", row.ChainID, row.ProviderNativeID, row.AssetID)
		}
		out = append(out, model.YieldPosition{
			Protocol:             "compoundv3",
			Provider:             "compoundv3",
			ChainID:              row.ChainID,
			AccountAddress:       row.AccountAddress,
			PositionType:         "deposit",
			OpportunityID:        opportunityID,
			AssetID:              row.AssetID,
			ProviderNativeID:     row.ProviderNativeID,
			ProviderNativeIDKind: row.ProviderNativeIDKind,
			Amount:               row.Amount,
			AmountUSD:            row.AmountUSD,
			APYTotal:             row.APY,
			SourceURL:            row.SourceURL,
			FetchedAt:            row.FetchedAt,
		})
	}

	sortYieldPositions(out)
	if req.Limit > 0 && len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

// ── Market data fetching ────────────────────────────────────────────────

func (c *Client) fetchMarkets(ctx context.Context, chain id.Chain) ([]cometMarket, error) {
	if !chain.IsEVM() {
		return nil, clierr.New(clierr.CodeUnsupported, "compoundv3 supports only EVM chains")
	}
	rpcURL, err := c.resolveRPC(chain.EVMChainID, "")
	if err != nil {
		return nil, err
	}
	deployments, ok := registry.CompoundV3Markets(chain.EVMChainID)
	if !ok {
		return nil, clierr.New(clierr.CodeUnsupported, "compoundv3 is not supported on this chain")
	}

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "connect rpc", err)
	}
	defer client.Close()

	// Phase 1: per Comet — baseToken, baseTokenPriceFeed, getUtilization, totalSupply, totalBorrow.
	const callsPerCometPhase1 = 5
	phase1Calls := make([]multicall3Call, 0, len(deployments)*callsPerCometPhase1)
	baseTokenCD, _ := cometABI.Pack("baseToken")
	priceFeedCD, _ := cometABI.Pack("baseTokenPriceFeed")
	utilCD, _ := cometABI.Pack("getUtilization")
	totalSupplyCD, _ := cometABI.Pack("totalSupply")
	totalBorrowCD, _ := cometABI.Pack("totalBorrow")

	for _, d := range deployments {
		comet := common.HexToAddress(d.Comet)
		phase1Calls = append(phase1Calls,
			multicall3Call{Target: comet, AllowFailure: true, CallData: baseTokenCD},
			multicall3Call{Target: comet, AllowFailure: true, CallData: priceFeedCD},
			multicall3Call{Target: comet, AllowFailure: true, CallData: utilCD},
			multicall3Call{Target: comet, AllowFailure: true, CallData: totalSupplyCD},
			multicall3Call{Target: comet, AllowFailure: true, CallData: totalBorrowCD},
		)
	}
	p1, err := execMulticall3(ctx, client, phase1Calls)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "multicall comet phase1", err)
	}

	type cometParsed struct {
		comet        common.Address
		label        string
		baseToken    common.Address
		priceFeed    common.Address
		utilization  *big.Int
		totalSupply  *big.Int
		totalBorrow  *big.Int
	}

	parsed := make([]cometParsed, 0, len(deployments))
	for i, d := range deployments {
		base := i * callsPerCometPhase1
		r := p1[base : base+callsPerCometPhase1]

		baseToken, ok := decodeAddress(r[0], cometABI, "baseToken")
		if !ok {
			continue
		}
		priceFeed, _ := decodeAddress(r[1], cometABI, "baseTokenPriceFeed")
		utilization := decodeUint256(r[2], cometABI, "getUtilization")
		totalSupply := decodeUint256(r[3], cometABI, "totalSupply")
		totalBorrow := decodeUint256(r[4], cometABI, "totalBorrow")

		parsed = append(parsed, cometParsed{
			comet:        common.HexToAddress(d.Comet),
			label:        d.Label,
			baseToken:    baseToken,
			priceFeed:    priceFeed,
			utilization:  utilization,
			totalSupply:  totalSupply,
			totalBorrow:  totalBorrow,
		})
	}
	if len(parsed) == 0 {
		return nil, nil
	}

	// Phase 2: per Comet — getSupplyRate(util), getBorrowRate(util), getPrice(priceFeed) + base symbol/decimals.
	const callsPerCometPhase2 = 5
	phase2Calls := make([]multicall3Call, 0, len(parsed)*callsPerCometPhase2)
	symbolCD, _ := erc20ABI.Pack("symbol")
	decimalsCD, _ := erc20ABI.Pack("decimals")
	for _, p := range parsed {
		sCD, _ := cometABI.Pack("getSupplyRate", p.utilization)
		bCD, _ := cometABI.Pack("getBorrowRate", p.utilization)
		pCD, _ := cometABI.Pack("getPrice", p.priceFeed)
		phase2Calls = append(phase2Calls,
			multicall3Call{Target: p.comet, AllowFailure: true, CallData: sCD},
			multicall3Call{Target: p.comet, AllowFailure: true, CallData: bCD},
			multicall3Call{Target: p.comet, AllowFailure: true, CallData: pCD},
			multicall3Call{Target: p.baseToken, AllowFailure: true, CallData: symbolCD},
			multicall3Call{Target: p.baseToken, AllowFailure: true, CallData: decimalsCD},
		)
	}
	p2, err := execMulticall3(ctx, client, phase2Calls)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeUnavailable, "multicall comet phase2", err)
	}

	out := make([]cometMarket, 0, len(parsed))
	for i, p := range parsed {
		base := i * callsPerCometPhase2
		supplyRate := decodeUint64(p2[base], cometABI, "getSupplyRate")
		borrowRate := decodeUint64(p2[base+1], cometABI, "getBorrowRate")
		priceUSD := decodeUSDPrice(p2[base+2], cometABI, "getPrice")
		symbol := decodeString(p2[base+3], erc20ABI, "symbol")
		dec := int(decodeUint8(p2[base+4], erc20ABI, "decimals"))
		if symbol == "" || dec == 0 {
			continue
		}

		tvlUSD := bigIntToFloat(p.totalSupply, dec) * priceUSD
		totalBorrowUSD := bigIntToFloat(p.totalBorrow, dec) * priceUSD
		liquidityUSD := tvlUSD - totalBorrowUSD
		if liquidityUSD < 0 {
			liquidityUSD = 0
		}
		util := mantissaToFloat(p.utilization, 18)

		out = append(out, cometMarket{
			Comet:           strings.ToLower(p.comet.Hex()),
			Label:           p.label,
			BaseAsset:       strings.ToLower(p.baseToken.Hex()),
			BaseSymbol:      symbol,
			BaseDecimals:    dec,
			BasePriceUSD:    priceUSD,
			SupplyAPY:       ratePerSecondToAPY(supplyRate),
			BorrowAPY:       ratePerSecondToAPY(borrowRate),
			Utilization:     util,
			TotalSupplyBase: p.totalSupply,
			TotalBorrowBase: p.totalBorrow,
			TVLUSD:          tvlUSD,
			TotalBorrowsUSD: totalBorrowUSD,
			LiquidityUSD:    liquidityUSD,
		})
	}
	return out, nil
}

func (c *Client) resolveRPC(chainID int64, override string) (string, error) {
	candidate := override
	if candidate == "" {
		candidate = c.rpcOverride
	}
	rpcURL, err := registry.ResolveRPCURL(candidate, chainID)
	if err != nil {
		return "", clierr.Wrap(clierr.CodeUnsupported, "resolve rpc url", err)
	}
	return rpcURL, nil
}

// ── Multicall3 plumbing ─────────────────────────────────────────────────

func execMulticall3(ctx context.Context, client *ethclient.Client, calls []multicall3Call) ([]multicall3Result, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	type call3Tuple struct {
		Target       common.Address `abi:"target"`
		AllowFailure bool           `abi:"allowFailure"`
		CallData     []byte         `abi:"callData"`
	}
	tuples := make([]call3Tuple, len(calls))
	for i, c := range calls {
		tuples[i] = call3Tuple{Target: c.Target, AllowFailure: c.AllowFailure, CallData: c.CallData}
	}

	data, err := mc3ABI.Pack("aggregate3", tuples)
	if err != nil {
		return nil, fmt.Errorf("pack aggregate3: %w", err)
	}

	mc3 := multicall3Addr
	out, err := client.CallContract(ctx, ethereum.CallMsg{To: &mc3, Data: data}, nil)
	if err != nil {
		return nil, fmt.Errorf("call aggregate3: %w", err)
	}

	decoded, err := mc3ABI.Unpack("aggregate3", out)
	if err != nil {
		return nil, fmt.Errorf("decode aggregate3: %w", err)
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("empty aggregate3 response")
	}

	rawResults, ok := decoded[0].([]struct {
		Success    bool   `json:"success"`
		ReturnData []byte `json:"returnData"`
	})
	if !ok {
		return nil, fmt.Errorf("unexpected aggregate3 result type: %T", decoded[0])
	}

	results := make([]multicall3Result, len(rawResults))
	for i, r := range rawResults {
		results[i] = multicall3Result{Success: r.Success, ReturnData: r.ReturnData}
	}
	return results, nil
}

// ── ABI decoders ────────────────────────────────────────────────────────

func decodeAddress(r multicall3Result, a abi.ABI, method string) (common.Address, bool) {
	if !r.Success || len(r.ReturnData) < 32 {
		return common.Address{}, false
	}
	dec, err := a.Unpack(method, r.ReturnData)
	if err != nil || len(dec) == 0 {
		return common.Address{}, false
	}
	addr, ok := dec[0].(common.Address)
	return addr, ok
}

func decodeUint256(r multicall3Result, a abi.ABI, method string) *big.Int {
	if !r.Success || len(r.ReturnData) < 32 {
		return new(big.Int)
	}
	dec, err := a.Unpack(method, r.ReturnData)
	if err != nil || len(dec) == 0 {
		return new(big.Int)
	}
	return asBigInt(dec[0])
}

func decodeUint64(r multicall3Result, a abi.ABI, method string) uint64 {
	if !r.Success || len(r.ReturnData) < 32 {
		return 0
	}
	dec, err := a.Unpack(method, r.ReturnData)
	if err != nil || len(dec) == 0 {
		return 0
	}
	switch v := dec[0].(type) {
	case uint64:
		return v
	case *big.Int:
		if v == nil {
			return 0
		}
		return v.Uint64()
	default:
		return 0
	}
}

func decodeUint8(r multicall3Result, a abi.ABI, method string) uint8 {
	if !r.Success || len(r.ReturnData) < 32 {
		return 0
	}
	dec, err := a.Unpack(method, r.ReturnData)
	if err != nil || len(dec) == 0 {
		return 0
	}
	switch v := dec[0].(type) {
	case uint8:
		return v
	case *big.Int:
		if v == nil {
			return 0
		}
		return uint8(v.Uint64())
	default:
		return 0
	}
}

func decodeString(r multicall3Result, a abi.ABI, method string) string {
	if !r.Success || len(r.ReturnData) < 32 {
		return ""
	}
	dec, err := a.Unpack(method, r.ReturnData)
	if err != nil || len(dec) == 0 {
		return ""
	}
	s, _ := dec[0].(string)
	return s
}

// decodeUserCollateralBalance handles userCollateral(user, asset) which
// returns (uint128 balance, uint128 _reserved). We only care about balance.
func decodeUserCollateralBalance(r multicall3Result) *big.Int {
	if !r.Success || len(r.ReturnData) < 64 {
		return new(big.Int)
	}
	dec, err := cometABI.Unpack("userCollateral", r.ReturnData)
	if err != nil || len(dec) == 0 {
		return new(big.Int)
	}
	return asBigInt(dec[0])
}

type collateralAsset struct {
	asset     common.Address
	priceFeed common.Address
	scale     uint64
}

// decodeAssetInfo extracts the (asset, priceFeed) pair from a Comet
// getAssetInfo(uint8) result. The geth ABI decoder returns named tuple
// outputs as anonymous structs whose fields are tagged with the ABI
// component names; the assertion below must match that exact shape.
func decodeAssetInfo(r multicall3Result) (collateralAsset, bool) {
	if !r.Success || len(r.ReturnData) == 0 {
		return collateralAsset{}, false
	}
	dec, err := cometABI.Unpack("getAssetInfo", r.ReturnData)
	if err != nil || len(dec) == 0 {
		return collateralAsset{}, false
	}
	info, ok := dec[0].(struct {
		Offset                    uint8          `json:"offset"`
		Asset                     common.Address `json:"asset"`
		PriceFeed                 common.Address `json:"priceFeed"`
		Scale                     uint64         `json:"scale"`
		BorrowCollateralFactor    uint64         `json:"borrowCollateralFactor"`
		LiquidateCollateralFactor uint64         `json:"liquidateCollateralFactor"`
		LiquidationFactor         uint64         `json:"liquidationFactor"`
		SupplyCap                 *big.Int       `json:"supplyCap"`
	})
	if !ok {
		return collateralAsset{}, false
	}
	return collateralAsset{asset: info.Asset, priceFeed: info.PriceFeed, scale: info.Scale}, true
}

func decodeUSDPrice(r multicall3Result, a abi.ABI, method string) float64 {
	mantissa := decodeUint256(r, a, method)
	if mantissa.Sign() == 0 {
		return 0
	}
	return mantissaToFloat(mantissa, 8) // Comet getPrice returns 8-decimal USD
}

// ── math helpers ────────────────────────────────────────────────────────

// ratePerSecondToAPY converts Comet's per-second rate (scaled by 1e18) into a
// percentage-points APY. Linear approximation matches how Moonwell adapter
// reports rates (see internal/providers/moonwell/client.go).
func ratePerSecondToAPY(ratePerSecond uint64) float64 {
	if ratePerSecond == 0 {
		return 0
	}
	rate := float64(ratePerSecond)
	apy := rate * secondsPerYear / rateScale * 100
	if math.IsNaN(apy) || math.IsInf(apy, 0) {
		return 0
	}
	return apy
}

func mantissaToFloat(v *big.Int, decimals int) float64 {
	if v == nil || v.Sign() == 0 {
		return 0
	}
	if decimals < 0 {
		decimals = 0
	}
	f := new(big.Float).SetInt(v)
	scale := new(big.Float).SetFloat64(math.Pow(10, float64(decimals)))
	f.Quo(f, scale)
	result, _ := f.Float64()
	return result
}

func bigIntToFloat(v *big.Int, decimals int) float64 {
	if v == nil || v.Sign() == 0 {
		return 0
	}
	f := new(big.Float).SetInt(v)
	divisor := new(big.Float).SetFloat64(math.Pow(10, float64(decimals)))
	f.Quo(f, divisor)
	result, _ := f.Float64()
	return result
}

func asBigInt(v interface{}) *big.Int {
	switch val := v.(type) {
	case *big.Int:
		if val == nil {
			return new(big.Int)
		}
		return val
	case big.Int:
		return &val
	default:
		return new(big.Int)
	}
}

// ── Other helpers ───────────────────────────────────────────────────────

func amountInfoFromBigInt(v *big.Int, decimals int) model.AmountInfo {
	if v == nil {
		v = new(big.Int)
	}
	base := v.String()
	return model.AmountInfo{
		AmountBaseUnits: base,
		AmountDecimal:   id.FormatDecimalCompat(base, decimals),
		Decimals:        decimals,
	}
}

func normalizeEVMAddress(address string) string {
	addr := strings.ToLower(strings.TrimSpace(address))
	if len(addr) != 42 || !strings.HasPrefix(addr, "0x") {
		return ""
	}
	return addr
}

func canonicalAssetIDForChain(chainID, address string) string {
	addr := normalizeEVMAddress(address)
	if chainID == "" || addr == "" {
		return ""
	}
	return fmt.Sprintf("%s/erc20:%s", chainID, addr)
}

func providerNativeID(chainID, comet, asset string) string {
	return fmt.Sprintf("compoundv3:%s:%s:%s", chainID, normalizeEVMAddress(comet), normalizeEVMAddress(asset))
}

func hashOpportunity(provider, chainID, marketID, assetID string) string {
	seed := strings.Join([]string{provider, chainID, marketID, assetID}, "|")
	h := sha1.Sum([]byte(seed))
	return hex.EncodeToString(h[:])
}

func matchesAsset(address, symbol string, asset id.Asset) bool {
	assetAddress := strings.TrimSpace(asset.Address)
	if assetAddress != "" {
		return strings.EqualFold(strings.TrimSpace(address), assetAddress)
	}
	assetSymbol := strings.TrimSpace(asset.Symbol)
	if assetSymbol != "" {
		return strings.EqualFold(strings.TrimSpace(symbol), assetSymbol)
	}
	return true
}

func matchesPositionType(filter, position providers.LendPositionType) bool {
	if filter == "" || filter == providers.LendPositionTypeAll {
		return true
	}
	return filter == position
}

func sortLendPositions(items []model.LendPosition) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].AmountUSD != items[j].AmountUSD {
			return items[i].AmountUSD > items[j].AmountUSD
		}
		if items[i].PositionType != items[j].PositionType {
			return items[i].PositionType < items[j].PositionType
		}
		if items[i].AssetID != items[j].AssetID {
			return items[i].AssetID < items[j].AssetID
		}
		return items[i].ProviderNativeID < items[j].ProviderNativeID
	})
}

func sortYieldPositions(items []model.YieldPosition) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].AmountUSD != items[j].AmountUSD {
			return items[i].AmountUSD > items[j].AmountUSD
		}
		if items[i].APYTotal != items[j].APYTotal {
			return items[i].APYTotal > items[j].APYTotal
		}
		if items[i].AssetID != items[j].AssetID {
			return items[i].AssetID < items[j].AssetID
		}
		return items[i].ProviderNativeID < items[j].ProviderNativeID
	})
}

// ── ABI singletons ──────────────────────────────────────────────────────

var (
	cometABI = mustABI(registry.CompoundV3CometABI)
	erc20ABI = mustABI(registry.MoonwellERC20MinimalABI)
	mc3ABI   = mustABI(registry.Multicall3ABI)
)

func mustABI(raw string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(raw))
	if err != nil {
		panic(fmt.Sprintf("invalid ABI: %v", err))
	}
	return parsed
}
