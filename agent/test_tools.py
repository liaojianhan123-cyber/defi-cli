"""
Unit tests for agent/tools.py — verifies tool dispatch builds the correct
defi-cli argument lists without invoking the binary.

Run with:  python -m pytest agent/test_tools.py -v
   or:     python agent/test_tools.py
"""

from __future__ import annotations

import sys
import unittest
from unittest.mock import patch

import tools


class _CapturedRun:
    """Replaces tools._run; stores the args list so tests can assert on it."""
    def __init__(self):
        self.last_args: list[str] | None = None
    def __call__(self, args: list[str]) -> dict:
        self.last_args = list(args)
        return {"_mock": True, "args": args}


# ─── Read-only tools ──────────────────────────────────────────────────────────

class TestReadOnlyDispatch(unittest.TestCase):

    def setUp(self):
        self.captured = _CapturedRun()
        self._patcher = patch.object(tools, "_run", self.captured)
        self._patcher.start()

    def tearDown(self):
        self._patcher.stop()

    def test_providers_list(self):
        tools.execute("providers_list", {})
        self.assertEqual(self.captured.last_args, ["providers", "list"])

    def test_yield_opportunities_basic(self):
        tools.execute("yield_opportunities", {"chain": "1"})
        self.assertEqual(
            self.captured.last_args,
            ["yield", "opportunities", "--chain", "1"],
        )

    def test_yield_opportunities_full(self):
        tools.execute("yield_opportunities", {
            "chain": "ethereum",
            "asset": "USDC",
            "providers": "aave,morpho,pendle",
            "limit": 5,
            "min_tvl_usd": 1_000_000,
            "min_apy": 3.5,
        })
        args = self.captured.last_args
        self.assertIn("--chain", args)
        self.assertIn("ethereum", args)
        self.assertIn("--asset", args)
        self.assertIn("USDC", args)
        self.assertIn("--providers", args)
        self.assertIn("aave,morpho,pendle", args)
        self.assertIn("--limit", args)
        self.assertIn("5", args)
        self.assertIn("--min-tvl-usd", args)
        self.assertIn("--min-apy", args)

    def test_lend_markets(self):
        tools.execute("lend_markets", {
            "provider": "aave",
            "chain": "1",
            "asset": "USDC",
        })
        self.assertEqual(
            self.captured.last_args,
            ["lend", "markets", "--provider", "aave", "--chain", "1", "--asset", "USDC"],
        )

    def test_lend_positions_with_type(self):
        tools.execute("lend_positions", {
            "provider": "compoundv3",
            "chain": "1",
            "address": "0xABC",
            "type": "supply",
        })
        args = self.captured.last_args
        self.assertEqual(args[:4], ["lend", "positions", "--provider", "compoundv3"])
        self.assertIn("--type", args)
        self.assertIn("supply", args)
        self.assertIn("--address", args)
        self.assertIn("0xABC", args)

    def test_yield_positions(self):
        tools.execute("yield_positions", {
            "chain": "base",
            "address": "0xdEaD",
            "providers": "pendle",
        })
        args = self.captured.last_args
        self.assertEqual(args[:2], ["yield", "positions"])
        self.assertIn("--providers", args)
        self.assertIn("pendle", args)

    def test_swap_quote(self):
        tools.execute("swap_quote", {
            "provider": "uniswap",
            "chain": "1",
            "from_asset": "USDC",
            "to_asset": "DAI",
            "amount": "1000000",
            "from_address": "0xABC",
        })
        args = self.captured.last_args
        self.assertEqual(args[:2], ["swap", "quote"])
        self.assertIn("--from-asset", args)
        self.assertIn("--to-asset", args)
        self.assertIn("--amount", args)
        self.assertIn("--from-address", args)

    def test_gas_price(self):
        tools.execute("gas_price", {"chain": "1"})
        self.assertEqual(self.captured.last_args, ["chains", "gas", "--chain", "1"])

    def test_unknown_tool(self):
        result = tools.execute("nonexistent_tool", {})
        self.assertIn("error", result)
        self.assertIn("unknown tool", result)

# ─── Execution: plan ──────────────────────────────────────────────────────────

