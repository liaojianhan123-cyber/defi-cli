package com.defiagent.intent;

import org.springframework.stereotype.Component;

import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * Rule-based intent parser (Chinese + English).
 *
 * <p>This is deliberately dependency-free so the demo runs with NO LLM API key.
 * It is designed as a drop-in: replace {@link #parse(String)} with an LLM call
 * (Spring AI / OpenAI / Claude) that returns the same {@link Intent} structure,
 * and the rest of the pipeline is unchanged.
 */
@Component
public class IntentParser {

    private static final Pattern AMOUNT = Pattern.compile("(\\d+(?:\\.\\d+)?)");
    private static final Pattern ASSET =
            Pattern.compile("\\b(USDC|USDT|DAI|ETH|WETH|BTC)\\b", Pattern.CASE_INSENSITIVE);
    private static final Pattern ADDRESS = Pattern.compile("\\b(0x[a-fA-F0-9]{40})\\b");

    public Intent parse(String message) {
        if (message == null || message.isBlank()) {
            return Intent.help();
        }
        String text = message.trim();
        String lower = text.toLowerCase();

        if (matchesAny(lower, "确认", "确定", "执行", "yes", "confirm", "go ahead", "proceed", "ok")) {
            return new Intent(Intent.Type.CONFIRM, null, null, null, null);
        }
        if (matchesAny(lower, "状态", "查询", "进度", "status", "check")) {
            return new Intent(Intent.Type.STATUS, null, null, null, null);
        }
        if (matchesAny(lower, "存", "存入", "收益", "理财", "deposit", "supply", "earn", "yield", "invest")) {
            return new Intent(Intent.Type.DEPOSIT, extractAsset(text), extractChain(lower), extractAmount(text), extractAddress(text));
        }
        return Intent.help();
    }

    private String extractAsset(String text) {
        Matcher m = ASSET.matcher(text);
        return m.find() ? m.group(1).toUpperCase() : "USDC";
    }

    private String extractChain(String lower) {
        if (lower.contains("base")) {
            return "base";
        }
        if (lower.contains("arbitrum") || lower.contains("arb")) {
            return "arbitrum";
        }
        if (lower.contains("optimism") || lower.contains("op ")) {
            return "optimism";
        }
        // default & explicit ethereum / 以太坊
        return "ethereum";
    }

    private String extractAmount(String text) {
        Matcher m = AMOUNT.matcher(text);
        return m.find() ? m.group(1) : "1000";
    }

    /** Optional EVM wallet address (0x...) mentioned in the message; null if absent. */
    private String extractAddress(String text) {
        Matcher m = ADDRESS.matcher(text);
        return m.find() ? m.group(1) : null;
    }

    private boolean matchesAny(String haystack, String... needles) {
        for (String n : needles) {
            if (haystack.contains(n)) {
                return true;
            }
        }
        return false;
    }
}
