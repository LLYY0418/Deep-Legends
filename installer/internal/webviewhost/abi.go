package webviewhost

// Zero-based slots in the base COM interfaces. abi_test.go independently derives
// their layout from the complete dependency declarations, including IUnknown.
const (
	unknownQueryInterface = 0
	unknownRelease        = 2

	controllerGetVisible     = 3
	controllerPutVisible     = 4
	controllerPutBounds      = 6
	controllerClose          = 24
	controllerGetCore        = 25
	controller2PutBackground = 27

	coreGetSettings      = 3
	coreNavigateToString = 6

	settingsPutScriptEnabled        = 4
	settingsPutWebMessageEnabled    = 6
	settingsPutDefaultScriptDialogs = 8
	settingsPutStatusBar            = 10
	settingsPutDevTools             = 12
	settingsPutContextMenus         = 14
	settingsPutZoomControl          = 18
	settingsPutBuiltInErrorPage     = 20

	navigationGetSuccess = 3
	navigationGetError   = 4
)
