package planner

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	clierr "github.com/ggonzalez94/defi-cli/internal/errors"
	"github.com/ggonzalez94/defi-cli/internal/execution"
	"github.com/ggonzalez94/defi-cli/internal/id"
	"github.com/ggonzalez94/defi-cli/internal/registry"
)

// CompoundV3LendRequest captures everything needed to plan a Compound V3
// (Comet) supply / withdraw / borrow / repay action. The verb type is shared
// with Aave (planner.AaveLendVerb) so the actionbuilder router can pass the
// same value through without translation.
//
// Comet's on-chain surface only exposes two write methods — supply() and
// withdraw() — and overloads them to express both base-asset balance changes
// and collateral movements:
//
//   - supply(base, x):    deposit base; if user has debt, repays first
//   - supply(coll, x):    deposit collateral
//   - withdraw(base, x):  withdraw supply; if amount > balance, opens a borrow
//   - withdraw(coll, x):  withdraw collateral (must keep position healthy)
//
// We map the four CLI verbs onto those two methods, restricting borrow/repay
// to the base asset since Comet does not expose any other borrowable asset.
type CompoundV3LendRequest struct {
	Verb            AaveLendVerb // supply / withdraw / borrow / repay
	Chain           id.Chain
	Asset           id.Asset
	AmountBaseUnits string
	Sender          string
	Recipient       string
	Simulate        bool
	RPCURL          string

	// CometAddress disambiguates which Comet to target. When empty we
	// auto-resolve by base-asset match against the registry; this works for
	// supply/withdraw of a base asset and is required for any operation
	// against a collateral asset (since the same collateral can appear in
	// multiple Comets).
	CometAddress string
}

