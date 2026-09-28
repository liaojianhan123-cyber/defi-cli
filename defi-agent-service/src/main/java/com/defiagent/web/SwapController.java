package com.defiagent.web;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.cli.model.SwapQuote;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/v1/swap")
@Tag(name = "Swap", description = "Swap quotes across 1inch, Uniswap, Jupiter, Tempo, TaikoSwap, Fibrous, Bungee")
public class SwapController {

    private final DefiCliClient cli;

    public SwapController(DefiCliClient cli) {
        this.cli = cli;
    }

    @GetMapping("/quote")
    @Operation(summary = "Get a swap quote between two assets (read-only, does not execute)")
    public SwapQuote quote(@RequestParam String provider,
                            @RequestParam String chain,
                            @RequestParam String fromAsset,
                            @RequestParam String toAsset,
                            @RequestParam String amount,
                            @RequestParam(required = false) String fromAddress) {
        return cli.swapQuote(provider, chain, fromAsset, toAsset, amount, fromAddress);
    }
}
