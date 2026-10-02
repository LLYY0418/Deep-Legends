package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"time"
)

// Pro opening offset is provisional until raw field-shape diagnostics from a
// real client verify the feed's first frame against the in-game clock.
// proRuneIndexGame.Start is the original window first-frame timestamp.
const proStarterOffset = 60 * time.Second

type proDetailsObservationKey struct{}
type proBackgroundRequestKey struct{}

func withProBackground(ctx context.Context) context.Context {
	return context.WithValue(ctx, proBackgroundRequestKey{}, true)
}

type proDetailsObservation struct {
	game         proRuneIndexGame
	kind         string
	participants []int
}

func proStarterItems(details proDetails, participantID int, start time.Time) ([]int64, float64) {
	items, offset := proOpeningInventory(details, participantID, start)
	return items[:min(len(items), 6)], offset
}

func proOpeningInventory(details proDetails, participantID int, start time.Time) ([]int64, float64) {
	for _, frame := range details.Frames {
		for _, participant := range frame.Participants {
			if participant.ID != participantID {
				continue
			}
			items := []int64{}
			for _, id := range participant.Items {
				if id > 0 && !isRuneTrinket(id) {
					items = append(items, id)
				}
			}
			if len(items) > 0 {
				return items, frame.Timestamp.Sub(start).Seconds()
			}
		}
	}
	return []int64{}, 0
}

func validProStartDetails(details proDetails, game proRuneIndexGame) bool {
	if len(details.Frames) == 0 {
		return false
	}
	allowed := map[int]bool{}
	for _, team := range []proTeamMetadata{game.Metadata.Blue, game.Metadata.Red} {
		for _, participant := range team.Participants {
			allowed[participant.ParticipantID] = true
		}
	}
	for _, frame := range details.Frames {
		if len(frame.Participants) == 0 || frame.Timestamp.IsZero() || frame.Timestamp.Before(game.Start) {
			return false
		}
		seen := map[int]bool{}
		for _, participant := range frame.Participants {
			if !allowed[participant.ID] || seen[participant.ID] {
				return false
			}
			seen[participant.ID] = true
		}
	}
	return true
}

func (p *proRuneProvider) openingDetails(ctx context.Context, game proRuneIndexGame, participants []int) (proDetails, error) {
	var result proDetails
	err := p.gameResource(ctx, "details-start-"+game.ID, func() error {
		p.mu.Lock()
		cached, found := p.startDetails[game.ID]
		failedAt := p.startFailures[game.ID]
		p.mu.Unlock()
		if found {
			result = cached
			return nil
		}
		if !failedAt.IsZero() && time.Since(failedAt) < 90*time.Second {
			return errors.New("opening retry cooldown")
		}
		key := "details-start-" + game.ID
		if !p.readDisk(key, &result) || !validProStartDetails(result, game) {
			observation := proDetailsObservation{game, "start", participants}
			if err := p.fetch(context.WithValue(withProBackground(ctx), proDetailsObservationKey{}, observation), true, "details/"+game.ID, url.Values{"startingTime": {proStartingTime(game.Start.Add(proStarterOffset))}}, &result); err != nil {
				p.mu.Lock()
				p.startFailures[game.ID] = time.Now()
				p.mu.Unlock()
				return err
			}
		}
		if !validProStartDetails(result, game) {
			p.mu.Lock()
			p.startFailures[game.ID] = time.Now()
			p.mu.Unlock()
			return errors.New("invalid opening frame or participants")
		}
		p.writeDisk(key, result)
		p.mu.Lock()
		p.startDetails[game.ID] = result
		p.mu.Unlock()
		return nil
	})
	return result, err
}

func (p *proRuneProvider) starterRows(ctx context.Context, championID int, position string, wanted map[string]int64) []runeStarterRow {
	meta, err := p.provider.championMetadataByID(ctx, championID)
	if err != nil {
		return []runeStarterRow{}
	}
	p.mu.Lock()
	snapshot := p.snapshot
	p.mu.Unlock()
	rows := proNewestRows(p.rows(snapshot, time.Now()), meta.Key, position)
	selected := []proRuneRow{}
	for _, row := range rows {
		if at, ok := wanted[proRuneRowKey(row)]; ok && at == row.Game.Start.UnixMilli() {
			selected = append(selected, row)
		}
		if len(selected) == runeStarterRowLimit {
			break
		}
	}
	result := make([]runeStarterRow, len(selected))
	runeStarterParallel(ctx, selected, 2, func(index int, row proRuneRow) {
		details, err := p.openingDetails(ctx, row.Game, []int{row.Participant.ParticipantID})
		recordRuneStarterBatchError(ctx, err)
		if err == nil {
			items, _ := proStarterItems(details, row.Participant.ParticipantID, row.Game.Start)
			result[index] = runeStarterRow{proRuneRowKey(row), row.Game.Start.UnixMilli(), items}
		}
		if p.provider.diag != nil {
			outcome, reason := "success", ""
			if err != nil {
				outcome, reason = "failed", proRuneErrorReason(err)
			}
			p.provider.diag(map[string]any{"event": "rune_starter_items", "source": "pro", "outcome": outcome, "reason": reason, "item_count": len(result[index].StarterItemIDs)})
		}
	})
	return nonemptyStarterRows(result)
}

// The raw JSON is inspected only here. Unknown keys could contain identity
// strings, so only known protocol keys survive. Values never enter the log.
func (p *proRuneProvider) recordDetailsShape(raw []byte, observation proDetailsObservation) {
	if p.provider.diag == nil || (observation.kind != "start" && observation.kind != "end") {
		return
	}
	var response struct {
		Frames []struct {
			Timestamp    time.Time         `json:"rfc460Timestamp"`
			Participants []json.RawMessage `json:"participants"`
		} `json:"frames"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return
	}
	p.mu.Lock()
	if p.detailsShapes == nil {
		p.detailsShapes = map[string]bool{}
	}
	key := observation.game.ID + "|" + observation.kind
	if p.detailsShapes[key] {
		p.mu.Unlock()
		return
	}
	p.detailsShapes[key] = true
	p.mu.Unlock()
	keys := map[string]bool{}
	for _, frame := range response.Frames {
		for _, participant := range frame.Participants {
			var fields map[string]json.RawMessage
			if json.Unmarshal(participant, &fields) == nil {
				for key := range fields {
					keys[proDetailsParticipantKeys(key)] = true
				}
			}
		}
	}
	names := []string{}
	for key := range keys {
		names = append(names, key)
	}
	sort.Strings(names)
	event := map[string]any{"event": "pro_details_shape", "kind": observation.kind, "frames_length": len(response.Frames), "participant_keys": names}
	if len(response.Frames) > 0 {
		first, last := response.Frames[0].Timestamp, response.Frames[len(response.Frames)-1].Timestamp
		if !first.IsZero() {
			event["first_offset_seconds"] = first.Sub(observation.game.Start).Seconds()
		}
		if !last.IsZero() {
			event["last_offset_seconds"] = last.Sub(observation.game.Start).Seconds()
		}
	}
	if observation.kind == "start" {
		var details proDetails
		if json.Unmarshal(raw, &details) == nil {
			selected := []map[string]any{}
			for _, id := range observation.participants {
				items, offset := proOpeningInventory(details, id, observation.game.Start)
				selected = append(selected, map[string]any{"offset_seconds": offset, "item_count": len(items)})
			}
			event["request_offset_seconds"] = proStarterOffset.Seconds()
			event["selected_frames"] = selected
		}
	}
	p.provider.diag(event)
}
