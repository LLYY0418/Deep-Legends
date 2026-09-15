package main

// Only the request owning the shared overview flight publishes progress. Other
// callers join that flight and receive its final snapshot without extra reads.
type riotOverviewProgressKey struct{}
