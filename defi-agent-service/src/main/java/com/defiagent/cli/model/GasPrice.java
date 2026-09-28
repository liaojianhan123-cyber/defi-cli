package com.defiagent.cli.model;

import java.util.List;

/** Mirrors defi-cli's model.GasPrice ("defi chains gas"). */
public record GasPrice(
        String chainId,
        String chainName,
        Long blockNumber,
        Boolean eip1559,
        String baseFeeGwei,
        String priorityFeeGwei,
        String gasPriceGwei,
        List<String> warnings,
        String fetchedAt
) {
}
