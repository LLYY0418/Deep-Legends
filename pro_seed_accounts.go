package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Reviewed Riot IDs only. PUUIDs are resolved by Riot and stay in private caches.
type proSeedAccount struct {
	TeamCode string
	Player   string
	RealName string
	Position string
	OPGGID   int
	Accounts []proSeedAccountRef
	Reviewed string
}
type proSeedAccountRef struct {
	GameName string
	TagLine  string
}

// R105: fully embedded from docs/pro-accounts-verification-2026-09-17.md.
// Keep account order stable: the index is the rename-resistant anchor identity.
var proSeedAccounts = reviewedProSeeds([]proSeedAccount{
	{TeamCode: "BLG", Player: "Bin", Accounts: []proSeedAccountRef{{"빈 스토리", "KR1"}}},
	{TeamCode: "BLG", Player: "Wenbo", Accounts: []proSeedAccountRef{{"14小孩幻想赢对线", "4453"}}},
	{TeamCode: "BLG", Player: "Flandre", Accounts: []proSeedAccountRef{{"aierlanlaozhu", "KR1"}}},
	{TeamCode: "BLG", Player: "Xun", Accounts: []proSeedAccountRef{{"我累铜泥丸", "小重o"}, {"Xun", "OOK"}}},
	{TeamCode: "BLG", Player: "knight", Accounts: []proSeedAccountRef{{"BLG 온", "KR1"}, {"쯔지 쌰 쯔지", "4165"}}},
	{TeamCode: "BLG", Player: "Viper", Accounts: []proSeedAccountRef{{"Blue", "KR33"}}},
	{TeamCode: "BLG", Player: "ON", Accounts: []proSeedAccountRef{{"키보드에음료쏟아서고장내는이도윤", "KR1"}}},
	{TeamCode: "IG", Player: "TheShy", Accounts: []proSeedAccountRef{{"The shy", "asdf"}, {"은여하", "1103"}, {"스몰더 아빠", "tsts"}, {"눈사람", "cold1"}, {"말랑한블루베리", "KR1"}}},
	{TeamCode: "IG", Player: "Wei", Accounts: []proSeedAccountRef{{"Kimman", "zxfkk"}, {"dyjkbysb", "KR1"}}},
	{TeamCode: "IG", Player: "Rookie", Accounts: []proSeedAccountRef{{"모든일은같이", "KR1"}, {"벼락식혜", "0070"}, {"EmberKnight", "KR0"}, {"asdfzxcvasdfwqer", "KR2"}}},
	{TeamCode: "IG", Player: "Assum", Accounts: []proSeedAccountRef{{"xycg", "KR1"}}},
	{TeamCode: "IG", Player: "JiaQi", Accounts: []proSeedAccountRef{{"강딱풀", "500"}, {"Whymesodiao", "jiaq3"}}},
	{TeamCode: "IG", Player: "Meiko", Accounts: []proSeedAccountRef{{"qweasdqweasdb", "1372"}}},
	{TeamCode: "T1", Player: "Doran", Accounts: []proSeedAccountRef{{"어리고싶다", "KR1"}}},
	{TeamCode: "T1", Player: "Oner", Accounts: []proSeedAccountRef{{"오 너", "111"}}},
	{TeamCode: "T1", Player: "Faker", Accounts: []proSeedAccountRef{{"Hide on bush", "KR1"}}},
	{TeamCode: "T1", Player: "Peyz", Accounts: []proSeedAccountRef{{"Peyz", "KR11"}}},
	{TeamCode: "T1", Player: "Keria", Accounts: []proSeedAccountRef{{"Ciro", "KR10"}}},
	{TeamCode: "HLE", Player: "Zeus", Accounts: []proSeedAccountRef{{"Athene", "lll"}}},
	{TeamCode: "HLE", Player: "Kanavi", Accounts: []proSeedAccountRef{{"vinaka", "KR1"}, {"PoAtan", "ASP"}}},
	{TeamCode: "HLE", Player: "Zeka", Accounts: []proSeedAccountRef{{"suis", "kr7"}, {"Kiruru", "kr7"}}},
	{TeamCode: "HLE", Player: "Gumayusi", Accounts: []proSeedAccountRef{{"HLE Gumayusi", "0298"}, {"thsorre", "2830"}}},
	{TeamCode: "HLE", Player: "Delight", Accounts: []proSeedAccountRef{{"플레이리스트겨울", "KR1"}}},
	{TeamCode: "GEN", Player: "Kiin", Accounts: []proSeedAccountRef{{"kiin", "KR1"}}},
	{TeamCode: "GEN", Player: "Canyon", Accounts: []proSeedAccountRef{{"JUGKlNG", "kr"}}},
	{TeamCode: "GEN", Player: "Chovy", Accounts: []proSeedAccountRef{{"허거덩", "0303"}}},
	{TeamCode: "GEN", Player: "Ruler", Accounts: []proSeedAccountRef{{"강 철", "샤 넬"}}},
	{TeamCode: "GEN", Player: "Duro", Accounts: []proSeedAccountRef{{"Duro", "Gen"}}},
	{TeamCode: "DK", Player: "Siwoo", Accounts: []proSeedAccountRef{{"TOPKING", "asd"}, {"아무것도 몰라요", "12345"}}},
	{TeamCode: "DK", Player: "Lucid", Accounts: []proSeedAccountRef{{"DK Lucid", "KR1"}, {"너무재미있겠다", "KR1"}}},
	{TeamCode: "DK", Player: "ShowMaker", Accounts: []proSeedAccountRef{{"DK ShowMaker", "KR1"}, {"MIDKING", "asd"}}},
	{TeamCode: "DK", Player: "Smash", Accounts: []proSeedAccountRef{{"DK Smash", "KR7"}, {"Smash", "KR2"}, {"사옥 지박령", "KR4"}}},
	{TeamCode: "DK", Player: "Career", Accounts: []proSeedAccountRef{{"팽도리", "1015"}, {"인간 병기", "0829"}}},
})

