"use strict";

const WINDOW_MIN_WIDTH = 1080;
const WINDOW_MIN_HEIGHT = 680;
// 渲染进程会把整个界面按窗口逻辑尺寸连续等比缩放，所以 2K/4K 上不需要靠"把窗口开大"
// 来换可读性——开大只会换来两侧的留白。首窗按工作区的 72% / 80% 打开，并封顶。
const WINDOW_MAX_WIDTH = 2200;
const WINDOW_MAX_HEIGHT = 1400;
const WINDOW_WIDTH_RATIO = 0.72;
const WINDOW_HEIGHT_RATIO = 0.80;
// 设计基准（1920×1080）完整展开所需的窗口尺寸。小屏上 72% 连基准都放不下，
// 与其把界面缩到 0.72 倍（字号会小到读不了），不如占满一点：取 92% / 94% 兜底。
const WINDOW_DESIGN_WIDTH = 1840;
const WINDOW_DESIGN_HEIGHT = 1010;
const WINDOW_SMALL_WIDTH_RATIO = 0.92;
const WINDOW_SMALL_HEIGHT_RATIO = 0.94;

function dimension(available, ratio, design, smallRatio, minimum, maximum) {
  const size = Number(available);
  if (!Number.isFinite(size)) return minimum;
  const preferred = Math.round(size * ratio);
  const fallback = Math.min(design, Math.round(size * smallRatio));
  return Math.min(maximum, Math.max(minimum, preferred, fallback));
}

function windowBoundsForWorkArea(workAreaSize) {
  return {
    width: dimension(workAreaSize?.width, WINDOW_WIDTH_RATIO, WINDOW_DESIGN_WIDTH, WINDOW_SMALL_WIDTH_RATIO, WINDOW_MIN_WIDTH, WINDOW_MAX_WIDTH),
    height: dimension(workAreaSize?.height, WINDOW_HEIGHT_RATIO, WINDOW_DESIGN_HEIGHT, WINDOW_SMALL_HEIGHT_RATIO, WINDOW_MIN_HEIGHT, WINDOW_MAX_HEIGHT),
  };
}

module.exports = {
  WINDOW_MIN_WIDTH,
  WINDOW_MIN_HEIGHT,
  WINDOW_MAX_WIDTH,
  WINDOW_MAX_HEIGHT,
  WINDOW_WIDTH_RATIO,
  WINDOW_HEIGHT_RATIO,
  WINDOW_DESIGN_WIDTH,
  WINDOW_DESIGN_HEIGHT,
  windowBoundsForWorkArea,
};
