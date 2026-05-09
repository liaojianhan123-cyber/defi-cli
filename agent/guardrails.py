"""
Safety guardrails for the DeFi Agent.

All guardrails return (ok: bool, reason: str).
The agent checks these BEFORE executing any on-chain action.
"""

from __future__ import annotations
from config import (
    MAX_SINGLE_TX_USD,
    ALLOWED_PROTOCOLS,
    HIGH_APY_WARNING_THRESHOLD,
    MIN_TVL_FOR_LARGE_AMOUNT,
    LARGE_AMOUNT_USD,
)


def check_protocol_allowed(provider: str) -> tuple[bool, str]:
    """Reject providers not in the allowlist (empty allowlist = all allowed)."""
    if not ALLOWED_PROTOCOLS:
        return True, ""
    if provider.lower() not in ALLOWED_PROTOCOLS:
        return False, (
            f"Protocol '{provider}' is not in the allowed list: "
            f"{', '.join(ALLOWED_PROTOCOLS)}. "
            "Set AGENT_ALLOWED_PROTOCOLS env var to expand the list."
        )
    return True, ""


def check_amount_usd(amount_usd: float) -> tuple[bool, str]:
    """Block transactions above the configured maximum."""
    if amount_usd > MAX_SINGLE_TX_USD:
        return False, (
            f"Transaction amount ${amount_usd:,.2f} exceeds the agent limit "
            f"of ${MAX_SINGLE_TX_USD:,.2f}. "
            "Increase AGENT_MAX_SINGLE_TX_USD or split the transaction."
        )
    return True, ""


def warn_high_apy(apy: float) -> str | None:
    """Return a warning string if APY looks suspiciously high, else None."""
    if apy > HIGH_APY_WARNING_THRESHOLD:
        return (
            f"⚠️  APY {apy:.1f}% is unusually high (>{HIGH_APY_WARNING_THRESHOLD}%). "
            "This may indicate a new or low-liquidity pool with elevated risk."
        )
    return None


def warn_low_tvl(tvl_usd: float, amount_usd: float) -> str | None:
    """Return a warning if TVL is low relative to the deposit size."""
    if amount_usd >= LARGE_AMOUNT_USD and tvl_usd < MIN_TVL_FOR_LARGE_AMOUNT:
        return (
            f"⚠️  Pool TVL is ${tvl_usd:,.0f}, which is low for a "
            f"${amount_usd:,.0f} deposit. Consider splitting across "
            "multiple protocols to reduce concentration risk."
        )
    return None


def run_all(
    provider: str,
    amount_usd: float = 0,
    apy: float = 0,
    tvl_usd: float = 0,
) -> list[str]:
    """
    Run all guardrails and return a list of blocking errors + warnings.
    Blocking errors have the prefix "BLOCK:"; warnings have "WARN:".
    """
    issues: list[str] = []

    ok, reason = check_protocol_allowed(provider)
    if not ok:
        issues.append(f"BLOCK: {reason}")

    if amount_usd:
        ok, reason = check_amount_usd(amount_usd)
        if not ok:
            issues.append(f"BLOCK: {reason}")

    if apy:
        w = warn_high_apy(apy)
        if w:
            issues.append(f"WARN: {w}")

    if tvl_usd and amount_usd:
        w = warn_low_tvl(tvl_usd, amount_usd)
        if w:
            issues.append(f"WARN: {w}")

    return issues
