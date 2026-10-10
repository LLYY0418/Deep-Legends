package main

import "time"

// Every partial frame uses the same verified matches and algorithms as the final
// overview. The renderer decides whether the partial sample is large enough.
func deriveRiotOverviewStats(response *gameplayOverview, puuid string, names map[int64]string, region string) {
	matches := response.Matches
	response.Overall = aggregateMatches(matches, puuid, nil)
	response.RecentRanked = recentRankedSummary(matches, puuid, nil)
	response.ChampionStats = championStats(matches, puuid, names)
	response.Positions = positionStats(matches, puuid)
	response.Ability = buildGameplayAbilityProfile(matches, puuid, response.Ranks, region)
	response.RankedQueues = buildGameplayRankedQueues([]gameplayRankedQueueTab{
		{Key: "420", Label: rankedQueueLabel(seasonQueueSoloDuo), QueueIDs: []int64{seasonQueueSoloDuo}, Matches: recentRankedMatchesForQueue(matches, 420, defaultMatchCount)},
		{Key: "440", Label: rankedQueueLabel(seasonQueueFlex), QueueIDs: []int64{seasonQueueFlex}, Matches: recentRankedMatchesForQueue(matches, 440, defaultMatchCount)},
	}, puuid, response.Ranks, region)
	response.ActivityHours = activityHours(matches)
	response.RecentPlayers = recentPlayers(matches, puuid, time.Now().Add(-recentWindowDays*24*time.Hour).UnixMilli())
}
