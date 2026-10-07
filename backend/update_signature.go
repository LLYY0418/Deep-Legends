//go:build license

package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
)

func verifyUpdateManifestTrust(manifest updateManifest, keys map[string]ed25519.PublicKey) error {
	if manifest.SignedManifest == nil {
		return errors.New("更新清单缺少可信签名，请从发布页手动下载")
	}
	data, err := verifySignedEnvelope(*manifest.SignedManifest, keys, "DL-UPDATE-MANIFEST-V1")
	if err != nil {
		return errors.New("更新清单签名无效，请从发布页手动下载")
	}
	var signed updateManifest
	if strictLicenseJSON(data, &signed) != nil || signed.SignedManifest != nil {
		return errors.New("更新清单签名载荷无效")
	}
	manifest.SignedManifest = nil
	left, _ := json.Marshal(manifest)
	right, _ := json.Marshal(signed)
	if !bytes.Equal(left, right) {
		return errors.New("更新清单与签名内容不一致")
	}
	return validateUpdateManifest(signed)
}