// BuildCompoundV3LendAction builds the action plan (approval + Comet call)
// for a Compound V3 lend verb against a single Comet market.
func BuildCompoundV3LendAction(ctx context.Context, req CompoundV3LendRequest) (execution.Action, error) {
	verb := strings.ToLower(strings.TrimSpace(string(req.Verb)))
	sender := strings.TrimSpace(req.Sender)
	if !common.IsHexAddress(sender) {
		return execution.Action{}, clierr.New(clierr.CodeUsage, "lend action requires sender address")
	}
	recipient := strings.TrimSpace(req.Recipient)
	if recipient == "" {
		recipient = sender
	}
	if !common.IsHexAddress(recipient) {
		return execution.Action{}, clierr.New(clierr.CodeUsage, "invalid recipient address")
	}
	if !common.IsHexAddress(req.Asset.Address) {
		return execution.Action{}, clierr.New(clierr.CodeUsage, "compound v3 lend asset must resolve to an ERC20 address")
	}
	amount, ok := new(big.Int).SetString(strings.TrimSpace(req.AmountBaseUnits), 10)
	if !ok || amount.Sign() <= 0 {
		return execution.Action{}, clierr.New(clierr.CodeUsage, "lend amount must be a positive integer in base units")
	}
	rpcURL, err := registry.ResolveRPCURL(req.RPCURL, req.Chain.EVMChainID)
	if err != nil {
		return execution.Action{}, clierr.Wrap(clierr.CodeUsage, "resolve rpc url", err)
	}

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return execution.Action{}, clierr.Wrap(clierr.CodeUnavailable, "connect rpc", err)
	}
	defer client.Close()

	senderAddr := common.HexToAddress(sender)
	recipientAddr := common.HexToAddress(recipient)
	tokenAddr := common.HexToAddress(req.Asset.Address)

	// Resolve the Comet contract for this (chain, asset) pair.
	cometAddr, baseAssetAddr, err := resolveCompoundV3Comet(ctx, client, req.Chain, req.CometAddress, tokenAddr)
	if err != nil {
		return execution.Action{}, err
	}

	// Verbs that move base-asset balances (borrow/repay) must target the
	// Comet's base asset; collateral assets cannot be borrowed or repaid.
	isBaseAsset := strings.EqualFold(tokenAddr.Hex(), baseAssetAddr.Hex())
	switch verb {
	case string(AaveVerbBorrow), string(AaveVerbRepay):
		if !isBaseAsset {
			return execution.Action{}, clierr.New(clierr.CodeUnsupported,
				fmt.Sprintf("compound v3 borrow/repay only supports the comet base asset (%s); pass --asset matching the base or use supply/withdraw for collateral", baseAssetAddr.Hex()))
		}
	}

	action := execution.NewAction(execution.NewActionID(), "lend_"+verb, req.Chain.CAIP2, execution.Constraints{Simulate: req.Simulate})
	action.Provider = "compoundv3"
	action.FromAddress = senderAddr.Hex()
	action.ToAddress = recipientAddr.Hex()
	action.InputAmount = amount.String()
	action.Metadata = map[string]any{
		"protocol":       "compoundv3",
		"asset_id":       req.Asset.AssetID,
		"comet":          cometAddr.Hex(),
		"base_asset":     baseAssetAddr.Hex(),
		"asset_role":     assetRole(isBaseAsset),
		"lending_action": verb,
	}

	switch verb {
	case string(AaveVerbSupply), string(AaveVerbRepay):
		// Both supply and repay map to Comet.supply/supplyTo on the asset.
		// repay uses the base asset (validated above) and Comet auto-applies
		// it against any outstanding debt before increasing supply.
		approvalDesc := "Approve token for Compound V3 supply"
		stepDesc := "Supply asset to Compound V3"
		stepID := "compoundv3-supply"
		if verb == string(AaveVerbRepay) {
			approvalDesc = "Approve base asset for Compound V3 repay"
			stepDesc = "Repay borrowed base asset on Compound V3"
			stepID = "compoundv3-repay"
		}
		if err := appendApprovalIfNeeded(ctx, client, &action, req.Chain.CAIP2, rpcURL, tokenAddr, senderAddr, cometAddr, amount, approvalDesc); err != nil {
			return execution.Action{}, err
		}
		data, err := buildCompoundV3SupplyCalldata(senderAddr, recipientAddr, tokenAddr, amount)
		if err != nil {
			return execution.Action{}, err
		}
		action.Steps = append(action.Steps, execution.ActionStep{
			StepID:      stepID,
			Type:        execution.StepTypeLend,
			Status:      execution.StepStatusPending,
			ChainID:     req.Chain.CAIP2,
			RPCURL:      rpcURL,
			Description: stepDesc,
			Target:      cometAddr.Hex(),
			Data:        "0x" + common.Bytes2Hex(data),
			Value:       "0",
		})

	case string(AaveVerbWithdraw), string(AaveVerbBorrow):
		// Both withdraw and borrow map to Comet.withdraw/withdrawTo. When
		// the base-asset withdraw amount exceeds the user's supply balance,
		// Comet automatically opens a borrow for the difference; that is
		// what we lean on for the borrow verb.
		stepDesc := "Withdraw asset from Compound V3"
		stepID := "compoundv3-withdraw"
		if verb == string(AaveVerbBorrow) {
			stepDesc = "Borrow base asset from Compound V3"
			stepID = "compoundv3-borrow"
		}
		data, err := buildCompoundV3WithdrawCalldata(senderAddr, recipientAddr, tokenAddr, amount)
		if err != nil {
			return execution.Action{}, err
		}
		action.Steps = append(action.Steps, execution.ActionStep{
			StepID:      stepID,
			Type:        execution.StepTypeLend,
			Status:      execution.StepStatusPending,
			ChainID:     req.Chain.CAIP2,
			RPCURL:      rpcURL,
			Description: stepDesc,
			Target:      cometAddr.Hex(),
			Data:        "0x" + common.Bytes2Hex(data),
			Value:       "0",
		})

	default:
		return execution.Action{}, clierr.New(clierr.CodeUsage, "unsupported compound v3 lend verb: "+verb)
	}

	return action, nil
}

// buildCompoundV3SupplyCalldata picks the right Comet entry point depending
// on whether the funds should land in the sender or in a different recipient.
func buildCompoundV3SupplyCalldata(sender, recipient, asset common.Address, amount *big.Int) ([]byte, error) {
	if strings.EqualFold(sender.Hex(), recipient.Hex()) {
		data, err := compoundV3CometABI.Pack("supply", asset, amount)
		if err != nil {
			return nil, clierr.Wrap(clierr.CodeInternal, "pack compound v3 supply calldata", err)
		}
		return data, nil
	}
	data, err := compoundV3CometABI.Pack("supplyTo", recipient, asset, amount)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeInternal, "pack compound v3 supplyTo calldata", err)
	}
	return data, nil
}

