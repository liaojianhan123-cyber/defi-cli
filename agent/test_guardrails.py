"""Unit tests for agent/guardrails.py."""

import unittest
from unittest.mock import patch

import guardrails


class TestProtocolAllowlist(unittest.TestCase):

    def test_allowed_when_in_list(self):
        with patch.object(guardrails, "ALLOWED_PROTOCOLS", ["aave", "morpho"]):
            ok, reason = guardrails.check_protocol_allowed("aave")
            self.assertTrue(ok)
            self.assertEqual(reason, "")

    def test_blocked_when_not_in_list(self):
        with patch.object(guardrails, "ALLOWED_PROTOCOLS", ["aave"]):
            ok, reason = guardrails.check_protocol_allowed("scammycoin")
            self.assertFalse(ok)
            self.assertIn("not in the allowed list", reason)

    def test_empty_allowlist_permits_all(self):
        with patch.object(guardrails, "ALLOWED_PROTOCOLS", []):
            ok, _ = guardrails.check_protocol_allowed("anyprotocol")
            self.assertTrue(ok)

    def test_case_insensitive(self):
        with patch.object(guardrails, "ALLOWED_PROTOCOLS", ["aave"]):
            ok, _ = guardrails.check_protocol_allowed("AAVE")
            self.assertTrue(ok)


class TestAmountCheck(unittest.TestCase):

    def test_within_limit(self):
        with patch.object(guardrails, "MAX_SINGLE_TX_USD", 10_000):
            ok, _ = guardrails.check_amount_usd(5_000)
            self.assertTrue(ok)

    def test_at_limit(self):
        with patch.object(guardrails, "MAX_SINGLE_TX_USD", 10_000):
            ok, _ = guardrails.check_amount_usd(10_000)
            self.assertTrue(ok)

    def test_above_limit(self):
        with patch.object(guardrails, "MAX_SINGLE_TX_USD", 10_000):
            ok, reason = guardrails.check_amount_usd(10_001)
            self.assertFalse(ok)
            self.assertIn("exceeds the agent limit", reason)


class TestWarnings(unittest.TestCase):

    def test_high_apy_triggers_warning(self):
        with patch.object(guardrails, "HIGH_APY_WARNING_THRESHOLD", 50):
            self.assertIsNotNone(guardrails.warn_high_apy(75))
            self.assertIsNone(guardrails.warn_high_apy(20))

    def test_low_tvl_triggers_warning(self):
        with patch.object(guardrails, "MIN_TVL_FOR_LARGE_AMOUNT", 1_000_000), \
             patch.object(guardrails, "LARGE_AMOUNT_USD", 1000):
            # Big deposit into small pool — warn.
            self.assertIsNotNone(guardrails.warn_low_tvl(tvl_usd=500_000, amount_usd=5000))
            # Small deposit — no warn.
            self.assertIsNone(guardrails.warn_low_tvl(tvl_usd=500_000, amount_usd=100))
            # Big pool — no warn even for big deposit.
            self.assertIsNone(guardrails.warn_low_tvl(tvl_usd=2_000_000, amount_usd=5000))


class TestRunAll(unittest.TestCase):

    def test_collects_block_and_warn_separately(self):
        with patch.object(guardrails, "ALLOWED_PROTOCOLS", ["aave"]), \
             patch.object(guardrails, "MAX_SINGLE_TX_USD", 100), \
             patch.object(guardrails, "HIGH_APY_WARNING_THRESHOLD", 50):
            issues = guardrails.run_all(
                provider="badcoin",   # blocks
                amount_usd=200,        # blocks
                apy=99,                # warns
            )
            blocks = [i for i in issues if i.startswith("BLOCK:")]
            warns  = [i for i in issues if i.startswith("WARN:")]
            self.assertEqual(len(blocks), 2)
            self.assertEqual(len(warns), 1)


if __name__ == "__main__":
    unittest.main(verbosity=2)
