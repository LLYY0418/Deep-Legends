package main

import (
	_ "embed"
	"encoding/json"
	"strconv"
)

// Verified release fields only. This snapshot does not cover all newer skins;
// absent IDs stay unknown rather than deriving dates from IDs or asset paths.
//
//go:embed data/skin_release_dates.json
var skinReleaseSnapshot []byte

// Kept separate so regenerating the older Meraki baseline cannot silently
// erase dates cross-checked against Riot's actual skin availability notices.
//
//go:embed data/skin_release_overrides.json
var skinReleaseOverrides []byte

var skinReleaseDates = func() map[string]string {
	var snapshot struct {
		Dates map[string]string `json:"dates"`
	}
	if err := json.Unmarshal(skinReleaseSnapshot, &snapshot); err != nil {
		panic(err)
	}
	var verified struct{ Records []struct{ ID, Date string } }
	if err := json.Unmarshal(skinReleaseOverrides, &verified); err != nil {
		panic(err)
	}
	for _, record := range verified.Records {
		snapshot.Dates[record.ID] = record.Date
	}
	return snapshot.Dates
}()

func projectFacadeSkin(skin Skin) facadeSkin {
	result := facadeSkin{ID: skin.ID, Name: skin.Name, ChampionID: skin.ChampionID, ChampionName: skin.ChampionName, SplashPath: skin.SplashPath, TilePath: skin.TilePath, Owned: skin.Owned, IsVariant: skin.IsVariant, ParentSkinID: skin.ParentSkinID}
	result.ReleaseDate = skinReleaseDates[strconv.FormatInt(skin.ID, 10)]
	result.ReleaseSortDate = result.ReleaseDate
	if result.ReleaseSortDate == "" && skin.IsVariant {
		// Group a quest tier with its parent. This is a sorting fallback, NOT an
		// independently verified release date or evidence of variant ownership.
		result.ReleaseSortDate = skinReleaseDates[strconv.FormatInt(skin.ParentSkinID, 10)]
	}
	return result
}
