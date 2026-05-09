"""
Tool definitions (schemas for Claude) + execution logic (subprocess calls to defi-cli).

Each tool maps 1-to-1 with a defi-cli command group.
Execution tools (plan/submit) go through guardrails before running.
"""

from __future__ import annotations

import json
import subprocess
from typing import Any

import config
import guardrails

# ─── Claude tool schemas ──────────────────────────────────────────────────────

TOOL_SCHEMAS: list[dict] = [
    {
        "name": "providers_list",
        "description": "List all available DeFi providers and their capabilities.",
        "input_schema": {"type": "object", "properties": {}},
    },
    {
        "name": "yield_opportunities",
        "description": (
            "Find yield opportunities (lending pools, AMM pools) across DeFi protocols. "
            "Returns APY breakdown (base + reward + total), TVL, liquidity, lockup days, "
            "and withdrawal terms. Use this to compare protocols before recommending one."
        ),
        "input_schema": {
            "type": "object",
            "properties": {
                "chain": {
                    "type": "string",
                    "description": "Chain name or ID (e.g. '1', 'ethereum', 'base', 'arbitrum')",
                },
                "asset": {
                    "type": "string",
                    "description": "Asset symbol filter (e.g. 'USDC', 'ETH', 'USDT'). Omit to see all assets.",
                },
                "providers": {
                    "type": "string",
                    "description": "Comma-separated provider filter (e.g. 'aave,morpho,pendle,compoundv3'). Omit for all.",
                },
                "limit": {
                    "type": "integer",
                    "description": "Max results (default 10).",
                    "default": 10,
                },
                "min_tvl_usd": {
                    "type": "number",
                    "description": "Minimum TVL in USD to filter out tiny pools.",
                },
                "min_apy": {
                    "type": "number",
                    "description": "Minimum APY percentage filter.",
                },
                "sort_by": {
                    "type": "string",
                    "description": "Sort field: 'apy_total' (default) or 'tvl_usd'.",
                },
            },
            "required": ["chain"],
        },
    },
    {
        "name": "lend_markets",
        "description": "Get lending market data (supply APY, borrow APY, TVL) for a specific protocol.",
        "input_schema": {
            "type": "object",
            "properties": {
                "provider": {
                    "type": "string",
                    "description": "Provider: aave | morpho | compoundv3 | moonwell | kamino",
                },
                "chain": {"type": "string"},
                "asset": {"type": "string", "description": "Asset symbol (optional filter)."},
            },
            "required": ["provider", "chain"],
        },
    },
    {
        "name": "lend_rates",
        "description": "Get current supply/borrow rates and utilization for a lending protocol.",
        "input_schema": {
            "type": "object",
            "properties": {
                "provider": {"type": "string"},
                "chain": {"type": "string"},
                "asset": {"type": "string"},
            },
            "required": ["provider", "chain"],
        },
    },
    {
        "name": "yield_positions",
        "description": "Check current yield positions (deposits, LP holdings) for a wallet.",
        "input_schema": {
            "type": "object",
            "properties": {
                "chain": {"type": "string"},
                "address": {"type": "string", "description": "EVM wallet address (0x...)"},
                "providers": {
                    "type": "string",
                    "description": "Comma-separated providers. Omit for all.",
                },
            },
            "required": ["chain", "address"],
        },
    },
    {
        "name": "lend_positions",
        "description": "Check current lending/borrowing positions for a wallet.",
        "input_schema": {
            "type": "object",
            "properties": {
                "provider": {"type": "string"},
                "chain": {"type": "string"},
                "address": {"type": "string"},
                "type": {
                    "type": "string",
                    "enum": ["all", "supply", "borrow", "collateral"],
                    "description": "Position type filter (default: all).",
                },
            },
            "required": ["provider", "chain", "address"],
        },
    },
    {
        "name": "swap_quote",
        "description": "Get a swap quote between two assets. Read-only — does not execute.",
        "input_schema": {
            "type": "object",
            "properties": {
                "provider": {
                    "type": "string",
                    "description": "Swap provider: uniswap | tempo | taikoswap",
                },
                "chain": {"type": "string"},
                "from_asset": {"type": "string", "description": "Input token symbol or address."},
                "to_asset": {"type": "string", "description": "Output token symbol or address."},
                "amount": {
                    "type": "string",
                    "description": "Input amount in base units (e.g. '1000000' for 1 USDC).",
                },
                "from_address": {
                    "type": "string",
                    "description": "Sender address (required for Uniswap).",
                },
            },
            "required": ["provider", "chain", "from_asset", "to_asset", "amount"],
        },
    },
    {
        "name": "gas_price",
        "description": "Get current gas prices (base fee, priority fee) for a chain.",
        "input_schema": {
            "type": "object",
            "properties": {"chain": {"type": "string"}},
            "required": ["chain"],
        },
    },
    # ── Execution: plan (creates action, does NOT broadcast) ──────────────────
    {
        "name": "yield_deposit_plan",
        "description": (
            "Plan a yield deposit. Creates a signed-ready action plan and returns an "
            "action_id. Does NOT broadcast the transaction — user must confirm first. "
            "Use after choosing a protocol from yield_opportunities."
        ),
        "input_schema": {
            "type": "object",
            "properties": {
                "provider": {
                    "type": "string",
                    "description": "Provider: aave | morpho | compoundv3 | moonwell",
                },
                "chain": {"type": "string"},
                "asset": {"type": "string", "description": "Asset to deposit (e.g. 'USDC')."},
                "amount": {
                    "type": "string",
                    "description": "Amount in base units (e.g. '1000000' = 1 USDC with 6 decimals).",
                },
                "from_address": {"type": "string", "description": "Sender wallet address."},
                "vault_address": {
                    "type": "string",
                    "description": "Morpho vault address (required for morpho).",
                },
                "pool_address": {
                    "type": "string",
                    "description": "Explicit pool/Comet address (optional override).",
                },
            },
            "required": ["provider", "chain", "asset", "amount", "from_address"],
        },
    },
    {
        "name": "lend_supply_plan",
        "description": (
            "Plan a lending supply. Creates action plan, does NOT broadcast. "
            "Returns action_id for user review."
        ),
        "input_schema": {
            "type": "object",
            "properties": {
                "provider": {"type": "string"},
                "chain": {"type": "string"},
                "asset": {"type": "string"},
                "amount": {"type": "string", "description": "Base units."},
                "from_address": {"type": "string"},
                "pool_address": {"type": "string", "description": "Optional pool override."},
            },
            "required": ["provider", "chain", "asset", "amount", "from_address"],
        },
    },
    # ── Execution: submit (broadcasts — REQUIRES user confirmation) ───────────
    {
        "name": "yield_deposit_submit",
        "description": (
            "Broadcast a yield deposit action. "
            "ONLY call this after the user explicitly types 'yes' or 'confirm'. "
            "Returns transaction hash."
        ),
        "input_schema": {
            "type": "object",
            "properties": {
                "action_id": {"type": "string", "description": "Action ID from yield_deposit_plan."},
            },
            "required": ["action_id"],
        },
    },
    {
        "name": "lend_supply_submit",
        "description": (
            "Broadcast a lend supply action. "
            "ONLY call this after the user explicitly types 'yes' or 'confirm'."
        ),
        "input_schema": {
            "type": "object",
            "properties": {
                "action_id": {"type": "string"},
            },
            "required": ["action_id"],
        },
    },
    # ── Action management ─────────────────────────────────────────────────────
    {
        "name": "actions_list",
        "description": "List all planned and submitted actions (history).",
        "input_schema": {
            "type": "object",
            "properties": {
                "limit": {"type": "integer", "default": 10},
            },
        },
    },
    {
        "name": "action_show",
        "description": "Show full details of a specific action by ID.",
        "input_schema": {
            "type": "object",
            "properties": {"action_id": {"type": "string"}},
            "required": ["action_id"],
        },
    },
    {
        "name": "action_status",
        "description": "Check on-chain status of a submitted action (confirmed/pending/failed).",
        "input_schema": {
            "type": "object",
            "properties": {"action_id": {"type": "string"}},
            "required": ["action_id"],
        },
    },
]

