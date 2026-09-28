package com.defiagent.web;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.cli.model.LendMarket;
import com.defiagent.cli.model.LendPosition;
import com.defiagent.cli.model.LendRate;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

@RestController
@RequestMapping("/api/v1/lend")
@Tag(name = "Lend", description = "Lending market/rate/position queries across Aave, Morpho, Moonwell, Compound V3, Kamino")
public class LendController {

    private final DefiCliClient cli;

    public LendController(DefiCliClient cli) {
        this.cli = cli;
    }

    @GetMapping("/markets")
    @Operation(summary = "List lending markets (supply/borrow APY, TVL) for a provider")
    public List<LendMarket> markets(@RequestParam String provider,
                                     @RequestParam String chain,
                                     @RequestParam(required = false) String asset) {
        return cli.lendMarkets(provider, chain, asset);
    }

    @GetMapping("/rates")
    @Operation(summary = "List current supply/borrow rates and utilization for a provider")
    public List<LendRate> rates(@RequestParam String provider,
                                 @RequestParam String chain,
                                 @RequestParam(required = false) String asset) {
        return cli.lendRates(provider, chain, asset);
    }

    @GetMapping("/positions")
    @Operation(summary = "List lending positions (supply/borrow/collateral) for an account")
    public List<LendPosition> positions(@RequestParam String provider,
                                         @RequestParam String chain,
                                         @RequestParam String address,
                                         @RequestParam(required = false, defaultValue = "all") String type) {
        return cli.lendPositions(provider, chain, address, type);
    }
}
