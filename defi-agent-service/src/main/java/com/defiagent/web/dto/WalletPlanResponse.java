package com.defiagent.web.dto;

import java.util.List;

public record WalletPlanResponse(
        String actionId,
        String defiActionId,
        String status,
        String fromAddress,
        String provider,
        String chain,
        List<UnsignedTx> transactions
) {
}
