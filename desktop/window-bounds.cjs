"use strict";

const WINDOW_MIN_WIDTH = 1080;
const WINDOW_MIN_HEIGHT = 680;
// High-resolution displays need a materially larger default than 1680x1050,
// while a ceiling keeps the app from opening nearly full-screen on 4K panels.
const WINDOW_MAX_WIDTH = 2560;
const WINDOW_MAX_HEIGHT = 1600;

function windowBoundsForWorkArea(workAreaSize) {
  const width = Math.min(WINDOW_MAX_WIDTH, Math.max(WINDOW_MIN_WIDTH, Math.round(Number(workAreaSize?.width) * 0.90)));
  const height = Math.min(WINDOW_MAX_HEIGHT, Math.max(WINDOW_MIN_HEIGHT, Math.round(Number(workAreaSize?.height) * 0.92)));
  return { width, height };
}

module.exports = {
  WINDOW_MIN_WIDTH,
  WINDOW_MIN_HEIGHT,
  WINDOW_MAX_WIDTH,
  WINDOW_MAX_HEIGHT,
  windowBoundsForWorkArea,
};