class TestPlanDispatch(unittest.TestCase):

    def setUp(self):
        self.captured = _CapturedRun()
        self._patcher = patch.object(tools, "_run", self.captured)
        self._patcher.start()

    def tearDown(self):
        self._patcher.stop()

    def test_yield_deposit_plan_basic(self):
        tools.execute("yield_deposit_plan", {
            "provider": "aave",
            "chain": "1",
            "asset": "USDC",
            "amount": "1000000",
            "from_address": "0xABC",
        })
        args = self.captured.last_args
        self.assertEqual(args[:3], ["yield", "deposit", "plan"])
        self.assertIn("--provider", args)
        self.assertIn("aave", args)
        self.assertIn("--from-address", args)
        self.assertIn("0xABC", args)

    def test_yield_deposit_plan_morpho_with_vault(self):
        tools.execute("yield_deposit_plan", {
            "provider": "morpho",
            "chain": "1",
            "asset": "USDC",
            "amount": "1000000",
            "from_address": "0xABC",
            "vault_address": "0xVAULT",
        })
        args = self.captured.last_args
        self.assertIn("--vault-address", args)
        self.assertIn("0xVAULT", args)

    def test_lend_supply_plan_with_pool_override(self):
        tools.execute("lend_supply_plan", {
            "provider": "compoundv3",
            "chain": "1",
            "asset": "WETH",
            "amount": "1000000000000000000",
            "from_address": "0xABC",
            "pool_address": "0xCOMET",
        })
        args = self.captured.last_args
        self.assertEqual(args[:3], ["lend", "supply", "plan"])
        self.assertIn("--pool-address", args)
        self.assertIn("0xCOMET", args)

    def test_plan_blocks_disallowed_protocol(self):
        # Inject a temporary allowlist that excludes "fakedex"
        with patch.object(tools.guardrails, "ALLOWED_PROTOCOLS", ["aave"]):
            result = tools.execute("yield_deposit_plan", {
                "provider": "fakedex",
                "chain": "1",
                "asset": "USDC",
                "amount": "1000000",
                "from_address": "0xABC",
            })
            # _run should NOT have been called
            self.assertIsNone(self.captured.last_args)
            self.assertIn("not in the allowed list", result)

# ─── Execution: submit ────────────────────────────────────────────────────────

class TestSubmitDispatch(unittest.TestCase):

    def setUp(self):
        self.captured = _CapturedRun()
        self._patcher = patch.object(tools, "_run", self.captured)
        self._patcher.start()

    def tearDown(self):
        self._patcher.stop()

    def test_yield_deposit_submit(self):
        tools.execute("yield_deposit_submit", {"action_id": "act_xyz"})
        self.assertEqual(
            self.captured.last_args,
            ["yield", "deposit", "submit", "--action-id", "act_xyz"],
        )

    def test_lend_supply_submit(self):
        tools.execute("lend_supply_submit", {"action_id": "act_abc"})
        self.assertEqual(
            self.captured.last_args,
            ["lend", "supply", "submit", "--action-id", "act_abc"],
        )

    def test_submit_tools_are_in_execution_set(self):
        # The agent loop relies on this set to gate user confirmation.
        self.assertIn("yield_deposit_submit", tools.EXECUTION_SUBMIT_TOOLS)
        self.assertIn("lend_supply_submit",  tools.EXECUTION_SUBMIT_TOOLS)

# ─── Action management ───────────────────────────────────────────────────────

class TestActionMgmtDispatch(unittest.TestCase):

    def setUp(self):
        self.captured = _CapturedRun()
        self._patcher = patch.object(tools, "_run", self.captured)
        self._patcher.start()

    def tearDown(self):
        self._patcher.stop()

    def test_actions_list(self):
        tools.execute("actions_list", {"limit": 5})
        args = self.captured.last_args
        self.assertEqual(args[:2], ["actions", "list"])
        self.assertIn("--limit", args)
        self.assertIn("5", args)

    def test_action_show(self):
        tools.execute("action_show", {"action_id": "act_1"})
        self.assertEqual(
            self.captured.last_args,
            ["actions", "show", "--action-id", "act_1"],
        )

# ─── Schema sanity ────────────────────────────────────────────────────────────

class TestToolSchemas(unittest.TestCase):

    def test_all_schemas_have_name_and_input(self):
        for s in tools.TOOL_SCHEMAS:
            self.assertIn("name", s, f"schema missing name: {s}")
            self.assertIn("description", s)
            self.assertIn("input_schema", s)
            self.assertEqual(s["input_schema"]["type"], "object")

    def test_no_duplicate_tool_names(self):
        names = [s["name"] for s in tools.TOOL_SCHEMAS]
        self.assertEqual(len(names), len(set(names)),
                         "duplicate tool names in TOOL_SCHEMAS")

    def test_execution_submit_tools_are_declared(self):
        declared = {s["name"] for s in tools.TOOL_SCHEMAS}
        for submit_name in tools.EXECUTION_SUBMIT_TOOLS:
            self.assertIn(submit_name, declared,
                          f"{submit_name} is in EXECUTION_SUBMIT_TOOLS but not in TOOL_SCHEMAS")


if __name__ == "__main__":
    unittest.main(verbosity=2)
