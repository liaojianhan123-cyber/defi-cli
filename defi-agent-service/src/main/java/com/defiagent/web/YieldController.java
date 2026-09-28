package com.defiagent.web;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.cli.model.YieldOpportunity;
import com.defiagent.cli.model.YieldPosition;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

/**
 * Real defi-cli-backed yield queries, distinct from the legacy mock-data
 * {@code /api/chat} deposit flow ({@link com.defiagent.yield.YieldService}).
 */
@RestController
@RequestMapping("/api/v1/yield")
@Tag(name = "Yield", description = "Yield opportunity comparison and position queries across DeFi protocols")
public class YieldController {

    private final DefiCliClient cli;

    public YieldController(DefiCliClient cli) {
        this.cli = cli;
    }

    @GetMapping("/opportunities")
    @Operation(summary = "Rank yield opportunities by APY/TVL across protocols")
    public List<YieldOpportunity> opportunities(@RequestParam String chain,
                                                 @RequestParam(required = false) String asset,
                                                 @RequestParam(required = false) String providers,
                                                 @RequestParam(required = false) Integer limit,
                                                 @RequestParam(required = false) Double minTvlUsd,
                                                 @RequestParam(required = false) Double minApy,
                                                 @RequestParam(required = false) String sortBy) {
        return cli.yieldOpportunities(chain, asset, providers, limit, minTvlUsd, minApy, sortBy);
    }

    @GetMapping("/positions")
    @Operation(summary = "List current yield positions (deposits, LP holdings) for a wallet")
    public List<YieldPosition> positions(@RequestParam String chain,
                                          @RequestParam String address,
                                          @RequestParam(required = false) String providers) {
        return cli.yieldPositions(chain, address, providers);
    }
}