// buildCompoundV3WithdrawCalldata mirrors the supply path: withdrawTo is used
// whenever the recipient differs from the sender (Comet pulls from msg.sender's
// account in either case).
func buildCompoundV3WithdrawCalldata(sender, recipient, asset common.Address, amount *big.Int) ([]byte, error) {
	if strings.EqualFold(sender.Hex(), recipient.Hex()) {
		data, err := compoundV3CometABI.Pack("withdraw", asset, amount)
		if err != nil {
			return nil, clierr.Wrap(clierr.CodeInternal, "pack compound v3 withdraw calldata", err)
		}
		return data, nil
	}
	data, err := compoundV3CometABI.Pack("withdrawTo", recipient, asset, amount)
	if err != nil {
		return nil, clierr.Wrap(clierr.CodeInternal, "pack compound v3 withdrawTo calldata", err)
	}
	return data, nil
}

// resolveCompoundV3Comet returns the Comet contract and its base asset.
//
// Resolution strategy:
//   - If --pool-address (cometOverride) is provided, trust it and read its
//     baseToken() so we know which asset is base for verb validation.
//   - Otherwise scan the chain's registered Comets and match by base asset.
//     If the user-supplied asset matches some Comet's base, use it. If not,
//     it must be a collateral asset shared by multiple Comets and we ask the
//     caller to disambiguate via --pool-address.
func resolveCompoundV3Comet(ctx context.Context, client *ethclient.Client, chain id.Chain, cometOverride string, asset common.Address) (common.Address, common.Address, error) {
	if strings.TrimSpace(cometOverride) != "" {
		if !common.IsHexAddress(cometOverride) {
			return common.Address{}, common.Address{}, clierr.New(clierr.CodeUsage, "invalid --pool-address (Comet address)")
		}
		comet := common.HexToAddress(cometOverride)
		base, err := readCompoundV3BaseToken(ctx, client, comet)
		if err != nil {
			return common.Address{}, common.Address{}, err
		}
		return comet, base, nil
	}

	deployments, ok := registry.CompoundV3Markets(chain.EVMChainID)
	if !ok {
		return common.Address{}, common.Address{}, clierr.New(clierr.CodeUnsupported,
			"compound v3 is not supported on this chain; pass --pool-address with the Comet address")
	}

	// Read baseToken() for every Comet and pick the one matching the asset.
	for _, d := range deployments {
		comet := common.HexToAddress(d.Comet)
		base, err := readCompoundV3BaseToken(ctx, client, comet)
		if err != nil {
			continue // skip unreachable Comets
		}
		if strings.EqualFold(base.Hex(), asset.Hex()) {
			return comet, base, nil
		}
	}

	return common.Address{}, common.Address{}, clierr.New(clierr.CodeUnsupported,
		fmt.Sprintf("asset %s is not the base of any registered Compound V3 comet on chain %d; pass --pool-address with the target Comet address",
			asset.Hex(), chain.EVMChainID))
}

func readCompoundV3BaseToken(ctx context.Context, client *ethclient.Client, comet common.Address) (common.Address, error) {
	data, err := compoundV3CometABI.Pack("baseToken")
	if err != nil {
		return common.Address{}, clierr.Wrap(clierr.CodeInternal, "pack compound v3 baseToken calldata", err)
	}
	out, err := client.CallContract(ctx, ethereum.CallMsg{To: &comet, Data: data}, nil)
	if err != nil {
		return common.Address{}, clierr.Wrap(clierr.CodeUnavailable, "call compound v3 baseToken", err)
	}
	dec, err := compoundV3CometABI.Unpack("baseToken", out)
	if err != nil || len(dec) == 0 {
		return common.Address{}, clierr.Wrap(clierr.CodeUnavailable, "decode compound v3 baseToken", err)
	}
	addr, ok := dec[0].(common.Address)
	if !ok {
		return common.Address{}, clierr.New(clierr.CodeUnavailable, "invalid baseToken response")
	}
	return addr, nil
}

func assetRole(isBase bool) string {
	if isBase {
		return "base"
	}
	return "collateral"
}

var compoundV3CometABI = mustPlannerABI(registry.CompoundV3CometABI)
