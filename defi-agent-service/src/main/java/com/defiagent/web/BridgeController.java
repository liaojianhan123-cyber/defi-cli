package com.defiagent.web;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.cli.model.BridgeQuote;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/v1/bridge")
@Tag(name = "Bridge", description = "Cross-chain bridge quotes across Across, LiFi, Bungee")
public class BridgeController {

    private final DefiCliClient cli;

    public BridgeController(DefiCliClient cli) {
        this.cli = cli;
    }

    @GetMapping("/quote")
    @Operation(summary = "Get a cross-chain bridge quote (read-only, does not execute)")
    public BridgeQuote quote(@RequestParam String provider,
                              @RequestParam String from,
                              @RequestParam String to,
                              @RequestParam String asset,
                              @RequestParam String amount) {
        return cli.bridgeQuote(provider, from, to, asset, amount);
    }
}
