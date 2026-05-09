package planner

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ggonzalez94/defi-cli/internal/id"
)

// Real Ethereum mainnet cUSDCv3 deployment from registry.CompoundV3Markets.
// We use real addresses so tests exercise the auto-resolution path against
// the production registry.
const (
	mainnetUSDC      = "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"
	mainnetWETH      = "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"
	mainnetCUSDCv3   = "0xc3d688B66703497DAA19211EEdff47f25384cdc3"
	mainnetCWETHv3   = "0xA17581A9E3356d9A858b789D68B4d866e593aE94"
	mainnetCUSDTv3   = "0x3Afdc9BCA9213A35503b077a6072F3D0d5AB0840"
	mainnetUSDT      = "0xdAC17F958D2ee523a2206206994597C13D831ec7"
	cv3TestSender    = "0x00000000000000000000000000000000000000AA"
	cv3TestRecipient = "0x00000000000000000000000000000000000000BB"
)

// newCompoundV3PlannerRPC mocks the RPC surface needed by the planner:
//   - baseToken() on each known mainnet Comet → returns the matching base
//   - allowance(owner, spender) on any token → returns the configured value
func newCompoundV3PlannerRPC(t *testing.T, allowance *big.Int) *httptest.Server {
	t.Helper()

	baseTokenSel := hex.EncodeToString(compoundV3CometABI.Methods["baseToken"].ID)
	allowanceSel := hex.EncodeToString(plannerERC20ABI.Methods["allowance"].ID)

	baseByComet := map[string]string{
		strings.ToLower(mainnetCUSDCv3): mainnetUSDC,
		strings.ToLower(mainnetCWETHv3): mainnetWETH,
		strings.ToLower(mainnetCUSDTv3): mainnetUSDT,
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req plannerRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Method != "eth_call" {
			writePlannerRPCError(w, req.ID, -32601, fmt.Sprintf("method not supported: %s", req.Method))
			return
		}
		var callObj struct {
			To    string `json:"to"`
			Data  string `json:"data"`
			Input string `json:"input"`
		}
		if err := json.Unmarshal(req.Params[0], &callObj); err != nil {
			writePlannerRPCError(w, req.ID, -32602, "bad params")
			return
		}
		dataHex := callObj.Data
		if dataHex == "" {
			dataHex = callObj.Input
		}
		raw, _ := hex.DecodeString(strings.TrimPrefix(dataHex, "0x"))
		if len(raw) < 4 {
			writePlannerRPCError(w, req.ID, -32602, "data too short")
			return
		}
		selector := hex.EncodeToString(raw[:4])

		switch selector {
		case baseTokenSel:
			to := strings.ToLower(callObj.To)
			base, ok := baseByComet[to]
			if !ok {
				// Unknown comet — return an unrelated zero address; planner
				// will skip it during resolution.
				encoded, _ := compoundV3CometABI.Methods["baseToken"].Outputs.Pack(common.Address{})
				writePlannerRPCResult(w, req.ID, "0x"+hex.EncodeToString(encoded))
				return
			}
			encoded, _ := compoundV3CometABI.Methods["baseToken"].Outputs.Pack(common.HexToAddress(base))
			writePlannerRPCResult(w, req.ID, "0x"+hex.EncodeToString(encoded))
		case allowanceSel:
			encoded, _ := plannerERC20ABI.Methods["allowance"].Outputs.Pack(allowance)
			writePlannerRPCResult(w, req.ID, "0x"+hex.EncodeToString(encoded))
		default:
			writePlannerRPCError(w, req.ID, -32601, "selector not stubbed: "+selector)
		}
	}))
}

func ethereumChain(t *testing.T) id.Chain {
	t.Helper()
	chain, err := id.ParseChain("ethereum")
	if err != nil {
		t.Fatalf("parse chain: %v", err)
	}
	return chain
}

// ── Supply ──────────────────────────────────────────────────────────────

