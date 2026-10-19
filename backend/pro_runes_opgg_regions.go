package main

// Verified 2026-10-19 against OP.GG 2026 LPL Split 3 (1422/1423)
// and LCK Rounds 1-2 (1386), including every current-season team.
// Sources: https://esports.op.gg/lpl/standings and /lck/standings.
// Exact public responses: testdata/opgg-team-regions-2026-10-19.json.
// Team IDs belong to esports.op.gg; event league and acronym are not region keys.
var proOPGGPriorityTeams = []struct{ ID, Region, Acronym, Name string }{
	{"632", "LPL", "BLG", "Bilibili Gaming"},
	{"698", "LPL", "AL", "Anyone's Legend"},
	{"415", "LPL", "TES", "Top Esports"},
	{"380", "LPL", "JDG", "JD Gaming"},
	{"379", "LPL", "WE", "Team WE"},
	{"585", "LPL", "LGD", "LGD Gaming"},
	{"645", "LPL", "TT", "ThunderTalk Gaming"},
	{"374", "LPL", "EDG", "EDward Gaming"},
	{"858", "LPL", "NIP", "Ninjas in Pyjamas"},
	{"371", "LPL", "IG", "Invictus Gaming"},
	{"699", "LPL", "WB", "Weibo Gaming"},
	{"592", "LPL", "LNG", "LNG Esports"},
	{"591", "LCK", "HLE", "Hanwha Life Esports"},
	{"385", "LCK", "T1", "T1"},
	{"416", "LCK", "GEN", "Gen.G"},
	{"384", "LCK", "KT", "KT Rolster"},
	{"1681", "LCK", "DK", "Dplus KIA"},
	{"670", "LCK", "BRO", "HANJIN BRION"},
	{"1755", "LCK", "BFX", "BNK FEARX"},
	{"590", "LCK", "KRX", "Kiwoom DRX"},
	{"669", "LCK", "NS", "Nongshim Red Force"},
	{"1791", "LCK", "DNS", "DN SOOPers"},
}

func proOPGGTeamRegion(id string) string {
	for _, team := range proOPGGPriorityTeams {
		if team.ID == id {
			return team.Region
		}
	}
	return "" // Unverified teams stay in the second group.
}
