package main

// Caller holds a.mu. Identity publication and business endpoints share this
// session predicate; collection readiness is a separate data state.
func (a *app) clientSessionConnectedLocked() bool {
	return (a.connected || a.lcu != nil && a.identityReady) && a.shutdownClient == nil
}
