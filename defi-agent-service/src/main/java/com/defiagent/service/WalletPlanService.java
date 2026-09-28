package com.defiagent.service;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.cli.model.ActionStep;
import com.defiagent.cli.model.ExecutionAction;
import com.defiagent.cli.model.StepCall;
import com.defiagent.domain.ActionStatus;
import com.defiagent.domain.AgentAction;
import com.defiagent.repo.AgentActionRepository;
import com.defiagent.web.dto.UnsignedTx;
import com.defiagent.web.dto.WalletPlanResponse;
import com.defiagent.web.dto.WalletSubmittedRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.web.server.ResponseStatusException;

import java.math.BigInteger;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/**
 * Builds unsigned EVM transactions from a defi-cli plan so the browser wallet
 * (MetaMask) can be the signer. This service never holds a private key and
 * never calls {@code submit}.
 */
@Service
public class WalletPlanService {

    private static final Logger log = LoggerFactory.getLogger(WalletPlanService.class);

    private final AgentActionRepository repository;
    private final DefiCliClient cli;

    public WalletPlanService(AgentActionRepository repository, DefiCliClient cli) {
        this.repository = repository;
        this.cli = cli;
    }

    public WalletPlanResponse loadPlan(String actionId) {
        AgentAction stored = repository.findByActionId(actionId).orElse(null);
        String defiId = stored != null ? stored.getDefiActionId() : null;
        if (defiId == null || defiId.isBlank()) {
            defiId = actionId;
        }

        ExecutionAction planned;
        try {
            planned = cli.actionsShow(defiId);
        } catch (Exception e) {
            if (stored == null) {
                throw new ResponseStatusException(HttpStatus.NOT_FOUND, "action not found: " + actionId);
            }
            throw new ResponseStatusException(HttpStatus.CONFLICT,
                    "this action has no defi-cli plan (need a real from-address when planning); "
                            + "paper-only rows cannot be sent to MetaMask");
        }

        List<UnsignedTx> txs = flatten(planned);
        if (txs.isEmpty()) {
            throw new ResponseStatusException(HttpStatus.CONFLICT, "plan has no unsigned EVM calls to send");
        }

        String status = stored != null ? stored.getStatus().name() : planned.status();
        return new WalletPlanResponse(
                stored != null ? stored.getActionId() : planned.actionId(),
                planned.actionId(),
                status,
                planned.fromAddress(),
                planned.provider(),
                planned.chainId(),
                txs);
    }

    public AgentAction recordWalletSubmission(String actionId, WalletSubmittedRequest request) {
        AgentAction action = repository.findByActionId(actionId)
                .orElseThrow(() -> new ResponseStatusException(HttpStatus.NOT_FOUND, "action not found: " + actionId));
        if (request == null || request.txHashes() == null || request.txHashes().isEmpty()) {
            throw new ResponseStatusException(HttpStatus.BAD_REQUEST, "txHashes required");
        }
        String hash = request.txHashes().get(request.txHashes().size() - 1);
        if (hash == null || hash.isBlank()) {
            throw new ResponseStatusException(HttpStatus.BAD_REQUEST, "txHash required");
        }
        if (action.getStatus() == ActionStatus.CONFIRMED) {
            return action;
        }
        if (action.getStatus() == ActionStatus.PLANNED) {
            action.transitionTo(ActionStatus.QUEUED);
            action.transitionTo(ActionStatus.EXECUTING);
            action.transitionTo(ActionStatus.CONFIRMED);
        } else if (action.getStatus() == ActionStatus.QUEUED) {
            action.transitionTo(ActionStatus.EXECUTING);
            action.transitionTo(ActionStatus.CONFIRMED);
        } else if (action.getStatus() == ActionStatus.EXECUTING) {
            action.transitionTo(ActionStatus.CONFIRMED);
        } else if (action.getStatus() == ActionStatus.FAILED) {
            action.transitionTo(ActionStatus.QUEUED);
            action.transitionTo(ActionStatus.EXECUTING);
            action.transitionTo(ActionStatus.CONFIRMED);
        } else {
            throw new ResponseStatusException(HttpStatus.CONFLICT,
                    "cannot record wallet tx from status " + action.getStatus());
        }
        action.setTxHash(hash.length() > 80 ? hash.substring(0, 80) : hash);
        String note = "wallet broadcast — user signed in MetaMask, backend did not hold a key; hashes="
                + String.join(",", request.txHashes());
        action.setNote(note.length() > 480 ? note.substring(0, 480) : note);
        log.info("wallet submission recorded actionId={} txHash={}", actionId, hash);
        return repository.save(action);
    }

    static List<UnsignedTx> flatten(ExecutionAction planned) {
        List<UnsignedTx> out = new ArrayList<>();
        if (planned == null || planned.steps() == null) {
            return out;
        }
        String from = planned.fromAddress();
        for (ActionStep step : planned.steps()) {
            if (step == null) {
                continue;
            }
            String chainId = firstNonBlank(step.chainId(), planned.chainId());
            String chainHex = toChainIdHex(chainId);
            if (step.calls() != null && !step.calls().isEmpty()) {
                int i = 0;
                for (StepCall call : step.calls()) {
                    UnsignedTx tx = toTx(step, from, chainId, chainHex, call.target(), call.data(), call.value(),
                            step.stepId() + "#" + i);
                    if (tx != null) {
                        out.add(tx);
                    }
                    i++;
                }
            } else {
                UnsignedTx tx = toTx(step, from, chainId, chainHex, step.target(), step.data(), step.value(),
                        step.stepId());
                if (tx != null) {
                    out.add(tx);
                }
            }
        }
        return out;
    }

    private static UnsignedTx toTx(ActionStep step, String from, String chainId, String chainHex,
                                    String to, String data, String value, String id) {
        if (to == null || to.isBlank() || data == null || data.isBlank()) {
            return null;
        }
        return new UnsignedTx(
                id,
                step.type(),
                step.description(),
                chainId,
                chainHex,
                from,
                to,
                data,
                toValueHex(value));
    }

    static String toChainIdHex(String caipOrNumber) {
        if (caipOrNumber == null || caipOrNumber.isBlank()) {
            return "0x1";
        }
        String raw = caipOrNumber.trim();
        int colon = raw.lastIndexOf(':');
        String numeric = colon >= 0 ? raw.substring(colon + 1) : raw;
        if (numeric.startsWith("0x") || numeric.startsWith("0X")) {
            return numeric.toLowerCase(Locale.ROOT);
        }
        return "0x" + new BigInteger(numeric).toString(16);
    }

    static String toValueHex(String value) {
        if (value == null || value.isBlank() || "0".equals(value)) {
            return "0x0";
        }
        String v = value.trim();
        if (v.startsWith("0x") || v.startsWith("0X")) {
            return v.equalsIgnoreCase("0x") ? "0x0" : v;
        }
        return "0x" + new BigInteger(v).toString(16);
    }

    private static String firstNonBlank(String a, String b) {
        return a != null && !a.isBlank() ? a : b;
    }
}
