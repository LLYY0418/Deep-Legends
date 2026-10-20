package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const playerTagsLimit = 5000
const playerTagsBytes = 8 << 20

type playerTag struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Group  string `json:"group"`
	Preset bool   `json:"preset"`
	Hidden bool   `json:"hidden"`
}
type playerTagName struct {
	GameName      string `json:"gameName"`
	TagLine       string `json:"tagLine"`
	ProfileIconID int64  `json:"profileIconId"`
	ServerID      string `json:"serverId"`
	Region        string `json:"region"`
}
type playerTagMet struct {
	At           int64  `json:"at"`
	QueueID      int64  `json:"queueId"`
	ChampionID   int64  `json:"championId"`
	ChampionName string `json:"championName"`
	GameID       int64  `json:"gameId"`
}
type playerTagRecord struct {
	Tags         []string      `json:"tags"`
	Note         string        `json:"note"`
	CreatedAt    int64         `json:"createdAt"`
	UpdatedAt    int64         `json:"updatedAt"`
	LastMet      playerTagMet  `json:"lastMet"`
	NameSnapshot playerTagName `json:"nameSnapshot"`
}
type playerTagsFile struct {
	Schema  int                        `json:"schema"`
	Version int64                      `json:"version"`
	Catalog []playerTag                `json:"catalog"`
	Players map[string]playerTagRecord `json:"players"`
}

