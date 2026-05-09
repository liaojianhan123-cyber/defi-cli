#!/usr/bin/env python3
"""
AI DeFi Agent — powered by Claude + defi-cli.

Usage:
  python defi_agent.py "把我的 5000 USDC 分配到以太坊上收益最高的稳定币池"
  python defi_agent.py --address 0xYourEOA --interactive
  python defi_agent.py --address 0xYourEOA "show my current yield positions on base"

Environment variables:
  ANTHROPIC_API_KEY        Required. Your Claude API key.
  DEFI_CLI_PATH            Path to defi binary (default: "defi").
  AGENT_WALLET_ADDRESS     Default wallet address.
  AGENT_MAX_SINGLE_TX_USD  Max USD per transaction (default: 10000).
  AGENT_ALLOWED_PROTOCOLS  Comma-separated protocol allowlist.
  DEFI_PRIVATE_KEY         Or DEFI_PRIVATE_KEY_FILE — for submit commands.
"""

from __future__ import annotations

import argparse
import json
import sys
from typing import Any

import anthropic

import config
import tools as tool_module

# ─── System prompt ────────────────────────────────────────────────────────────

SYSTEM_PROMPT = f"""You are a DeFi Agent powered by defi-cli. You help users analyze and interact \
with DeFi protocols safely and efficiently.

## What you can do
- **Read**: query yield opportunities, lending markets, rates, and wallet positions across \
Aave, Morpho, Pendle, Compound V3, Moonwell, and Kamino.
- **Plan**: construct transaction plans (yield deposits, lend supply) — these are dry-runs \
and do NOT broadcast anything.
- **Execute**: broadcast planned transactions after the user explicitly confirms.

## Safety rules (non-negotiable)
1. NEVER call `yield_deposit_submit` or `lend_supply_submit` unless the user has \
explicitly said "yes", "confirm", "go ahead", "execute", or equivalent in their most recent message.
2. Always show the plan details (action_id, estimated gas, steps) BEFORE asking for confirmation.
3. Warn the user clearly if:
   - APY > {config.HIGH_APY_WARNING_THRESHOLD}% (unusual, may indicate risk)
   - TVL < $1M and deposit > $1,000 (concentration risk)
   - Amount > ${config.MAX_SINGLE_TX_USD:,.0f} (exceeds agent limit)
4. Never borrow assets unless the user explicitly asks to borrow.
5. Prefer higher TVL protocols when multiple options exist with similar APY.

## Recommended workflow for deposit requests
1. Call `yield_opportunities` with the user's asset and chain.
2. Present a clear comparison table (protocol | type | APY | TVL | lockup).
3. Recommend the best option with reasoning.
4. Call `yield_deposit_plan` (or `lend_supply_plan`) to generate the action.
5. Show plan details: provider, chain, asset, amount, estimated gas, action_id.
6. Ask: "Shall I execute this? Reply **yes** to confirm."
7. Only after "yes": call the corresponding submit tool.
8. After submit: poll `action_status` once and report the tx hash.

## Output style
- Be concise, structured, and action-oriented.
- Use markdown tables when comparing multiple options.
- Show APY as percentage (e.g. "4.5%"), amounts in human-readable form (e.g. "1,000 USDC").
- Always mention which protocol and chain you are recommending.
- Respond in the same language the user writes in (Chinese or English).

## Current guardrails
- Allowed protocols: {', '.join(config.ALLOWED_PROTOCOLS) if config.ALLOWED_PROTOCOLS else 'all'}
- Max single transaction: ${config.MAX_SINGLE_TX_USD:,.0f} USD
"""

# ─── Colour helpers (terminal) ────────────────────────────────────────────────

def _c(code: str, text: str) -> str:
    """Wrap text in ANSI colour if stdout is a tty."""
    if not sys.stdout.isatty():
        return text
    return f"\033[{code}m{text}\033[0m"

def green(t: str) -> str:  return _c("32", t)
def yellow(t: str) -> str: return _c("33", t)
def cyan(t: str) -> str:   return _c("36", t)
def bold(t: str) -> str:   return _c("1",  t)
def red(t: str) -> str:    return _c("31", t)

# ─── Agent loop ───────────────────────────────────────────────────────────────

