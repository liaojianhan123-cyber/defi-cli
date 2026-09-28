package com.defiagent.service;

import java.math.BigDecimal;
import java.util.Locale;
import java.util.Map;

/** Converts a human decimal amount (e.g. "1000") into base units for defi-cli calls. */
final class AssetUnits {

    private static final Map<String, Integer> KNOWN_DECIMALS = Map.of(
            "USDC", 6,
            "USDT", 6,
            "DAI", 18,
            "ETH", 18,
            "WETH", 18,
            "BTC", 8,
            "WBTC", 8
    );

    private AssetUnits() {
    }

    static String toBaseUnits(String decimalAmount, String asset) {
        int decimals = KNOWN_DECIMALS.getOrDefault(asset == null ? "" : asset.toUpperCase(Locale.ROOT), 18);
        return new BigDecimal(decimalAmount).movePointRight(decimals).toBigInteger().toString();
    }
}
