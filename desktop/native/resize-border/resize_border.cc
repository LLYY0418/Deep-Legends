#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>
#include <windowsx.h>
#include <commctrl.h>
#include <dwmapi.h>
#include <node_api.h>
#include <cstring>

// Preserve DWM and the normal Windows sizing loop. Screen-pixel hit testing is
// shared by the root and its same-thread Chromium input child windows.
static LRESULT ResizeHit(HWND hwnd, LPARAM lp) {
  if (!IsWindow(hwnd) || IsZoomed(hwnd) || IsIconic(hwnd) ||
      !(GetWindowLongPtrW(hwnd, GWL_STYLE) & WS_THICKFRAME)) return HTNOWHERE;
  RECT visible{}, window{};
  if (FAILED(DwmGetWindowAttribute(hwnd, DWMWA_EXTENDED_FRAME_BOUNDS,
                                  &visible, sizeof(visible))) ||
      !GetWindowRect(hwnd, &window)) return HTNOWHERE;
  MONITORINFO monitor{sizeof(monitor)};
  if (GetMonitorInfoW(MonitorFromWindow(hwnd, MONITOR_DEFAULTTONEAREST), &monitor) &&
      window.left <= monitor.rcMonitor.left && window.top <= monitor.rcMonitor.top &&
      window.right >= monitor.rcMonitor.right && window.bottom >= monitor.rcMonitor.bottom)
    return HTNOWHERE;
  const int x = GET_X_LPARAM(lp), y = GET_Y_LPARAM(lp);
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
  return HTNOWHERE;
}

// DefWindowProc asks the parent to handle WM_SETCURSOR before using a child
// class cursor. Own the strip at BOTH levels; otherwise Chromium's root can
// accept the message with the last web-page hand/text cursor on inner pixels.
static bool ApplyResizeCursor(HWND root) {
  POINT point{};
  if (!GetCursorPos(&point)) return false;
  const LRESULT hit = ResizeHit(root, MAKELPARAM(point.x, point.y));
  const WORD cursor = hit == HTLEFT || hit == HTRIGHT ? 32644 :
                      hit == HTBOTTOM ? 32645 :
                      hit == HTBOTTOMLEFT ? 32643 :
                      hit == HTBOTTOMRIGHT ? 32642 : 0;
  if (!cursor) return false;
  SetCursor(LoadCursorW(nullptr, MAKEINTRESOURCEW(cursor)));
  return true;
}

static LRESULT CALLBACK ChildResizeBorder(HWND hwnd, UINT message, WPARAM wp,
                                          LPARAM lp, UINT_PTR id, DWORD_PTR root) {
  if (message == WM_NCDESTROY) RemoveWindowSubclass(hwnd, ChildResizeBorder, id);
  const HWND parent = reinterpret_cast<HWND>(root);
  if (message == WM_NCHITTEST) {
    const LRESULT hit = ResizeHit(parent, lp);
    if (hit) return hit;
  }
  // Chromium owns a child input HWND over the client-side 2px. Merely returning
  // HTTRANSPARENT does not forward its sizing gesture to the parent. Handle the
  // child's non-client press explicitly on the SAME thread's root HWND.
  if (message == WM_NCLBUTTONDOWN) {
    const LRESULT hit = ResizeHit(parent, lp);
    if (hit) {
      ReleaseCapture();
      return SendMessageW(parent, WM_NCLBUTTONDOWN, hit, lp);
    }
  }
  if (message == WM_SETCURSOR && ApplyResizeCursor(parent)) return TRUE;
  return DefSubclassProc(hwnd, message, wp, lp);
}

static BOOL CALLBACK InstallChild(HWND child, LPARAM root) {
  DWORD owner = 0;
  const DWORD thread = GetWindowThreadProcessId(child, &owner);
  if (owner == GetCurrentProcessId() && thread == GetCurrentThreadId())
    SetWindowSubclass(child, ChildResizeBorder, 314, static_cast<DWORD_PTR>(root));
  return TRUE;
}

static LRESULT CALLBACK ResizeBorder(HWND hwnd, UINT message, WPARAM wp,
                                     LPARAM lp, UINT_PTR id, DWORD_PTR) {
  if (message == WM_NCDESTROY) {
    RemoveWindowSubclass(hwnd, ResizeBorder, id);
    return DefSubclassProc(hwnd, message, wp, lp);
  }
  if (message == WM_NCLBUTTONDOWN) {
    const LRESULT hit = ResizeHit(hwnd, lp);
    if (hit) return DefWindowProcW(hwnd, message, hit, lp);
  }
  if (message == WM_SETCURSOR && ApplyResizeCursor(hwnd)) return TRUE;
  const LRESULT original = DefSubclassProc(hwnd, message, wp, lp);
  if (message == WM_PARENTNOTIFY && LOWORD(wp) == WM_CREATE)
    EnumChildWindows(hwnd, InstallChild, reinterpret_cast<LPARAM>(hwnd));
  if (message != WM_NCHITTEST) return original;
  const LRESULT hit = ResizeHit(hwnd, lp);
  return hit ? hit : original;
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
  EnumChildWindows(hwnd, InstallChild, reinterpret_cast<LPARAM>(hwnd));
  napi_value result; napi_get_boolean(env, true, &result); return result;
}
static napi_value Init(napi_env env, napi_value exports) {
  napi_property_descriptor method{"install", nullptr, Install, nullptr, nullptr, nullptr, napi_default, nullptr};
  if (napi_define_properties(env, exports, 1, &method) != napi_ok) return nullptr;
  return exports;
}
NAPI_MODULE(NODE_GYP_MODULE_NAME, Init)
