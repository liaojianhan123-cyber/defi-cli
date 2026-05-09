"""
Configuration for the DeFi Agent.
All values read from environment variables with safe defaults.
"""

import os

# ── Claude API ────────────────────────────────────────────────────────────────
ANTHROPIC_API_KEY = os.environ.get("ANTHROPIC_API_KEY", "")
CLAUDE_MODEL = os.environ.get("AGENT_MODEL", "claude-opus-4-5")

# ── defi-cli binary ───────────────────────────────────────────────────────────
# Path to the compiled defi binary. Defaults to "defi" (assumes it's in PATH).
DEFI_CLI_PATH = os.environ.get("DEFI_CLI_PATH", "defi")
DEFI_CLI_TIMEOUT = int(os.environ.get("DEFI_CLI_TIMEOUT", "30"))  # seconds

# ── Agent wallet ──────────────────────────────────────────────────────────────
# The agent uses DEFI_PRIVATE_KEY / DEFI_PRIVATE_KEY_FILE (passed through to
# the defi binary via environment); the agent itself never reads the key.
AGENT_WALLET_ADDRESS = os.environ.get("AGENT_WALLET_ADDRESS", "")

# ── Guardrails ────────────────────────────────────────────────────────────────
# Maximum USD value for a single transaction the agent is allowed to execute.
MAX_SINGLE_TX_USD = float(os.environ.get("AGENT_MAX_SINGLE_TX_USD", "10000"))

# Comma-separated list of allowed protocols (empty = all allowed).
ALLOWED_PROTOCOLS = [
    p.strip().lower()
    for p in os.environ.get("AGENT_ALLOWED_PROTOCOLS", "aave,morpho,pendle,compoundv3,moonwell").split(",")
    if p.strip()
]

# APY above this threshold triggers a risk warning.
HIGH_APY_WARNING_THRESHOLD = float(os.environ.get("AGENT_HIGH_APY_WARN", "50"))

# Minimum TVL (USD) before recommending a protocol for large amounts.
MIN_TVL_FOR_LARGE_AMOUNT = float(os.environ.get("AGENT_MIN_TVL_LARGE", "1_000_000"))
LARGE_AMOUNT_USD = float(os.environ.get("AGENT_LARGE_AMOUNT_USD", "1000"))
