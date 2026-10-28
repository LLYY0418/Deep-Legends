{
  "targets": [{
    "target_name": "resize_border",
    "sources": ["resize_border.cc"],
    "defines": ["NAPI_VERSION=8", "UNICODE", "_UNICODE"],
    "libraries": ["comctl32.lib", "dwmapi.lib", "user32.lib"],
    "msvs_settings": {"VCCLCompilerTool": {"AdditionalOptions": ["/std:c++17"]}}
  }]
}