func reviewedProSeeds(seeds []proSeedAccount) []proSeedAccount {
	for i := range seeds {
		seed := &seeds[i]
		for _, team := range proRoster {
			if team.Code != seed.TeamCode {
				continue
			}
			for _, player := range team.Players {
				if player.Name == seed.Player {
					seed.OPGGID, seed.RealName, seed.Position = team.OPGGID, player.Names[0], player.Position
					seed.Reviewed = "2026-09-17"
				}
			}
		}
		if seed.RealName == "" {
			panic("seed missing reviewed roster metadata")
		}
	}
	return seeds
}
func proSeedAnchor(seed proSeedAccount, index int) string {
	return "proseed:v1:" + seed.TeamCode + "/" + strings.ToLower(seed.Player) + "/" + strconv.Itoa(index)
}
func (p *riotProvider) resolveProSeed(ctx context.Context, seed proSeedAccount, index int) (riotAccount, error) {
	if index < 0 || index >= len(seed.Accounts) {
		return riotAccount{}, errRiotNotFound
	}
	ref := seed.Accounts[index]
	var anchor struct {
		PUUID string `json:"puuid"`
	}
	var initial riotAccount
	ctx = withRiotSingleWaitLimit(ctx, 100*time.Millisecond)
	err := p.cachedPublicIdentity(ctx, proSeedAnchor(seed, index), 30*24*time.Hour, &anchor, func(ctx context.Context) error {
		account, err := p.accountByRiotID(ctx, ref.GameName, ref.TagLine)
		if err != nil {
			return err
		}
		anchor.PUUID, initial = account.PUUID, account
		return nil
	})
	if err != nil {
		return riotAccount{}, err
	}
	if initial.PUUID != "" {
		return initial, nil
	}
	if anchor.PUUID == "" {
		return riotAccount{}, errRiotNotFound
	}
	return p.fetchAccountByPUUID(ctx, anchor.PUUID)
}

const proSeedPerAccountBudget = 4 * time.Second

// Legacy batch callers have a fixed ceiling; the scheduled worker narrows this
// further to one four-second account per minute. Account count cannot expand it.
func proSeedContext(ctx context.Context, _ int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 20*time.Second)
}
func (a *app) loadProSeeds(ctx context.Context, previous []opggProTeam, directory ...[]opggProTeam) []opggProTeam {
	return a.loadProSeedAccounts(ctx, previous, proSeedAccounts, directory...)
}

