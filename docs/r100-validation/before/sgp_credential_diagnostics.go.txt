package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

type sgpCredentialIdentity struct {
	owner  *LCUClient
	kind   sgpTokenKind
	digest [32]byte
}

// Only a process-local ordinal is exported. Never export a token or its hash.
// Account/client changes and token kinds are isolated; at most 32 digests stay
// in memory. Eviction gives a new ordinal, never reuses another credential's ID.
func (p *sgpProvider) credentialVersion(client *LCUClient, kind sgpTokenKind, token string) string {
	key := sgpCredentialIdentity{client, kind, sha256.Sum256([]byte(token))}
	p.mu.Lock()
	defer p.mu.Unlock()
	if id := p.credentialVersions[key]; id != "" {
		return id
	}
	if p.credentialVersions == nil {
		p.credentialVersions = make(map[sgpCredentialIdentity]string)
	}
	if len(p.credentialOrder) >= 32 {
		delete(p.credentialVersions, p.credentialOrder[0])
		p.credentialOrder = p.credentialOrder[1:]
	}
	id := newDiagnosticTrace("credential")
	p.credentialVersions[key] = id
	p.credentialOrder = append(p.credentialOrder, key)
	return id
}

func (p *sgpProvider) readTokenJSON(ctx context.Context, client *LCUClient, path string, out any) (resultErr error) {
	started := time.Now()
	fields := map[string]any{"event": "sgp_token_read", "token_endpoint": path, "stage": "read"}
	defer func() {
		fields["error_kind"], fields["duration_ms"] = diagnosticErrorKind(resultErr), time.Since(started).Milliseconds()
		if status, _, _ := watchFailureDetail(resultErr); status > 0 {
			fields["http_status"] = status
		}
		fields["context_error"] = diagnosticErrorKind(ctx.Err())
		p.recordObservation(fields)
	}()
	data, err := client.GetBytesContext(ctx, path)
	if err != nil {
		return err
	}
	fields["http_status"], fields["body_bytes"], fields["body_shape"] = 200, len(data), diagnosticPayloadPrefixShape(data)
	fields["stage"] = "decode"
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode token response: %w", err)
	}
	fields["stage"] = "decoded"
	return nil
}

func (p *sgpProvider) recordEmptyToken(ctx context.Context, kind string) {
	p.recordObservation(map[string]any{"event": "sgp_token_read", "token_kind": kind, "stage": "validate", "error_kind": "empty-token"})
}
