"use strict";

const fs = require("node:fs");

function numericBounds(value) {
  if (!value || typeof value !== "object") return null;
  const width = Math.round(Number(value.width));
  const height = Math.round(Number(value.height));
  const x = Math.round(Number(value.x));
  const y = Math.round(Number(value.y));
  if (![width, height, x, y].every(Number.isFinite) || width <= 0 || height <= 0) return null;
  return { width, height, x, y, maximized: value.maximized === true };
}

function intersectsDisplay(bounds, displays) {
  return (Array.isArray(displays) ? displays : []).some((display) => {
    const area = display?.workArea;
    if (!area) return false;
    const left = Number(area.x);
    const top = Number(area.y);
    const right = left + Number(area.width);
    const bottom = top + Number(area.height);
    return [left, top, right, bottom].every(Number.isFinite)
      && bounds.x < right && bounds.x + bounds.width > left
      && bounds.y < bottom && bounds.y + bounds.height > top;
  });
}

function readWindowBounds(filePath, displays) {
  try {
    const bounds = numericBounds(JSON.parse(fs.readFileSync(filePath, "utf8")));
    return bounds && intersectsDisplay(bounds, displays) ? bounds : null;
  } catch (_) {
    return null;
  }
}

function writeWindowBounds(filePath, bounds) {
  try {
    const value = numericBounds(bounds);
    if (!value) return false;
    fs.writeFileSync(filePath, `${JSON.stringify(value)}\n`, { mode: 0o600 });
    return true;
  } catch (_) {
    return false;
  }
}

module.exports = { intersectsDisplay, readWindowBounds, writeWindowBounds };
