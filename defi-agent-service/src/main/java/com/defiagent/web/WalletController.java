package com.defiagent.web;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.cli.model.WalletBalance;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/v1/wallet")
@Tag(name = "Wallet", description = "On-chain wallet balance queries (via defi-cli, no API key required)")
public class WalletController {

    private final DefiCliClient cli;

    public WalletController(DefiCliClient cli) {
        this.cli = cli;
    }

    @GetMapping("/balance")
    @Operation(summary = "Query native or ERC-20 token balance for an address")
    public WalletBalance balance(@RequestParam String chain,
                                  @RequestParam String address,
                                  @RequestParam(required = false) String asset) {
        return cli.walletBalance(chain, address, asset);
    }
}
