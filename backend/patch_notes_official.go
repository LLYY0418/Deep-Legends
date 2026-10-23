package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/text/encoding/simplifiedchinese"
)

const officialPatchIndex = "https://apps.game.qq.com/cmc/zmMcnTargetContentList?r0=jsonp&page=1&num=16&target=24&source=web_pc"

var officialPatchTitle = regexp.MustCompile(`^(\d+\.\d+)版本更新公告$`)

func readOfficialPatchURL(raw []byte) (string, string, error) {
	// The official index is JSONP. Decode the JSON value, never execute it.
	if start, end := bytes.IndexByte(raw, '('), bytes.LastIndexByte(raw, ')'); start >= 0 && end > start {
		raw = raw[start+1 : end]
	}
	var index struct {
		Data struct {
			Result []struct {
				Title string `json:"sTitle"`
				URL   string `json:"sRedirectURL"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		return "", "", err
	}
	version, url := "", ""
	for _, row := range index.Data.Result {
		match := officialPatchTitle.FindStringSubmatch(row.Title)
		if len(match) == 2 && validPatchSource(row.URL) && (version == "" || comparePatchVersions(match[1], version) > 0) {
			version, url = match[1], row.URL
		}
	}
	if url == "" {
		return "", "", errors.New("official patch index has no valid announcement")
	}
	return version, url, nil
}

func fetchOfficialPatchBytes(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("official patch source HTTP %d", response.StatusCode)
	}
	if response.Request != nil && response.Request.URL.String() != url {
		return nil, errors.New("official patch source redirected")
	}
	return readLimited(response.Body, limit)
}

func fetchLatestOfficialPatchNotes(ctx context.Context, client *http.Client, current string) ([]patchNoteDocument, error) {
	raw, err := fetchOfficialPatchBytes(ctx, client, officialPatchIndex, patchNotesLimit)
	if err != nil {
		return nil, err
	}
	version, url, err := readOfficialPatchURL(raw)
	if err != nil {
		return nil, err
	}
	if current != "" && comparePatchVersions(version, current) <= 0 {
		return nil, nil
	}
	raw, err = fetchOfficialPatchBytes(ctx, client, url, 2<<20)
	if err != nil {
		return nil, err
	}
	doc, err := parseOfficialPatchNotes(raw, url)
	if err != nil {
		return nil, err
	}
	if doc.Version != version {
		return nil, errors.New("official patch title does not match index")
	}
	return []patchNoteDocument{doc}, nil
}

// Parse only the checked article subtree. Keep official text, including future
// section names, as text rather than executing HTML or inventing missing IDs.
func parseOfficialPatchNotes(raw []byte, source string) (patchNoteDocument, error) {
	var result patchNoteDocument
	if !validPatchSource(source) {
		return result, errors.New("invalid official patch URL")
	}
	header := strings.ToLower(string(raw[:min(len(raw), 2000)]))
	if strings.Contains(header, "charset=gb") || strings.Contains(header, `charset="gb`) {
		decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw)
		if err != nil {
			return result, err
		}
		raw = decoded
	}
	root, err := xhtml.Parse(bytes.NewReader(raw))
	if err != nil {
		return result, err
	}
	var article *xhtml.Node
	var visit func(*xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			if attribute(n, "id") == "article" {
				article = n
			}
			if n.Data == "h1" && strings.Contains(attribute(n, "class"), "art-tit") {
				m := officialPatchTitle.FindStringSubmatch(strings.TrimSpace(nodeText(n)))
				if len(m) == 2 {
					result.Version = m[1]
				}
			}
			if strings.Contains(attribute(n, "class"), "art-time") {
				result.PublishedAt = attribute(n, "data")
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	if article == nil || result.Version == "" || !strings.Contains(nodeText(article), "国服") {
		return result, errors.New("unrecognized national-server patch article")
	}
	result.Schema, result.SourceURL = 1, source
	result.Modes.Ranked.Sections = []patchNoteSection{}
	result.Modes.Mayhem.Sections = []patchNoteSection{}
	result.Modes.Arena.Sections = []patchNoteSection{}
	mode := ""
	major := ""
	section := -1
	newAugments := false
	entry := -1
	sections := func() *[]patchNoteSection {
		if mode == "mayhem" {
			return &result.Modes.Mayhem.Sections
		}
		if mode == "arena" {
			return &result.Modes.Arena.Sections
		}
		return &result.Modes.Ranked.Sections
	}
	addSection := func(title string) {
		s := sections()
		*s = append(*s, patchNoteSection{ID: fmt.Sprintf("official-%d", len(*s)), Title: title})
		section = len(*s) - 1
		entry = -1
	}
	var blocks []*xhtml.Node
	var collect func(*xhtml.Node)
	collect = func(parent *xhtml.Node) {
		for n := parent.FirstChild; n != nil; n = n.NextSibling {
			if n.Type != xhtml.ElementNode {
				continue
			}
			switch n.Data {
			case "div", "section", "main":
				collect(n)
			case "script", "style":
				continue
			default:
				blocks = append(blocks, n)
			}
		}
	}
	collect(article)
	for _, n := range blocks {
		if n.Type != xhtml.ElementNode || n.Data == "script" || n.Data == "style" {
			continue
		}
		text := strings.Join(strings.Fields(nodeText(n)), " ")
		if text == "" {
			continue
		}
		if n.Data == "h3" {
			major = text
			newAugments = false
			section = -1
			entry = -1
			switch text {
			case "海克斯大乱斗":
				mode = "mayhem"
			case "斗魂竞技场":
				mode = "arena"
			case "版本要闻", "经典模式", "极地大乱斗", "即将到来的皮肤和炫彩":
				mode = ""
			default:
				mode = "ranked"
				addSection(text)
			}
			continue
		}
		if mode == "" {
			continue
		}
		if n.Data == "h4" {
			newAugments = strings.Contains(text, "新增") && strings.Contains(text, "强化符文")
			if mode == "ranked" && (major == "英雄" || major == "装备") {
				if section < 0 {
					addSection(major)
				}
				s := sections()
				(*s)[section].Entries = append((*s)[section].Entries, patchNoteEntry{Name: text, Groups: []patchNoteGroup{{Lines: []patchNoteLine{}}}})
				entry = len((*s)[section].Entries) - 1
			} else {
				addSection(text)
			}
			continue
		}
		if section < 0 {
			addSection(major)
		}
		if newAugments && n.Data == "p" && text != "白银阶" && text != "黄金阶" && text != "棱彩阶" {
			s := sections()
			(*s)[section].Entries = append((*s)[section].Entries, patchNoteEntry{Name: text, IsNew: true, Groups: []patchNoteGroup{{Lines: []patchNoteLine{}}}})
			entry = len((*s)[section].Entries) - 1
			continue
		}
		blocks := []*xhtml.Node{n}
		if n.Data == "ul" || n.Data == "ol" {
			blocks = nil
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == xhtml.ElementNode && child.Data == "li" {
					blocks = append(blocks, child)
				}
			}
		}
		for _, block := range blocks {
			text := strings.Join(strings.Fields(nodeText(block)), " ")
			if text == "" {
				continue
			}
			s := sections()
			if entry >= 0 {
				e := &(*s)[section].Entries[entry]
				e.Groups[0].Lines = append(e.Groups[0].Lines, patchNoteLine{Text: text})
			} else {
				(*s)[section].Items = append((*s)[section].Items, text)
			}
		}
	}
	if len(result.Modes.Ranked.Sections)+len(result.Modes.Mayhem.Sections)+len(result.Modes.Arena.Sections) == 0 {
		return patchNoteDocument{}, errors.New("official patch article has no recognized content")
	}
	if err := validatePatchDocument(result); err != nil {
		return patchNoteDocument{}, err
	}
	return result, nil
}
