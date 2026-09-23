"use strict";

function artifactSuffix(mode) {
  if (mode !== "private" && mode !== "public") throw new Error(`Invalid key mode: ${mode}`);
  return mode === "public" ? "-public" : "";
}

function setupArtifactName(version, mode) {
  return `Deep Legends Setup ${version}${artifactSuffix(mode)}.exe`;
}

function checksumArtifactName(mode) {
  return `SHA256SUMS${artifactSuffix(mode)}.txt`;
}

module.exports = { artifactSuffix, setupArtifactName, checksumArtifactName };
