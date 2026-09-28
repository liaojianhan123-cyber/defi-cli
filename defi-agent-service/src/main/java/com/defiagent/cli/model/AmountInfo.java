package com.defiagent.cli.model;

/** Base-units + human-decimal amount pair, mirrors defi-cli's AmountInfo. */
public record AmountInfo(String amountBaseUnits, String amountDecimal, Integer decimals) {
}