# Tools that broadcast transactions and REQUIRE interactive user confirmation.
EXECUTION_SUBMIT_TOOLS = {"yield_deposit_submit", "lend_supply_submit"}

# ─── defi-cli subprocess execution ───────────────────────────────────────────

def _run(args: list[str]) -> dict[str, Any]:
    """Run a defi-cli command and return the parsed JSON envelope."""
    cmd = [config.DEFI_CLI_PATH, "--results-only"] + args
    try:
        proc = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=config.DEFI_CLI_TIMEOUT,
        )
    except subprocess.TimeoutExpired:
        return {"error": f"defi-cli timed out after {config.DEFI_CLI_TIMEOUT}s"}
    except FileNotFoundError:
        return {
            "error": (
                f"defi binary not found at '{config.DEFI_CLI_PATH}'. "
                "Build with: cd /path/to/defi-cli && go build -o defi ./cmd/defi"
            )
        }

    raw = (proc.stdout or proc.stderr or "").strip()
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return {"raw_output": raw, "exit_code": proc.returncode}


def execute(name: str, inputs: dict[str, Any]) -> str:
    """
    Dispatch a tool call to the appropriate defi-cli command.
    Returns JSON string (always — even errors).
    """
    result = _dispatch(name, inputs)
    return json.dumps(result, indent=2)


