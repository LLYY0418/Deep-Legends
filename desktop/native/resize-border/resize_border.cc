#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>
#include <windowsx.h>
#include <commctrl.h>
#include <dwmapi.h>
#include <node_api.h>
#include <cstring>

// Keep the Windows frame, DWM appearance, caption buttons, and default sizing
// loop. Only the two client-side pixels adjoining its invisible resize frame
// are reclassified. The renderer/scrollbar cannot consume this native strip.
static LRESULT CALLBACK ResizeBorder(HWND hwnd, UINT message, WPARAM wp,
                                     LPARAM lp, UINT_PTR id, DWORD_PTR) {
  if (message == WM_NCDESTROY) {
    RemoveWindowSubclass(hwnd, ResizeBorder, id);
    return DefSubclassProc(hwnd, message, wp, lp);
  }
  const LRESULT original = DefSubclassProc(hwnd, message, wp, lp);
  if (message != WM_NCHITTEST || IsZoomed(hwnd) || IsIconic(hwnd) ||
      !(GetWindowLongPtrW(hwnd, GWL_STYLE) & WS_THICKFRAME)) return original;
  RECT visible{}, window{};
  if (FAILED(DwmGetWindowAttribute(hwnd, DWMWA_EXTENDED_FRAME_BOUNDS,
                                  &visible, sizeof(visible))) ||
      !GetWindowRect(hwnd, &window)) return original;
  // A borderless full-screen window must not acquire resize handles.
  MONITORINFO monitor{sizeof(monitor)};
  if (GetMonitorInfoW(MonitorFromWindow(hwnd, MONITOR_DEFAULTTONEAREST), &monitor) &&
      window.left <= monitor.rcMonitor.left && window.top <= monitor.rcMonitor.top &&
      window.right >= monitor.rcMonitor.right && window.bottom >= monitor.rcMonitor.bottom)
    return original;
  const int x = GET_X_LPARAM(lp), y = GET_Y_LPARAM(lp);
  // Screen pixels, not DIP or CSS zoom. Retain at least this physical width at
  // every monitor DPI, using the DWM visible edge as the measurement origin.
  const bool left = x >= visible.left - 6 && x <= visible.left + 2;
  const bool right = x >= visible.right - 3 && x <= visible.right + 5;
  const bool bottom = y >= visible.bottom - 3 && y <= visible.bottom + 5;
  if (bottom && left) return HTBOTTOMLEFT;
  if (bottom && right) return HTBOTTOMRIGHT;
  if (bottom && x >= visible.left && x < visible.right) return HTBOTTOM;
  if (y >= visible.top + 8 && y < visible.bottom) {
    if (left) return HTLEFT;
    if (right) return HTRIGHT;
  }
  return original;
}

static napi_value Install(napi_env env, napi_callback_info info) {
  size_t argc = 1, size = 0;
  napi_value argv[1]; void* buffer = nullptr;
  bool is_buffer = false;
  if (napi_get_cb_info(env, info, &argc, argv, nullptr, nullptr) != napi_ok || argc != 1 ||
      napi_is_buffer(env, argv[0], &is_buffer) != napi_ok || !is_buffer ||
      napi_get_buffer_info(env, argv[0], &buffer, &size) != napi_ok || size != sizeof(HWND)) {
    napi_throw_type_error(env, nullptr, "Expected this BrowserWindow's native HWND buffer");
    return nullptr;
  }
  HWND hwnd = nullptr; std::memcpy(&hwnd, buffer, sizeof(hwnd));
  DWORD owner = 0;
  const DWORD thread = GetWindowThreadProcessId(hwnd, &owner);
  if (!IsWindow(hwnd) || owner != GetCurrentProcessId() || thread != GetCurrentThreadId() ||
      !SetWindowSubclass(hwnd, ResizeBorder, 314, 0)) {
    napi_throw_error(env, nullptr, "Cannot install resize border on this UI-thread window");
    return nullptr;
  }
  napi_value result; napi_get_boolean(env, true, &result); return result;
}
static napi_value Init(napi_env env, napi_value exports) {
  napi_property_descriptor method{"install", nullptr, Install, nullptr, nullptr, nullptr, napi_default, nullptr};
  if (napi_define_properties(env, exports, 1, &method) != napi_ok) return nullptr;
  return exports;
}
NAPI_MODULE(NODE_GYP_MODULE_NAME, Init)
