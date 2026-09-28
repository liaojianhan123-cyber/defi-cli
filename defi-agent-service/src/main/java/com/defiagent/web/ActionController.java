package com.defiagent.web;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.cli.model.ActionGasEstimate;
import com.defiagent.domain.AgentAction;
import com.defiagent.repo.AgentActionRepository;
import com.defiagent.service.WalletPlanService;
import com.defiagent.web.dto.WalletPlanResponse;
import com.defiagent.web.dto.WalletSubmittedRequest;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.server.ResponseStatusException;

import java.util.List;

@RestController
@RequestMapping("/api/actions")
@Tag(name = "Actions", description = "Action lifecycle & history")
public class ActionController {

    private final AgentActionRepository repository;
    private final DefiCliClient cli;
    private final WalletPlanService walletPlanService;

    public ActionController(AgentActionRepository repository, DefiCliClient cli, WalletPlanService walletPlanService) {
        this.repository = repository;
        this.cli = cli;
        this.walletPlanService = walletPlanService;
    }

    @GetMapping("/{actionId}")
    @Operation(summary = "Get a single action by id")
    public ResponseEntity<AgentAction> get(@PathVariable String actionId) {
        return repository.findByActionId(actionId)
                .map(ResponseEntity::ok)
                .orElse(ResponseEntity.notFound().build());
    }

    @GetMapping
    @Operation(summary = "List recent actions for a session")
    public List<AgentAction> list(@RequestParam String sessionId) {
        return repository.findTop20BySessionIdOrderByCreatedAtDesc(sessionId);
    }

    @GetMapping("/{actionId}/estimate")
    @Operation(summary = "Real gas/fee estimate for the action's defi-cli plan (read-only, no signing involved)")
    public ActionGasEstimate estimate(@PathVariable String actionId) {
        AgentAction action = repository.findByActionId(actionId)
                .orElseThrow(() -> new ResponseStatusException(HttpStatus.NOT_FOUND, "action not found: " + actionId));
        if (action.getDefiActionId() == null || action.getDefiActionId().isBlank()) {
            throw new ResponseStatusException(HttpStatus.CONFLICT,
                    "action has no linked defi-cli plan (defiActionId is missing); it was likely created before the CLI integration or via the mock fallback path");
        }
        return cli.estimate(action.getDefiActionId());
    }

    @GetMapping("/{actionId}/wallet-plan")
    @Operation(summary = "Unsigned EVM calls from the defi-cli plan, for MetaMask eth_sendTransaction (no server-side signing)")
    public WalletPlanResponse walletPlan(@PathVariable String actionId) {
        return walletPlanService.loadPlan(actionId);
    }

    @PostMapping("/{actionId}/wallet-submitted")
    @Operation(summary = "Record tx hashes after the user signed in a browser wallet. Backend never broadcasts.")
    public AgentAction walletSubmitted(@PathVariable String actionId, @RequestBody WalletSubmittedRequest body) {
        return walletPlanService.recordWalletSubmission(actionId, body);
    }
}
