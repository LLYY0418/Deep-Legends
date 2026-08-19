package main

// riot_client.go 连接本机 RiotClientServices 进程。它与 League Client
// (LCU) 是两个独立的本地 HTTPS 服务，端口和短期令牌不能混用：
// Riot Client 负责跨国服子服务器解析 Riot ID，LCU 继续提供当前登录
// 会话的 league-session 令牌，供腾讯 SGP 查询目标服务器资料。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var (
	riotClientPortPattern = regexp.MustCompile(`(?i)--riotclient-app-port(?:=|\s+)"?(\d+)"?`)
	riotClientAuthPattern = regexp.MustCompile(`(?i)--riotclient-auth-token(?:=|\s+)"?([^\s"]+)"?`)

	errRiotClientNotFound              = errors.New("RiotClientServices process was not found")
	errRiotClientCredentialsUnreadable = errors.New("RiotClientServices process found but command-line credentials are unavailable")
	errRiotClientAliasNotFound         = errors.New("Riot ID alias was not found")
)

// RiotClientAPI is intentionally a distinct type even though Riot Client and
// LCU use the same loopback TLS and Basic-auth transport. Keeping the wrapper
// separate prevents their credentials from being accidentally interchanged.
type RiotClientAPI struct {
	local *LCUClient
}

type riotClientAlias struct {
	Alias struct {
		GameName string `json:"game_name"`
		TagLine  string `json:"tag_line"`
	} `json:"alias"`
	PUUID string `json:"puuid"`
}

func newRiotClientAPI(port int, token string) *RiotClientAPI {
	return &RiotClientAPI{local: newLCUClient(port, token)}
}

func (c *RiotClientAPI) Close() {
	if c != nil && c.local != nil {
		c.local.Close()
	}
}

func riotClientFromCommandLine(commandLine string) (*RiotClientAPI, bool) {
	if !strings.Contains(strings.ToLower(commandLine), "riotclientservices.exe") {
		return nil, false
	}
	portMatch := riotClientPortPattern.FindStringSubmatch(commandLine)
	authMatch := riotClientAuthPattern.FindStringSubmatch(commandLine)
	if len(portMatch) != 2 || len(authMatch) != 2 {
		return nil, false
	}
	port, err := strconv.Atoi(portMatch[1])
	if err != nil || port < 1 || port > 65535 || strings.TrimSpace(authMatch[1]) == "" {
		return nil, false
	}
	return newRiotClientAPI(port, authMatch[1]), true
}

func discoverRiotClient() (*RiotClientAPI, error) {
	if err := ensureWindows(); err != nil {
		return nil, err
	}
	query, nativeErr := nativeRiotClientProcessCommands()
	if nativeErr != nil || query.ProcessCount > 0 && len(query.CommandLines) == 0 {
		fallback, fallbackErr := powerShellRiotClientProcessCommands()
		if fallbackErr == nil {
			query = fallback
		} else if nativeErr != nil {
			return nil, fmt.Errorf("RiotClientServices process lookup failed: native: %v; powershell: %w", nativeErr, fallbackErr)
		}
	}
	for _, commandLine := range query.CommandLines {
		if client, ok := riotClientFromCommandLine(commandLine); ok {
			return client, nil
		}
	}
	if query.ProcessCount > 0 {
		return nil, errRiotClientCredentialsUnreadable
	}
	return nil, errRiotClientNotFound
}

func powerShellRiotClientProcessCommands() (processQueryResult, error) {
	script := `$items = @(Get-CimInstance Win32_Process | Where-Object { $_.Name -eq 'RiotClientServices.exe' }); Write-Output ('COUNT:' + $items.Count); foreach ($item in $items) { if ([string]::IsNullOrWhiteSpace($item.CommandLine)) { Write-Output 'UNREADABLE' } else { Write-Output ('CMD:' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($item.CommandLine))) } }`
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	hideCommandWindow(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return processQueryResult{Method: "powershell"}, fmt.Errorf("powershell Riot Client process query failed: %w", err)
	}
	return parsePowerShellProcessOutput(output, "powershell")
}

func (c *RiotClientAPI) aliasesByRiotID(ctx context.Context, gameName, tagLine string) ([]riotClientAlias, error) {
	if c == nil || c.local == nil {
		return nil, errRiotClientNotFound
	}
	query := url.Values{"gameName": {strings.TrimSpace(gameName)}, "tagLine": {strings.TrimSpace(tagLine)}}
	path := "/player-account/aliases/v1/lookup?" + query.Encode()
	var aliases []riotClientAlias
	if err := c.local.RequestJSON(ctx, http.MethodGet, path, nil, &aliases); err != nil {
		var httpErr *LCUHTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, errRiotClientAliasNotFound
		}
		return nil, fmt.Errorf("Riot Client alias lookup failed: %w", err)
	}
	filtered := make([]riotClientAlias, 0, len(aliases))
	for _, alias := range aliases {
		if !validPlayerReference(alias.PUUID) {
			continue
		}
		if alias.Alias.GameName != "" && !strings.EqualFold(strings.TrimSpace(alias.Alias.GameName), strings.TrimSpace(gameName)) {
			continue
		}
		if alias.Alias.TagLine != "" && !strings.EqualFold(strings.TrimSpace(alias.Alias.TagLine), strings.TrimSpace(tagLine)) {
			continue
		}
		filtered = append(filtered, alias)
	}
	if len(filtered) == 0 {
		return nil, errRiotClientAliasNotFound
	}
	return filtered, nil
}