func TestBuildCompoundV3SupplyAutoResolvesComet(t *testing.T) {
	rpc := newCompoundV3PlannerRPC(t, big.NewInt(0)) // zero allowance → approval needed
	defer rpc.Close()

	asset := id.Asset{Address: mainnetUSDC, AssetID: "eip155:1/erc20:" + strings.ToLower(mainnetUSDC)}
	action, err := BuildCompoundV3LendAction(context.Background(), CompoundV3LendRequest{
		Verb:            AaveVerbSupply,
		Chain:           ethereumChain(t),
		Asset:           asset,
		AmountBaseUnits: "1000000",
		Sender:          cv3TestSender,
		Recipient:       cv3TestSender,
		Simulate:        true,
		RPCURL:          rpc.URL,
	})
	if err != nil {
		t.Fatalf("BuildCompoundV3LendAction supply failed: %v", err)
	}
	if action.IntentType != "lend_supply" {
		t.Fatalf("unexpected intent type: %s", action.IntentType)
	}
	if action.Provider != "compoundv3" {
		t.Fatalf("unexpected provider: %s", action.Provider)
	}
	if len(action.Steps) != 2 {
		t.Fatalf("expected 2 steps (approval + supply), got %d: %+v", len(action.Steps), action.Steps)
	}
	if action.Steps[0].Type != "approval" {
		t.Fatalf("expected first step approval, got %s", action.Steps[0].Type)
	}
	if !strings.EqualFold(action.Steps[0].Target, mainnetUSDC) {
		t.Fatalf("expected approval target USDC, got %s", action.Steps[0].Target)
	}
	if action.Steps[1].Type != "lend_call" {
		t.Fatalf("expected lend_call, got %s", action.Steps[1].Type)
	}
	if !strings.EqualFold(action.Steps[1].Target, mainnetCUSDCv3) {
		t.Fatalf("expected lend target cUSDCv3, got %s", action.Steps[1].Target)
	}
	// Supply selector for supply(address,uint256).
	supplySelector := hex.EncodeToString(compoundV3CometABI.Methods["supply"].ID)
	if !strings.HasPrefix(strings.TrimPrefix(action.Steps[1].Data, "0x"), supplySelector) {
		t.Fatalf("expected supply() selector, got %s", action.Steps[1].Data)
	}
	if got, want := action.Metadata["asset_role"], "base"; got != want {
		t.Fatalf("asset_role: got %v, want %v", got, want)
	}
	if got, want := action.Metadata["comet"], common.HexToAddress(mainnetCUSDCv3).Hex(); got != want {
		t.Fatalf("comet metadata: got %v, want %v", got, want)
	}
}

func TestBuildCompoundV3SupplySkipsApprovalWhenAllowanceSufficient(t *testing.T) {
	rpc := newCompoundV3PlannerRPC(t, new(big.Int).SetUint64(1<<62))
	defer rpc.Close()

	asset := id.Asset{Address: mainnetUSDC}
	action, err := BuildCompoundV3LendAction(context.Background(), CompoundV3LendRequest{
		Verb:            AaveVerbSupply,
		Chain:           ethereumChain(t),
		Asset:           asset,
		AmountBaseUnits: "1000000",
		Sender:          cv3TestSender,
		RPCURL:          rpc.URL,
	})
	if err != nil {
		t.Fatalf("supply failed: %v", err)
	}
	if len(action.Steps) != 1 {
		t.Fatalf("expected 1 step (supply only), got %d", len(action.Steps))
	}
	if action.Steps[0].StepID != "compoundv3-supply" {
		t.Fatalf("expected supply step, got %s", action.Steps[0].StepID)
	}
}

func TestBuildCompoundV3SupplyTo(t *testing.T) {
	rpc := newCompoundV3PlannerRPC(t, new(big.Int).SetUint64(1<<62))
	defer rpc.Close()

	asset := id.Asset{Address: mainnetUSDC}
	action, err := BuildCompoundV3LendAction(context.Background(), CompoundV3LendRequest{
		Verb:            AaveVerbSupply,
		Chain:           ethereumChain(t),
		Asset:           asset,
		AmountBaseUnits: "1000000",
		Sender:          cv3TestSender,
		Recipient:       cv3TestRecipient,
		RPCURL:          rpc.URL,
	})
	if err != nil {
		t.Fatalf("supply failed: %v", err)
	}
	supplyToSel := hex.EncodeToString(compoundV3CometABI.Methods["supplyTo"].ID)
	if !strings.HasPrefix(strings.TrimPrefix(action.Steps[0].Data, "0x"), supplyToSel) {
		t.Fatalf("expected supplyTo() selector, got %s", action.Steps[0].Data)
	}
}

// ── Withdraw ────────────────────────────────────────────────────────────