class DeFiAgent:
    def __init__(self, wallet_address: str = ""):
        if not config.ANTHROPIC_API_KEY:
            print(red("Error: ANTHROPIC_API_KEY is not set."))
            sys.exit(1)

        self.client = anthropic.Anthropic(api_key=config.ANTHROPIC_API_KEY)
        self.wallet_address = wallet_address or config.AGENT_WALLET_ADDRESS
        self.messages: list[dict] = []
        self._pending_confirmation: str | None = None  # action_id awaiting yes/no

    def _ask_confirmation(self, prompt: str) -> bool:
        """Prompt the user for explicit confirmation on the terminal."""
        print(f"\n{yellow('❓ Confirmation required:')}")
        print(f"   {prompt}")
        answer = input(f"\n{bold('   → Type YES to confirm, anything else to cancel: ')}").strip().lower()
        return answer in {"yes", "y", "确认", "是", "go", "ok", "confirm", "execute"}

    def _tool_label(self, name: str) -> str:
        icons = {
            "yield_opportunities": "📊",
            "lend_markets":        "📋",
            "lend_rates":          "📈",
            "yield_positions":     "💼",
            "lend_positions":      "💰",
            "swap_quote":          "🔄",
            "gas_price":           "⛽",
            "yield_deposit_plan":  "🗒️",
            "lend_supply_plan":    "🗒️",
            "yield_deposit_submit":"🚀",
            "lend_supply_submit":  "🚀",
            "actions_list":        "📜",
            "action_show":         "🔍",
            "action_status":       "🔍",
            "providers_list":      "🏦",
        }
        return icons.get(name, "🔧")

    def _process_turn(self) -> bool:
        """
        Send messages to Claude and process one full response (which may include
        multiple tool calls).  Returns True if the conversation should continue.
        """
        response = self.client.messages.create(
            model=config.CLAUDE_MODEL,
            max_tokens=4096,
            system=SYSTEM_PROMPT,
            tools=tool_module.TOOL_SCHEMAS,
            messages=self.messages,
        )

        if response.stop_reason == "end_turn":
            # Final text answer — print and return False to stop the loop.
            for block in response.content:
                if hasattr(block, "text"):
                    print(f"\n{cyan('💬 Agent:')} {block.text}\n")
            return False

        if response.stop_reason != "tool_use":
            print(red(f"Unexpected stop_reason: {response.stop_reason}"))
            return False

        # Append assistant message (tool calls) to history.
        self.messages.append({"role": "assistant", "content": response.content})

        tool_results: list[dict] = []

        for block in response.content:
            if block.type != "tool_use":
                continue

            name   = block.name
            inputs = block.input
            icon   = self._tool_label(name)

            print(f"\n{icon} {bold(name)}")
            if inputs:
                for k, v in inputs.items():
                    print(f"   {cyan(k)}: {v}")

            # ── Intercept submit tools for user confirmation ───────────────
            if name in tool_module.EXECUTION_SUBMIT_TOOLS:
                action_id = inputs.get("action_id", "unknown")
                confirmed = self._ask_confirmation(
                    f"Submit action {bold(action_id)} to the blockchain?"
                )
                if not confirmed:
                    result_str = json.dumps({
                        "status": "cancelled",
                        "message": "Transaction cancelled by user.",
                    })
                    print(f"   {yellow('⚠️  Cancelled.')}")
                else:
                    print(f"   {green('✅ Confirmed — broadcasting...')}")
                    result_str = tool_module.execute(name, inputs)
            else:
                result_str = tool_module.execute(name, inputs)

            # Preview of result (truncated for readability).
            preview = result_str[:300] + "…" if len(result_str) > 300 else result_str
            print(f"   {cyan('↩')} {preview}")

            tool_results.append({
                "type":        "tool_result",
                "tool_use_id": block.id,
                "content":     result_str,
            })

        self.messages.append({"role": "user", "content": tool_results})
        return True  # keep looping — Claude may want to do more

    def chat(self, user_message: str) -> None:
        """Send one user message and run the agent until it produces a final answer."""
        # Prepend wallet context if we have an address.
        if self.wallet_address:
            full_msg = f"[My wallet: {self.wallet_address}]\n\n{user_message}"
        else:
            full_msg = user_message

        self.messages.append({"role": "user", "content": full_msg})

        # Agentic loop — keep calling Claude until stop_reason == "end_turn".
        max_iterations = 12  # guard against runaway loops
        for _ in range(max_iterations):
            should_continue = self._process_turn()
            if not should_continue:
                break
        else:
            print(yellow("⚠️  Reached max iterations — agent stopped."))

# ─── Entry point ──────────────────────────────────────────────────────────────

def _build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        description="AI DeFi Agent powered by Claude + defi-cli",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python defi_agent.py "find the best USDC yield on ethereum"
  python defi_agent.py --address 0xABC "show my yield positions on base"
  python defi_agent.py --interactive
        """,
    )
    p.add_argument("query", nargs="?", help="Natural language DeFi query (omit for interactive)")
    p.add_argument("--address", "-a", default="", help="Your EVM wallet address")
    p.add_argument("--interactive", "-i", action="store_true", help="Start interactive chat mode")
    return p


def main() -> None:
    args = _build_parser().parse_args()
    agent = DeFiAgent(wallet_address=args.address)

    banner = bold("🤖 DeFi Agent") + f" [{config.CLAUDE_MODEL}]"
    if agent.wallet_address:
        banner += f"  |  wallet: {cyan(agent.wallet_address[:10])}…"

    print(f"\n{banner}")
    print("─" * 60)

    if args.interactive or not args.query:
        quit_hint = yellow('type your query, Ctrl-C or "exit" to quit')
        print(f"Interactive mode — {quit_hint}\n")
        while True:
            try:
                query = input(bold("You: ")).strip()
            except (KeyboardInterrupt, EOFError):
                print("\nBye!")
                break
            if not query:
                continue
            if query.lower() in {"exit", "quit", "q", "bye"}:
                print("Bye!")
                break
            agent.chat(query)
    else:
        agent.chat(args.query)


if __name__ == "__main__":
    main()
