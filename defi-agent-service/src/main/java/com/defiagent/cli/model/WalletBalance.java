package com.defiagent.cli.model;

/** Mirrors defi-cli's model.WalletBalance ("defi wallet balance"). */
public record WalletBalance(
        String chainId,
        String accountAddress,
        String assetType,
        String assetId,
        String symbol,
        AmountInfo balance,
        String fetchedAt
) {
}
