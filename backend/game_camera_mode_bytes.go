package main

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

type cameraJSONSpan struct {
	start, end int
	value      any
	object     map[string]*cameraJSONSpan
	array      []*cameraJSONSpan
}

func cameraJSONTree(data []byte) (*cameraJSONSpan, error) {
	if !json.Valid(data) {
		return nil, errors.New("invalid settings JSON")
	}
	offset := 0
	space := func() {
		for offset < len(data) && strings.ContainsRune(" \t\r\n", rune(data[offset])) {
			offset++
		}
	}
	var read func(int) (*cameraJSONSpan, error)
	read = func(depth int) (*cameraJSONSpan, error) {
		if depth > 32 {
			return nil, errors.New("settings too deep")
		}
		space()
		n := &cameraJSONSpan{start: offset}
		switch data[offset] {
		case '{':
			offset++
			space()
			n.object = map[string]*cameraJSONSpan{}
			for data[offset] != '}' {
				key, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				name, ok := key.value.(string)
				if !ok || n.object[name] != nil {
					return nil, errors.New("duplicate settings key")
				}
				space()
				offset++
				child, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				n.object[name] = child
				space()
				if data[offset] == ',' {
					offset++
					space()
				} else {
					break
				}
			}
			offset++
		case '[':
			offset++
			space()
			for data[offset] != ']' {
				child, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				n.array = append(n.array, child)
				space()
				if data[offset] == ',' {
					offset++
					space()
				} else {
					break
				}
			}
			offset++
		case '"':
			offset++
			for offset < len(data) {
				if data[offset] == '\\' {
					offset += 2
					continue
				}
				if data[offset] == '"' {
					offset++
					break
				}
				offset++
			}
			if json.Unmarshal(data[n.start:offset], &n.value) != nil {
				return nil, errors.New("invalid scalar")
			}
		default:
			for offset < len(data) && !strings.ContainsRune(" \t\r\n,}]", rune(data[offset])) {
				offset++
			}
			decoder := json.NewDecoder(strings.NewReader(string(data[n.start:offset])))
			decoder.UseNumber()
			if decoder.Decode(&n.value) != nil {
				return nil, errors.New("invalid scalar")
			}
		}
		n.end = offset
		return n, nil
	}
	return read(0)
}
func cameraJSONValueSpan(root *cameraJSONSpan) (*cameraJSONSpan, error) {
	var found []*cameraJSONSpan
	var walk func(*cameraJSONSpan, string)
	walk = func(n *cameraJSONSpan, prefix string) {
		if n.object != nil {
			if nameNode := n.object["name"]; nameNode != nil {
				name, _ := nameNode.value.(string)
				if strings.EqualFold(name, "Game.cfg") || strings.EqualFold(name, "Input.ini") {
					prefix = name
				} else {
					prefix = strings.Trim(prefix+"."+name, ".")
				}
				if value := n.object["value"]; value != nil {
					if strings.EqualFold(prefix, "Game.cfg.General.CameraMode") || strings.EqualFold(prefix, "General.CameraMode") {
						found = append(found, value)
					}
					return
				}
			}
			for key, child := range n.object {
				if key == "name" || key == "description" {
					continue
				}
				path := strings.Trim(prefix+"."+key, ".")
				if key == "files" || key == "sections" || key == "settings" {
					path = prefix
				}
				if (strings.EqualFold(path, "Game.cfg.General.CameraMode") || strings.EqualFold(path, "General.CameraMode")) && child.object == nil && child.array == nil {
					found = append(found, child)
				} else {
					walk(child, path)
				}
			}
		}
		for _, child := range n.array {
			walk(child, prefix)
		}
	}
	walk(root, "")
	if len(found) != 1 {
		return nil, errors.New("camera setting missing or ambiguous")
	}
	return found[0], nil
}
func replaceCameraModeBytes(data []byte, isJSON bool, target int) ([]byte, string, error) {
	start, end := -1, -1
	before := ""
	replacement := strconv.Itoa(target)
	if isJSON {
		tree, err := cameraJSONTree(data)
		if err != nil {
			return nil, "", err
		}
		node, err := cameraJSONValueSpan(tree)
		if err != nil {
			return nil, "", err
		}
		start, end = node.start, node.end
		switch value := node.value.(type) {
		case string:
			before = value
			quoted, _ := json.Marshal(replacement)
			replacement = string(quoted)
		case json.Number:
			before = string(value)
		default:
			return nil, "", errors.New("camera setting is not numeric")
		}
	} else {
		section := ""
		offset := 0
		for _, line := range strings.SplitAfter(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "]") {
				section = strings.TrimSpace(strings.SplitN(trimmed[1:], "]", 2)[0])
			}
			if strings.EqualFold(section, "General") {
				equal := strings.IndexByte(line, '=')
				if equal >= 0 && strings.EqualFold(strings.TrimSpace(line[:equal]), "CameraMode") {
					if start >= 0 {
						return nil, "", errors.New("duplicate camera setting")
					}
					left := equal + 1
					for left < len(line) && (line[left] == ' ' || line[left] == '\t') {
						left++
					}
					right := left
					for right < len(line) && line[right] >= '0' && line[right] <= '9' {
						right++
					}
					remainder := strings.TrimSpace(line[right:])
					if right == left || remainder != "" && !strings.HasPrefix(remainder, ";") && !strings.HasPrefix(remainder, "#") {
						return nil, "", errors.New("invalid camera value")
					}
					start, end = offset+left, offset+right
					before = line[left:right]
				}
			}
			offset += len(line)
		}
	}
	if start < 0 || cameraValue(before) == "unknown" {
		return nil, "", errors.New("camera value unavailable")
	}
	out := append([]byte(nil), data[:start]...)
	out = append(out, []byte(replacement)...)
	out = append(out, data[end:]...)
	return out, cameraValue(before), nil
}