func defaultPlayerTags() []playerTag {
	names := []string{"消极比赛", "言语辱骂", "挂机 / 退出", "故意送人头", "仇恨言论", "作弊", "不当名称", "大腿", "好队友", "会沟通", "稳"}
	result := make([]playerTag, 0, len(names))
	for i, n := range names {
		group := "neg"
		if i >= 7 {
			group = "pos"
		}
		result = append(result, playerTag{ID: fmt.Sprintf("preset-%d", i+1), Name: n, Group: group, Preset: true})
	}
	return result
}
func validCustomTagName(s string) bool {
	n := 0
	for _, c := range s {
		if unicode.IsControl(c) {
			return false
		}
		if c > 127 {
			n += 2
		} else {
			n++
		}
	}
	return strings.TrimSpace(s) != "" && n <= 12
}
func validatePlayerTags(v playerTagsFile) error {
	if v.Schema != 1 || len(v.Players) > playerTagsLimit || len(v.Catalog) > 23 {
		return errors.New("玩家标签数量超出限制")
	}
	defaults := defaultPlayerTags()
	seen := map[string]bool{}
	custom := 0
	for _, t := range v.Catalog {
		if t.ID == "" || len(t.ID) > 64 || seen[t.ID] || !(t.Group == "neg" || t.Group == "pos" || t.Group == "mine") {
			return errors.New("标签目录格式无效")
		}
		seen[t.ID] = true
		if t.Preset {
			found := false
			for _, d := range defaults {
				if t.ID == d.ID && t.Name == d.Name && t.Group == d.Group {
					found = true
				}
			}
			if !found {
				return errors.New("预设标签不可修改")
			}
		} else {
			custom++
			if !validCustomTagName(t.Name) {
				return errors.New("标签最多6个汉字或12个字符")
			}
		}
	}
	if custom > 12 {
		return errors.New("自定义标签最多12个")
	}
	for _, d := range defaults {
		found := false
		for _, t := range v.Catalog {
			if t.ID == d.ID && t.Preset && t.Name == d.Name && t.Group == d.Group {
				found = true
			}
		}
		if !found {
			return errors.New("预设标签不可删除或修改")
		}
	}
	for id, r := range v.Players {
		if !validPlayerReference(id) || utf8.RuneCountInString(r.Note) > 40 || len(r.Tags) > 23 || r.CreatedAt < 0 || r.UpdatedAt < 0 || len(r.NameSnapshot.GameName) > 256 || len(r.NameSnapshot.TagLine) > 128 || len(r.NameSnapshot.ServerID) > 64 || len(r.NameSnapshot.Region) > 32 || len(r.LastMet.ChampionName) > 128 {
			return errors.New("玩家标记格式无效")
		}
		tags := map[string]bool{}
		for _, t := range r.Tags {
			if !seen[t] || tags[t] {
				return errors.New("未知或重复标签")
			}
			tags[t] = true
		}
	}
	return nil
}
func (s *localStore) readPlayerTagsLocked() (playerTagsFile, error) {
	v := playerTagsFile{Schema: 1, Catalog: defaultPlayerTags(), Players: map[string]playerTagRecord{}}
	raw, err := readLocalStoreFile(s, "player-tags.json")
	if errors.Is(err, os.ErrNotExist) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	if len(raw) > playerTagsBytes || json.Unmarshal(raw, &v) != nil {
		return v, errors.New("玩家标签文件无效")
	}
	if v.Players == nil {
		v.Players = map[string]playerTagRecord{}
	}
	return v, validatePlayerTags(v)
}
func (s *localStore) writePlayerTagsLocked(v *playerTagsFile) error {
	if err := validatePlayerTags(*v); err != nil {
		return err
	}
	v.Version++
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(raw) > playerTagsBytes {
		return errors.New("玩家标签文件过大")
	}
	return writeLocalStoreFile(s, "player-tags.json", raw)
}
func (a *app) visibleTagReference(public string) (gameplayReference, bool) {
	if !strings.HasPrefix(public, "player_") {
		return gameplayReference{}, false
	}
	ref, ok := a.resolveGameplayReferenceDetails(public)
	return ref, ok && !ref.TagBlocked && (ref.GameName != "" || ref.DisplayName != "")
}
func tagNameSnapshot(ref gameplayReference) playerTagName {
	return playerTagName{GameName: firstNonEmpty(ref.GameName, ref.DisplayName), TagLine: ref.TagLine, ProfileIconID: ref.ProfileIconID, ServerID: ref.ServerID, Region: ref.Region}
}
func (a *app) handlePlayerTags(w http.ResponseWriter, r *http.Request) {
	var diagnostic map[string]any
	defer func() {
		if diagnostic != nil {
			a.recordDiagnostic(diagnostic)
		}
	}()
	if a.storage == nil {
		http.Error(w, "玩家标签暂不可用", 503)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	var req struct {
		Op         string          `json:"op"`
		PlayerRef  string          `json:"playerRef"`
		PlayerRefs []string        `json:"playerRefs"`
		Tags       []string        `json:"tags"`
		Note       string          `json:"note"`
		Catalog    []playerTag     `json:"catalog"`
		Import     *playerTagsFile `json:"import"`
		Commit     bool            `json:"commit"`
	}
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, playerTagsBytes)
		defer r.Body.Close()
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var extra any
		if d.Decode(&req) != nil || d.Decode(&extra) != io.EOF {
			http.Error(w, "玩家标签请求无效", 400)
			return
		}
	}
	// Resolve references before taking the store lock: reference registration has its own mutex.
	refs := map[string]gameplayReference{}
	if req.Op == "query" {
		if len(req.PlayerRefs) > 200 {
			http.Error(w, "查询过多", 400)
			return
		}
		for _, p := range req.PlayerRefs {
			if ref, ok := a.visibleTagReference(p); ok {
				refs[p] = ref
			}
		}
	}
	var ref gameplayReference
	if req.Op == "update" {
		var ok bool
		ref, ok = a.visibleTagReference(req.PlayerRef)
		if !ok {
			http.Error(w, "该玩家身份尚未公开或引用已过期", 404)
			return
		}
	}
	s := a.storage
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.readPlayerTagsLocked()
	if err != nil {
		http.Error(w, "玩家标签读取失败", 500)
		return
	}
	fail := func(e error) { http.Error(w, e.Error(), 400) }
	switch {
	case r.Method == "GET" && r.URL.Query().Get("export") == "1":
		w.Header().Set("Content-Disposition", `attachment; filename="player-tags.json"`)
		diagnostic = map[string]any{"event": "player_tags_export", "players": len(v.Players), "tags": len(v.Catalog)}
		respondJSON(w, v)
		return
	case r.Method == "GET":
		type row struct {
			PlayerRef string `json:"playerRef"`
			playerTagRecord
		}
		rows := make([]row, 0, len(v.Players))
		for id, record := range v.Players {
			n := record.NameSnapshot
			public := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: id, GameName: n.GameName, TagLine: n.TagLine, ProfileIconID: n.ProfileIconID, ServerID: n.ServerID, Region: n.Region})
			rows = append(rows, row{public, record})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].UpdatedAt > rows[j].UpdatedAt })
		respondJSON(w, map[string]any{"schema": 1, "version": v.Version, "catalog": v.Catalog, "players": rows})
		return
	case req.Op == "query":
		result := map[string]playerTagRecord{}
		for p, ref := range refs {
			if value, ok := v.Players[ref.PlayerRef]; ok {
				result[p] = value
			}
		}
		respondJSON(w, map[string]any{"version": v.Version, "catalog": v.Catalog, "records": result})
		return
	case req.Op == "update":
		old, exists := v.Players[ref.PlayerRef]
		now := time.Now().UnixMilli()
		if len(req.Tags) == 0 && req.Note == "" {
			delete(v.Players, ref.PlayerRef)
		} else {
			if !exists && len(v.Players) >= playerTagsLimit {
				fail(errors.New("最多标记5000位玩家"))
				return
			}
			if old.CreatedAt == 0 {
				old.CreatedAt = now
			}
			old.UpdatedAt = now
			old.Tags = req.Tags
			old.Note = req.Note
			old.NameSnapshot = tagNameSnapshot(ref)
			v.Players[ref.PlayerRef] = old
		}
	case req.Op == "catalog":
		// Removing custom entries also removes their use from every record.
		keep := map[string]bool{}
		for _, t := range req.Catalog {
			keep[t.ID] = true
		}
		for id, row := range v.Players {
			tags := []string{}
			for _, t := range row.Tags {
				if keep[t] {
					tags = append(tags, t)
				}
			}
			row.Tags = tags
			if len(tags) == 0 && row.Note == "" {
				delete(v.Players, id)
			} else {
				v.Players[id] = row
			}
		}
		v.Catalog = req.Catalog
	case req.Op == "import":
		if req.Import == nil {
			fail(errors.New("缺少导入文件"))
			return
		}
		if err := validatePlayerTags(*req.Import); err != nil {
			fail(err)
			return
		}
		added, updated := 0, 0
		catalog := map[string]playerTag{}
		for _, t := range v.Catalog {
			catalog[t.ID] = t
		}
		mapping := map[string]string{}
		for _, t := range req.Import.Catalog {
			originalID := t.ID
			if old, ok := catalog[t.ID]; ok && old.Name != t.Name {
				t.ID = fmt.Sprintf("import-%d-%d", time.Now().UnixNano(), len(mapping))
			}
			mappingID := t.ID
			for _, own := range v.Catalog {
				if own.Name == t.Name && own.Group == t.Group {
					mappingID = own.ID
					break
				}
			}
			mapping[originalID] = mappingID
			if _, ok := catalog[mappingID]; !ok {
				t.ID = mappingID
				v.Catalog = append(v.Catalog, t)
				catalog[t.ID] = t
			}
		}
		for id, incoming := range req.Import.Players {
			old, exists := v.Players[id]
			if exists {
				updated++
			} else {
				added++
			}
			tags := map[string]bool{}
			for _, t := range old.Tags {
				tags[t] = true
			}
			for _, t := range incoming.Tags {
				tags[mapping[t]] = true
			}
			if old.UpdatedAt > incoming.UpdatedAt {
				incoming.Note = old.Note
				incoming.UpdatedAt = old.UpdatedAt
				incoming.NameSnapshot = old.NameSnapshot
			}
			if old.CreatedAt > 0 && (incoming.CreatedAt == 0 || old.CreatedAt < incoming.CreatedAt) {
				incoming.CreatedAt = old.CreatedAt
			}
			if old.LastMet.At > incoming.LastMet.At {
				incoming.LastMet = old.LastMet
			}
			incoming.Tags = []string{}
			for _, t := range v.Catalog {
				if tags[t.ID] {
					incoming.Tags = append(incoming.Tags, t.ID)
				}
			}
			if len(incoming.Tags) > 0 || incoming.Note != "" {
				v.Players[id] = incoming
			}
		}
		if err := validatePlayerTags(v); err != nil {
			fail(err)
			return
		}
		if !req.Commit {
			respondJSON(w, map[string]int{"added": added, "updated": updated})
			return
		}
	default:
		fail(errors.New("未知玩家标签操作"))
		return
	}
	if err := s.writePlayerTagsLocked(&v); err != nil {
		fail(err)
		return
	}
	diagnostic = map[string]any{"event": "player_tags_saved", "players": len(v.Players), "tags": len(v.Catalog)}
	respondJSON(w, map[string]any{"version": v.Version, "saved": true})
}

// Called after live enrichment; no HTTP lookup and at most one disk write per game.
func (a *app) rememberTaggedLivePlayers(live gameplayLiveResponse) {
	if a.storage == nil || live.GameID <= 0 {
		return
	}
	s := a.storage
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.readPlayerTagsLocked()
	if err != nil || len(v.Players) == 0 {
		return
	}
	changed := false
	for _, p := range live.Players {
		if p.Hidden || p.IdentityUnresolved {
			continue
		}
		ref, ok := a.visibleTagReference(p.PlayerRef)
		if !ok {
			continue
		}
		row, exists := v.Players[ref.PlayerRef]
		if !exists || row.LastMet.GameID == live.GameID {
			continue
		}
		row.LastMet = playerTagMet{At: time.Now().UnixMilli(), QueueID: int64(live.QueueID), ChampionID: p.ChampionID, ChampionName: p.ChampionName, GameID: live.GameID}
		row.NameSnapshot = tagNameSnapshot(ref)
		v.Players[ref.PlayerRef] = row
		changed = true
	}
	if changed {
		_ = s.writePlayerTagsLocked(&v)
	}
}