def _dispatch(name: str, inputs: dict[str, Any]) -> dict[str, Any]:
    g = inputs.get  # shorthand

    # ── Read-only ─────────────────────────────────────────────────────────────
    if name == "providers_list":
        return _run(["providers", "list"])

    if name == "yield_opportunities":
        args = ["yield", "opportunities", "--chain", g("chain")]
        if g("asset"):       args += ["--asset", g("asset")]
        if g("providers"):   args += ["--providers", g("providers")]
        if g("limit"):       args += ["--limit", str(g("limit"))]
        if g("min_tvl_usd"): args += ["--min-tvl-usd", str(g("min_tvl_usd"))]
        if g("min_apy"):     args += ["--min-apy", str(g("min_apy"))]
        if g("sort_by"):     args += ["--sort-by", g("sort_by")]
        return _run(args)

    if name == "lend_markets":
        args = ["lend", "markets",
                "--provider", g("provider"),
                "--chain",    g("chain")]
        if g("asset"): args += ["--asset", g("asset")]
        return _run(args)

    if name == "lend_rates":
        args = ["lend", "rates",
                "--provider", g("provider"),
                "--chain",    g("chain")]
        if g("asset"): args += ["--asset", g("asset")]
        return _run(args)

    if name == "yield_positions":
        args = ["yield", "positions",
                "--chain",   g("chain"),
                "--address", g("address")]
        if g("providers"): args += ["--providers", g("providers")]
        return _run(args)

    if name == "lend_positions":
        args = ["lend", "positions",
                "--provider", g("provider"),
                "--chain",    g("chain"),
                "--address",  g("address")]
        if g("type"): args += ["--type", g("type")]
        return _run(args)

    if name == "swap_quote":
        args = ["swap", "quote",
                "--provider",   g("provider"),
                "--chain",      g("chain"),
                "--from-asset", g("from_asset"),
                "--to-asset",   g("to_asset"),
                "--amount",     g("amount")]
        if g("from_address"): args += ["--from-address", g("from_address")]
        return _run(args)

    if name == "gas_price":
        return _run(["chains", "gas", "--chain", g("chain")])

    # ── Execution: plan ───────────────────────────────────────────────────────
    if name == "yield_deposit_plan":
        ok, reason = guardrails.check_protocol_allowed(g("provider", ""))
        if not ok:
            return {"error": reason}
        args = ["yield", "deposit", "plan",
                "--provider",     g("provider"),
                "--chain",        g("chain"),
                "--asset",        g("asset"),
                "--amount",       g("amount"),
                "--from-address", g("from_address")]
        if g("vault_address"): args += ["--vault-address", g("vault_address")]
        if g("pool_address"):  args += ["--pool-address",  g("pool_address")]
        return _run(args)

    if name == "lend_supply_plan":
        ok, reason = guardrails.check_protocol_allowed(g("provider", ""))
        if not ok:
            return {"error": reason}
        args = ["lend", "supply", "plan",
                "--provider",     g("provider"),
                "--chain",        g("chain"),
                "--asset",        g("asset"),
                "--amount",       g("amount"),
                "--from-address", g("from_address")]
        if g("pool_address"): args += ["--pool-address", g("pool_address")]
        return _run(args)

    # ── Execution: submit (only reached after user confirmation in agent loop) ─
    if name == "yield_deposit_submit":
        return _run(["yield", "deposit", "submit", "--action-id", g("action_id")])

    if name == "lend_supply_submit":
        return _run(["lend", "supply", "submit", "--action-id", g("action_id")])

    # ── Action management ─────────────────────────────────────────────────────
    if name == "actions_list":
        args = ["actions", "list"]
        if g("limit"): args += ["--limit", str(g("limit"))]
        return _run(args)

    if name == "action_show":
        return _run(["actions", "show", "--action-id", g("action_id")])

    if name == "action_status":
        # defi-cli doesn't have a standalone status command;
        # reuse actions show which includes on-chain status.
        return _run(["actions", "show", "--action-id", g("action_id")])

    return {"error": f"unknown tool: {name}"}
