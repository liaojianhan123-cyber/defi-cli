package registry

// Canonical Uniswap V3-compatible contracts used by swap execution/quoting.
// Today this map includes Taiko deployments and can be extended chain-by-chain.
var uniswapV3ContractsByChainID = map[int64]struct {
	QuoterV2 string
	Router   string
}{
	167000: {
		QuoterV2: "0xcBa70D57be34aA26557B8E80135a9B7754680aDb",
		Router:   "0x1A0c3a0Cfd1791FAC7798FA2b05208B66aaadfeD",
	},
	167013: {
		QuoterV2: "0xAC8D93657DCc5C0dE9d9AF2772aF9eA3A032a1C6",
		Router:   "0x482233e4DBD56853530fA1918157CE59B60dF230",
	},
}

func UniswapV3Contracts(chainID int64) (quoterV2 string, router string, ok bool) {
	contracts, ok := uniswapV3ContractsByChainID[chainID]
	if !ok {
		return "", "", false
	}
	return contracts.QuoterV2, contracts.Router, true
}

// Canonical Aave V3 PoolAddressesProvider contracts used by planners.
var aavePoolAddressProviderByChainID = map[int64]string{
	1:     "0x2f39d218133AFaB8F2B819B1066c7E434Ad94E9e", // Ethereum
	10:    "0xa97684ead0e402dC232d5A977953DF7ECBaB3CDb", // Optimism
	137:   "0xa97684ead0e402dC232d5A977953DF7ECBaB3CDb", // Polygon
	8453:  "0xe20fCBdBfFC4Dd138cE8b2E6FBb6CB49777ad64D", // Base
	42161: "0xa97684ead0e402dC232d5A977953DF7ECBaB3CDb", // Arbitrum
	43114: "0xa97684ead0e402dC232d5A977953DF7ECBaB3CDb", // Avalanche
}

func AavePoolAddressProvider(chainID int64) (string, bool) {
	value, ok := aavePoolAddressProviderByChainID[chainID]
	return value, ok
}

// Canonical Moonwell Comptroller (Unitroller) contracts per chain.
var moonwellComptrollerByChainID = map[int64]string{
	8453: "0xfBb21d0380beE3312B33c4353c8936a0F13EF26C", // Base
	10:   "0xCa889f40aae37FFf165BccF69aeF1E82b5C511B9", // Optimism
}

func MoonwellComptroller(chainID int64) (string, bool) {
	value, ok := moonwellComptrollerByChainID[chainID]
	return value, ok
}

// CompoundV3Market identifies a single Comet (Compound V3) market on a chain.
// Each Comet manages exactly one base asset (the asset that can be supplied
// for yield and borrowed); other assets serve only as collateral.
type CompoundV3Market struct {
	Comet string // Comet contract address
	Label string // Human-readable label (e.g. "cUSDCv3")
}

// Canonical Compound V3 (Comet) deployments by chain ID.
// Source: https://docs.compound.finance/#networks
var compoundV3MarketsByChainID = map[int64][]CompoundV3Market{
	1: { // Ethereum
		{Comet: "0xc3d688B66703497DAA19211EEdff47f25384cdc3", Label: "cUSDCv3"},
		{Comet: "0xA17581A9E3356d9A858b789D68B4d866e593aE94", Label: "cWETHv3"},
		{Comet: "0x3Afdc9BCA9213A35503b077a6072F3D0d5AB0840", Label: "cUSDTv3"},
	},
	10: { // Optimism
		{Comet: "0x2e44e174f7D53F0212823acC11C01A11d58c5bCB", Label: "cUSDCv3"},
		{Comet: "0xE36A30D249f7761327fd973001A32010b521b6Fd", Label: "cWETHv3"},
		{Comet: "0x995E394b8B2437aC8Ce61Ee0bC610D617962B214", Label: "cUSDTv3"},
	},
	137: { // Polygon
		{Comet: "0xF25212E676D1F7F89Cd72fFEe66158f541246445", Label: "cUSDCv3"},
		{Comet: "0xaeB318360f27748Acb200CE616E389A6C9409a07", Label: "cUSDTv3"},
	},
	8453: { // Base
		{Comet: "0xb125E6687d4313864e53df431d5425969c15Eb2F", Label: "cUSDCv3"},
		{Comet: "0x46e6b214b524310239732D51387075E0e70970bf", Label: "cWETHv3"},
	},
	42161: { // Arbitrum
		{Comet: "0x9c4ec768c28520B50860ea7a15bd7213a9fF58bf", Label: "cUSDCv3"},
		{Comet: "0x6f7D514bbD4aFf3BcD1140B7344b32f063dEe486", Label: "cWETHv3"},
		{Comet: "0xd98Be00b5D27fc98112BdE293e487f8D4cA57d07", Label: "cUSDTv3"},
	},
	534352: { // Scroll
		{Comet: "0xB2f97c1Bd3bf02f5e74d13f02E3e26F93D77CE44", Label: "cUSDCv3"},
	},
}

// CompoundV3Markets returns all known Comet deployments on a chain.
func CompoundV3Markets(chainID int64) ([]CompoundV3Market, bool) {
	markets, ok := compoundV3MarketsByChainID[chainID]
	if !ok || len(markets) == 0 {
		return nil, false
	}
	out := make([]CompoundV3Market, len(markets))
	copy(out, markets)
	return out, true
}

// CompoundV3SupportedChainIDs returns chain IDs that have at least one Comet.
func CompoundV3SupportedChainIDs() []int64 {
	out := make([]int64, 0, len(compoundV3MarketsByChainID))
	for id := range compoundV3MarketsByChainID {
		out = append(out, id)
	}
	return out
}

const tempoStablecoinDEXAddress = "0xdec0000000000000000000000000000000000000"

var tempoChainIDs = map[int64]struct{}{
	31318: {},
	4217:  {},
	42431: {},
}

func TempoStablecoinDEX(chainID int64) (string, bool) {
	if _, ok := tempoChainIDs[chainID]; !ok {
		return "", false
	}
	return tempoStablecoinDEXAddress, true
}

// Canonical fee token addresses for Tempo chains.
var tempoFeeTokenByChainID = map[int64]string{
	4217:  "0x20c000000000000000000000b9537d11c60e8b50",
	42431: "0x20c0000000000000000000000000000000000001",
	31318: "0x20c0000000000000000000000000000000000001",
}

// TempoFeeToken returns the fee token address for the given Tempo chain ID.
func TempoFeeToken(chainID int64) (string, bool) {
	addr, ok := tempoFeeTokenByChainID[chainID]
	return addr, ok
}