// Each account has an independent timeout and its own last successful snapshot.
func (a *app) loadProSeedAccounts(ctx context.Context, previous []opggProTeam, seeds []proSeedAccount, directory ...[]opggProTeam) []opggProTeam {
	ctx, cost := newProRefreshCost(ctx)
	defer func() { a.recordProRefreshCost("pro_seed_cost", cost) }()
	known := proDirectoryAccounts(directory...)
	var rows []opggProTeam
	for _, seed := range seeds {
		seedCtx, cancelSeed := proSeedContext(ctx, len(seed.Accounts))
		var accounts []opggProAccount
		for index, ref := range seed.Accounts {
			key := proSeedAnchor(seed, index)
			cost.total++
			var old opggProAccount
			for _, team := range previous {
				if team.ID != seed.OPGGID {
					continue
				}
				for _, member := range team.Members {
					if member.Nickname != seed.Player {
						continue
					}
					for _, candidate := range member.Summoners {
						// The empty-key branch migrates the original single Rookie snapshot.
						if candidate.Source == "seed" && (candidate.SeedKey == key || candidate.SeedKey == "" && seed.Player == "Rookie" && index == 0) {
							old = candidate
						}
					}
				}
			}
			row := old
			if row.GameName == "" {
				row = opggProAccount{SeedKey: key, GameName: ref.GameName, TagLine: ref.TagLine, Region: "kr", Source: "seed"}
			}
			if fromDirectory, ok := known[proLadderAccountKey(ref.GameName, ref.TagLine)]; ok {
				row = fromDirectory
				row.SeedKey, row.Source = key, "seed"
				a.rememberProSeedAnchor(ctx, key, row.PUUID)
				cost.refreshed++
			} else if a.riot != nil && riotKeyConfigured() && proSeedSelected(ctx, key) {
				accountCtx, cancel := context.WithTimeout(seedCtx, proSeedPerAccountBudget)
				accountCtx = withRiotSingleWaitLimit(withRiotBackground(accountCtx), 100*time.Millisecond)
				account, err := a.riot.resolveProSeed(accountCtx, seed, index)
				if err == nil {
					cost.refreshed++
					rank, rankErr := a.riot.proSeedRank(accountCtx, account.PUUID)
					if rankErr == nil {
						row = opggProAccount{SeedKey: key, PUUID: account.PUUID, GameName: account.GameName, TagLine: account.TagLine, Region: "kr", Source: "seed", Rank: rank, UpdatedAt: time.Now().UTC().Format(time.RFC3339), LastMatchAt: old.LastMatchAt, LastMatchAtKnown: old.LastMatchAtKnown}
					} else if old.GameName == "" {
						row = opggProAccount{SeedKey: key, PUUID: account.PUUID, GameName: account.GameName, TagLine: account.TagLine, Region: "kr", Source: "seed"}
					}
					// Rank failures/unranked backoff never suppress activity lookups.
					at, known, matchErr := a.riot.lastMatchStart(accountCtx, account.PUUID)
					if matchErr == nil {
						setProLastMatch(&row, at, known)
					}
					if account.GameName != ref.GameName || account.TagLine != ref.TagLine {
						a.recordDiagnostic(map[string]any{"event": "pro_seed_renamed", "changed": true})
					}
				}
				cancel()
			}
			if row.GameName != "" {
				accounts = append(accounts, row)
			}
		}
		cancelSeed()
		if len(accounts) > 0 {
			rows = append(rows, opggProTeam{ID: seed.OPGGID, ShortName: seed.TeamCode, Members: []opggProMember{{TeamID: seed.OPGGID, Nickname: seed.Player, RealName: seed.RealName, Position: seed.Position, Authority: "PROGAMER", Summoners: accounts}}})
		}
	}
	return rows
}
func withProSeed(base []opggProTeam, seeds []opggProTeam) []opggProTeam {
	return append(cloneProTeams(base), cloneProTeams(seeds)...)
}

func proSeedRankTTL(data []byte) time.Duration {
	if string(data) == "null" {
		return 72 * time.Hour
	}
	return 24 * time.Hour
}

// Seed freshness is independent of the overview's three-minute ranks cache.
func (p *riotProvider) proSeedRank(ctx context.Context, puuid string) (json.RawMessage, error) {
	var rank json.RawMessage
	err := p.cachedPublicIdentityTTL(ctx, "proseed-rank:v1:"+puuid, 24*time.Hour, proSeedRankTTL, &rank, func(ctx context.Context) error {
		var entries []riotLeagueEntry
		if err := p.get(ctx, riotPlatformHost, "/lol/league/v4/entries/by-puuid/"+url.PathEscape(puuid), nil, &entries); err != nil {
			return err
		}
		rank = proSeedRankFromEntries(entries)
		return nil
	})
	return rank, err
}

func proSeedRankFromEntries(entries []riotLeagueEntry) json.RawMessage {
	var rank json.RawMessage
	for _, entry := range entries {
		if entry.QueueType != "RANKED_SOLO_5x5" {
			continue
		}
		division := map[string]int{"I": 1, "II": 2, "III": 3, "IV": 4}[entry.Rank]
		if entry.Tier == "MASTER" || entry.Tier == "GRANDMASTER" || entry.Tier == "CHALLENGER" {
			division = 1
		}
		rank, _ = json.Marshal(struct {
			Tier     string `json:"tier"`
			Division int    `json:"division"`
			LP       int    `json:"lp"`
		}{entry.Tier, division, entry.LeaguePoints})
		break
	}
	return rank
}

// A successful earlier overview lookup can end a confirmed-unranked backoff.
// It cannot extend an already ranked entry on every three-minute overview read.
func (p *riotProvider) observeProSeedRank(puuid string, entries []riotLeagueEntry) {
	c := p.identityDisk
	rank := proSeedRankFromEntries(entries)
	if c == nil || len(rank) == 0 {
		return
	}
	key := riotIdentityKey("proseed-rank:v1:" + puuid)
	c.mu.Lock()
	old, ok := c.entries[key]
	c.mu.Unlock()
	if !ok {
		old, _ = c.readDisk(key)
	}
	if string(old.Data) != "null" {
		return
	}
	now := c.cacheNow()
	hash := sha256.Sum256(rank)
	entry := championCacheEnvelope{Schema: championCacheSchema, Key: key, FetchedAt: now, ExpiresAt: now.Add(24 * time.Hour), StaleUntil: now.Add(24 * time.Hour), Hash: hex.EncodeToString(hash[:]), Data: rank}
	c.mu.Lock()
	c.storeMemoryLocked(key, entry)
	c.mu.Unlock()
	_ = c.writeDisk(entry)
}
