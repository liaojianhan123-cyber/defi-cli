package com.defiagent.web.dto;

import java.util.List;

public record WalletSubmittedRequest(String fromAddress, List<String> txHashes) {
}