func TestBuildCompoundV3WithdrawNoApproval(t *testing.T) {
	rpc := newCompoundV3PlannerRPC(t, big.NewInt(0))
	defer rpc.Close()

	asset := id.Asset{Address: mainnetWETH}
	action, err := BuildCompoundV3LendAction(context.Background(), CompoundV3LendRequest{
		Verb:            AaveVerbWithdraw,
		Chain:           ethereumChain(t),
		Asset:           asset,
		AmountBaseUnits: "500000000000000000", // 0.5 WETH
		Sender:          cv3TestSender,
		RPCURL:          rpc.URL,
	})
	if err != nil {
		t.Fatalf("withdraw failed: %v", err)
	}
	if len(action.Steps) != 1 {
		t.Fatalf("expected 1 step (withdraw only, no approval), got %d", len(action.Steps))
	}
	if action.Steps[0].StepID != "compoundv3-withdraw" {
		t.Fatalf("expected withdraw step, got %s", action.Steps[0].StepID)
	}
	withdrawSel := hex.EncodeToString(compoundV3CometABI.Methods["withdraw"].ID)
	if !strings.HasPrefix(strings.TrimPrefix(action.Steps[0].Data, "0x"), withdrawSel) {
		t.Fatalf("expected withdraw() selector, got %s", action.Steps[0].Data)
	}
	if !strings.EqualFold(action.Steps[0].Target, mainnetCWETHv3) {
		t.Fatalf("expected target cWETHv3, got %s", action.Steps[0].Target)
	}
}

func TestBuildCompoundV3WithdrawTo(t *testing.T) {
	rpc := newCompoundV3PlannerRPC(t, big.NewInt(0))
	defer rpc.Close()

	action, err := BuildCompoundV3LendAction(context.Background(), CompoundV3LendRequest{
		Verb:            AaveVerbWithdraw,
		Chain:           ethereumChain(t),
		Asset:           id.Asset{Address: mainnetUSDC},
		AmountBaseUnits: "100",
		Sender:          cv3TestSender,
		Recipient:       cv3TestRecipient,
		RPCURL:          rpc.URL,
	})
	if err != nil {
		t.Fatalf("withdraw failed: %v", err)
	}
	withdrawToSel := hex.EncodeToString(compoundV3CometABI.Methods["withdrawTo"].ID)
	if !strings.HasPrefix(strings.TrimPrefix(action.Steps[0].Data, "0x"), withdrawToSel) {
		t.Fatalf("expected withdrawTo() selector, got %s", action.Steps[0].Data)
	}
}

// ── Borrow ──────────────────────────────────────────────────────────────

func TestBuildCompoundV3BorrowBaseAssetUsesWithdraw(t *testing.T) {
	rpc := newCompoundV3PlannerRPC(t, big.NewInt(0))
	defer rpc.Close()

	action, err := BuildCompoundV3LendAction(context.Background(), CompoundV3LendRequest{
		Verb:            AaveVerbBorrow,
		Chain:           ethereumChain(t),
		Asset:           id.Asset{Address: mainnetUSDC},
		AmountBaseUnits: "1000000",
		Sender:          cv3TestSender,
		RPCURL:          rpc.URL,
	})
	if err != nil {
		t.Fatalf("borrow failed: %v", err)
	}
	if action.IntentType != "lend_borrow" {
		t.Fatalf("unexpected intent: %s", action.IntentType)
	}
	if len(action.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(action.Steps))
	}
	if action.Steps[0].StepID != "compoundv3-borrow" {
		t.Fatalf("expected compoundv3-borrow step, got %s", action.Steps[0].StepID)
	}
	withdrawSel := hex.EncodeToString(compoundV3CometABI.Methods["withdraw"].ID)
	if !strings.HasPrefix(strings.TrimPrefix(action.Steps[0].Data, "0x"), withdrawSel) {
		t.Fatalf("borrow should call Comet.withdraw(), got %s", action.Steps[0].Data)
	}
}

func TestBuildCompoundV3BorrowCollateralAssetRejected(t *testing.T) {
	rpc := newCompoundV3PlannerRPC(t, big.NewInt(0))
	defer rpc.Close()

	// WETH is the base asset of cWETHv3, so to test the rejection path we use
	// an explicit Comet (cUSDCv3) and ask to borrow WETH from it. WETH is a
	// collateral on cUSDCv3, not its base, so the planner must reject.
	_, err := BuildCompoundV3LendAction(context.Background(), CompoundV3LendRequest{
		Verb:            AaveVerbBorrow,
		Chain:           ethereumChain(t),
		Asset:           id.Asset{Address: mainnetWETH},
		AmountBaseUnits: "1",
		Sender:          cv3TestSender,
		RPCURL:          rpc.URL,
		CometAddress:    mainnetCUSDCv3,
	})
	if err == nil {
		t.Fatalf("expected error when borrowing non-base asset")
	}
	if !strings.Contains(err.Error(), "base asset") {
		t.Fatalf("expected error mentioning base asset, got %v", err)
	}
}

