package main

import (
	"errors"
	"strings"
	"unicode/utf16"
)

const productFolder = "Deep Legends"

func cleanPathInput(input string) string {
	input = strings.TrimSpace(input)
	if len(input) >= 2 && input[0] == '"' && input[len(input)-1] == '"' {
		input = strings.TrimSpace(input[1 : len(input)-1])
	}
	return strings.ReplaceAll(input, "/", `\`)
}

func normalizeInstallDir(input string) (string, error) {
	input = cleanPathInput(input)
	if len(input) < 3 || !((input[0] >= 'a' && input[0] <= 'z') || (input[0] >= 'A' && input[0] <= 'Z')) || input[1:3] != `:\` {
		return "", errors.New("请填写本机磁盘的完整路径")
	}
	var parts []string
	for _, part := range strings.Split(input[3:], `\`) {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(parts) == 0 {
				return "", errors.New("安装路径不能超出磁盘根目录")
			}
			parts = parts[:len(parts)-1]
			continue
		}
		if strings.ContainsAny(part, `<>:"|?*`) || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", errors.New("安装路径包含不支持的字符")
		}
		for _, c := range part {
			if c < 32 || c == 127 {
				return "", errors.New("安装路径包含不支持的字符")
			}
		}
		device := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		device = strings.TrimRight(device, " ")
		if device == "CON" || device == "PRN" || device == "AUX" || device == "NUL" ||
			(len(device) == 4 && (strings.HasPrefix(device, "COM") || strings.HasPrefix(device, "LPT")) && device[3] >= '1' && device[3] <= '9') {
			return "", errors.New("安装路径不能使用系统保留名称")
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "", errors.New("请选择磁盘内的文件夹，不能直接安装到磁盘根目录")
	}
	result := strings.ToUpper(input[:1]) + `:\` + strings.Join(parts, `\`)
	if len(utf16.Encode([]rune(result))) > 180 {
		return "", errors.New("路径太长，请选择更短的安装路径")
	}
	return result, nil
}

func appendProductFolder(selected string) string {
	selected = strings.TrimRight(cleanPathInput(selected), `\`)
	leaf := selected[strings.LastIndex(selected, `\`)+1:]
	if strings.EqualFold(leaf, productFolder) {
		return selected
	}
	return selected + `\` + productFolder
}

func defaultInstallDir(localAppData string) string {
	return strings.TrimRight(cleanPathInput(localAppData), `\`) + `\Programs\` + productFolder
}
