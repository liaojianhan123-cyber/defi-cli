package com.defiagent.web.dto;

/**
 * One unsigned EVM transaction the browser wallet should present to the user.
 * {@code chainIdHex} is MetaMask's {@code wallet_switchEthereumChain} format (e.g. {@code 0x1}).
 */
public record UnsignedTx(
        String stepId,
        String type,
        String description,
        String chainId,
        String chainIdHex,
        String from,
        String to,
        String data,
        String valueHex
) {
}