// ── Repay ───────────────────────────────────────────────────────────────

func TestBuildCompoundV3RepayUsesSupplyWithApproval(t *testing.T) {
	rpc := newCompoundV3PlannerRPC(t, big.NewInt(0))
	defer rpc.Close()

	action, err := BuildCompoundV3LendAction(context.Background(), CompoundV3LendRequest{
		Verb:            AaveVerbRepay,
		Chain:           ethereumChain(t),
		Asset:           id.Asset{Address: mainnetUSDC},
		AmountBaseUnits: "1000000",
		Sender:          cv3TestSender,
		RPCURL:          rpc.URL,
	})
	if err != nil {
		t.Fatalf("repay failed: %v", err)
	}
	if action.IntentType != "lend_repay" {
		t.Fatalf("unexpected intent: %s", action.IntentType)
	}
	if len(action.Steps) != 2 {
		t.Fatalf("expected 2 steps (approval + repay), got %d", len(action.Steps))
	}
	if action.Steps[0].Type != "approval" {
		t.Fatalf("expected approval step, got %s", action.Steps[0].Type)
	}
	if action.Steps[1].StepID != "compoundv3-repay" {
		t.Fatalf("expected compoundv3-repay step, got %s", action.Steps[1].StepID)
	}
	supplySel := hex.EncodeToString(compoundV3CometABI.Methods["supply"].ID)
	if !strings.HasPrefix(strings.TrimPrefix(action.Steps[1].Data, "0x"), supplySel) {
		t.Fatalf("repay should call Comet.supply(), got %s", action.Steps[1].Data)
	}
}

// ── Validation ──────────────────────────────────────────────────────────

func TestBuildCompoundV3RequiresPositiveAmount(t *testing.T) {
	rpc := newCompoundV3PlannerRPC(t, big.NewInt(0))
	defer rpc.Close()

	_, err := BuildCompoundV3LendAction(context.Background(), CompoundV3LendRequest{
		Verb:            AaveVerbSupply,
		Chain:           ethereumChain(t),
		Asset:           id.Asset{Address: mainnetUSDC},
		AmountBaseUnits: "0",
		Sender:          cv3TestSender,
		RPCURL:          rpc.URL,
	})
	if err == nil {
		t.Fatalf("expected error for zero amount")
	}
}

func TestBuildCompoundV3RequiresValidSender(t *testing.T) {
	_, err := BuildCompoundV3LendAction(context.Background(), CompoundV3LendRequest{
		Verb:            AaveVerbSupply,
		Chain:           ethereumChain(t),
		Asset:           id.Asset{Address: mainnetUSDC},
		AmountBaseUnits: "1",
		Sender:          "not-an-address",
	})
	if err == nil {
		t.Fatalf("expected error for invalid sender")
	}
}

func TestBuildCompoundV3UnsupportedAssetRequiresPoolAddress(t *testing.T) {
	rpc := newCompoundV3PlannerRPC(t, big.NewInt(0))
	defer rpc.Close()

	// DAI is not the base asset of any Ethereum Comet in our registry; without
	// --pool-address the planner cannot route this to a specific Comet.
	dai := "0x6B175474E89094C44Da98b954EedeAC495271d0F"
	_, err := BuildCompoundV3LendAction(context.Background(), CompoundV3LendRequest{
		Verb:            AaveVerbSupply,
		Chain:           ethereumChain(t),
		Asset:           id.Asset{Address: dai},
		AmountBaseUnits: "1",
		Sender:          cv3TestSender,
		RPCURL:          rpc.URL,
	})
	if err == nil {
		t.Fatalf("expected error when asset is not any Comet base and no --pool-address provided")
	}
	if !strings.Contains(err.Error(), "pool-address") {
		t.Fatalf("expected error mentioning --pool-address, got %v", err)
	}
}

func TestBuildCompoundV3UnsupportedChain(t *testing.T) {
	// Chain 12345 is not in CompoundV3Markets.
	_, err := BuildCompoundV3LendAction(context.Background(), CompoundV3LendRequest{
		Verb:            AaveVerbSupply,
		Chain:           id.Chain{CAIP2: "eip155:12345", EVMChainID: 12345},
		Asset:           id.Asset{Address: mainnetUSDC},
		AmountBaseUnits: "1",
		Sender:          cv3TestSender,
	})
	if err == nil {
		t.Fatalf("expected error for unsupported chain")
	}
}
