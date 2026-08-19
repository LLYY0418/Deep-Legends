package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	appPortPattern              = regexp.MustCompile(`--app-port(?:=|\s+)"?(\d+)"?`)
	authPattern                 = regexp.MustCompile(`--remoting-auth-token(?:=|\s+)"?([^\s"]+)"?`)
	installPattern              = regexp.MustCompile(`--install-directory(?:=|\s+)("[^"]+"|[^\s]+)`)
	regionPattern               = regexp.MustCompile(`--region(?:=|\s+)"?([A-Za-z0-9_]+)"?`)
	rsoPlatformPattern          = regexp.MustCompile(`--rso_platform_id(?:=|\s+)"?([A-Za-z0-9_]+)"?`)
	errLCUNotFound              = errors.New("LeagueClient process was not found")
	errLCUCredentialsUnreadable = errors.New("LeagueClient process found but command-line credentials are unavailable")
	errLCUProbeFailed           = errors.New("LeagueClient credentials found but local API probe failed")
)

type processQueryResult struct {
	CommandLines []string
	ProcessCount int
	Unreadable   int
	Method       string
}

type LCUDiscoveryStatus struct {
	AttemptAt            time.Time `json:"attemptAt"`
	Method               string    `json:"method"`
	ProcessCount         int       `json:"processCount"`
	UnreadableProcesses  int       `json:"unreadableProcesses"`
	CommandLineCount     int       `json:"commandLineCount"`
	CredentialCandidates int       `json:"credentialCandidates"`
	LockfilesChecked     int       `json:"lockfilesChecked"`
	LockfilesFound       int       `json:"lockfilesFound"`
	ProbeFailures        int       `json:"probeFailures"`
	Result               string    `json:"result"`
	Detail               string    `json:"detail"`
}

type LCUClient struct {
	mu      sync.RWMutex
	baseURL string
	token   string
	http    *http.Client
	port    int
	source  string
	// region 与 rsoPlatform 来自客户端启动参数（例如 TENCENT / HN1），
	// 用于确定国服玩家所属的 SGP 大区服务器；读取失败时留空。
	region        string
	rsoPlatform   string
	platformProbe bool
}

type LCUHTTPError struct {
	Method     string
	Path       string
	StatusCode int
}

func (e *LCUHTTPError) Error() string {
	method := e.Method
	if method == "" {
		method = http.MethodGet
	}
	return fmt.Sprintf("LCU %s %s: HTTP %d", method, e.Path, e.StatusCode)
}

type Summoner struct {
	SummonerID    int64  `json:"summonerId"`
	AccountID     int64  `json:"accountId,omitempty"`
	PUUID         string `json:"puuid,omitempty"`
	DisplayName   string `json:"displayName,omitempty"`
	GameName      string `json:"gameName,omitempty"`
	TagLine       string `json:"tagLine,omitempty"`
	ProfileIconID int64  `json:"profileIconId,omitempty"`
	SummonerLevel int64  `json:"summonerLevel,omitempty"`
	// Privacy 为 "PRIVATE" 表示玩家开启了“隐藏战绩”；界面据此在
	// 总览头部展示“隐藏战绩”标签，不影响战绩加载本身。
	Privacy string `json:"privacy,omitempty"`
}

func discoverLCU() (*LCUClient, error) {
	client, _, err := discoverLCUDetailed()
	return client, err
}

func discoverLCUDetailed() (*LCUClient, LCUDiscoveryStatus, error) {
	report := LCUDiscoveryStatus{AttemptAt: time.Now(), Result: "searching"}
	if err := ensureWindows(); err != nil {
		report.Result = "unsupported"
		report.Detail = "当前系统不支持客户端发现"
		return nil, report, err
	}
	query, commandErr := leagueProcessCommands()
	lines := query.CommandLines
	report.Method = query.Method
	report.ProcessCount = query.ProcessCount
	report.UnreadableProcesses = query.Unreadable
	report.CommandLineCount = len(lines)
	for _, commandLine := range lines {
		if client, ok := clientFromCommandLine(commandLine); ok {
			report.CredentialCandidates++
			client.source = "process"
			if err := client.probe(); err == nil {
				report.Result = "connected"
				report.Detail = "已通过客户端进程连接"
				return client, report, nil
			}
			report.ProbeFailures++
			client.Close()
		}
	}

	lockfiles := lockfileCandidates(lines)
	report.LockfilesChecked = len(lockfiles)
	for _, path := range lockfiles {
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			report.LockfilesFound++
		}
		client, err := clientFromLockfile(path)
		if err == nil {
			client.source = "lockfile"
			if err := client.probe(); err == nil {
				report.Result = "connected"
				report.Detail = "已通过客户端 lockfile 连接"
				return client, report, nil
			}
			report.ProbeFailures++
			client.Close()
		}
	}
	if commandErr != nil {
		report.Result = "process-query-failed"
		report.Detail = "客户端进程查询失败"
		return nil, report, fmt.Errorf("LeagueClient process lookup failed: %w", commandErr)
	}
	if report.CredentialCandidates > 0 || report.LockfilesFound > 0 {
		report.Result = "probe-failed"
		report.Detail = "已找到客户端凭据，但本地接口尚未响应"
		return nil, report, errLCUProbeFailed
	}
	if report.ProcessCount > 0 {
		report.Result = "credentials-unreadable"
		report.Detail = "已检测到客户端进程，但无法读取连接凭据"
		return nil, report, errLCUCredentialsUnreadable
	}
	report.Result = "process-not-found"
	report.Detail = "未检测到 LeagueClientUx 进程"
	return nil, report, errLCUNotFound
}

func leagueProcessCommands() (processQueryResult, error) {
	native, nativeErr := nativeLeagueProcessCommands()
	if nativeErr == nil && (native.ProcessCount == 0 || len(native.CommandLines) > 0) {
		return native, nil
	}
	fallback, fallbackErr := powerShellLeagueProcessCommands()
	if fallbackErr == nil {
		if native.ProcessCount > fallback.ProcessCount {
			fallback.ProcessCount = native.ProcessCount
		}
		if native.Unreadable > fallback.Unreadable {
			fallback.Unreadable = native.Unreadable
		}
		return fallback, nil
	}
	if nativeErr != nil {
		return processQueryResult{Method: "native+powershell"}, fmt.Errorf("native: %v; powershell: %w", nativeErr, fallbackErr)
	}
	return native, nil
}

func powerShellLeagueProcessCommands() (processQueryResult, error) {
	script := `$names = @('LeagueClientUx.exe','LeagueClient.exe'); $items = @(Get-CimInstance Win32_Process | Where-Object { $names -contains $_.Name }); Write-Output ('COUNT:' + $items.Count); foreach ($item in $items) { if ([string]::IsNullOrWhiteSpace($item.CommandLine)) { Write-Output 'UNREADABLE' } else { Write-Output ('CMD:' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($item.CommandLine))) } }`
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	hideCommandWindow(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return processQueryResult{Method: "powershell"}, fmt.Errorf("powershell process query failed: %w", err)
	}
	return parsePowerShellProcessOutput(output, "powershell")
}

func parsePowerShellProcessOutput(output []byte, method string) (processQueryResult, error) {
	result := processQueryResult{Method: method}
	countSeen := false
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "COUNT:"):
			if countSeen {
				return result, errors.New("process query returned duplicate COUNT")
			}
			count, parseErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "COUNT:")))
			if parseErr != nil || count < 0 {
				return result, errors.New("process query returned an invalid COUNT")
			}
			result.ProcessCount = count
			countSeen = true
		case line == "UNREADABLE":
			result.Unreadable++
		case strings.HasPrefix(line, "CMD:"):
			decoded, decodeErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "CMD:"))
			if decodeErr != nil || len(decoded) == 0 {
				return result, errors.New("process query returned an invalid command line")
			}
			result.CommandLines = append(result.CommandLines, string(decoded))
		}
	}
	if !countSeen {
		return result, errors.New("process query did not return COUNT")
	}
	accounted := len(result.CommandLines) + result.Unreadable
	if accounted > result.ProcessCount {
		return result, errors.New("process query returned inconsistent process counts")
	}
	result.Unreadable += result.ProcessCount - accounted
	return result, nil
}

func clientFromCommandLine(commandLine string) (*LCUClient, bool) {
	lower := strings.ToLower(commandLine)
	if !strings.Contains(lower, "leagueclientux.exe") && !strings.Contains(lower, "leagueclient.exe") {
		return nil, false
	}
	portMatch := appPortPattern.FindStringSubmatch(commandLine)
	authMatch := authPattern.FindStringSubmatch(commandLine)
	if len(portMatch) != 2 || len(authMatch) != 2 {
		return nil, false
	}
	port, err := strconv.Atoi(portMatch[1])
	if err != nil || port < 1 || port > 65535 {
		return nil, false
	}
	client := newLCUClient(port, authMatch[1])
	client.applyPlatformArgs(commandLine)
	return client, true
}

func (c *LCUClient) applyPlatformArgs(commandLine string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if match := regionPattern.FindStringSubmatch(commandLine); len(match) == 2 {
		c.region = strings.ToUpper(match[1])
	}
	if match := rsoPlatformPattern.FindStringSubmatch(commandLine); len(match) == 2 {
		c.rsoPlatform = strings.ToUpper(match[1])
	}
	if c.region != "" {
		c.platformProbe = true
	}
}

// platformInfo 返回客户端所属大区（如 TENCENT）与子服务器（如 HN1）。
// 启动参数缺失时向客户端查询一次命令行参数作为兜底。
func (c *LCUClient) platformInfo() (string, string) {
	c.mu.RLock()
	region, platform, probed := c.region, c.rsoPlatform, c.platformProbe
	c.mu.RUnlock()
	if region != "" || probed {
		return region, platform
	}
	var args []string
	if err := c.GetJSON("/riotclient/command-line-args", &args); err == nil {
		c.applyPlatformArgs(strings.Join(args, " "))
	}
	c.mu.Lock()
	c.platformProbe = true
	region, platform = c.region, c.rsoPlatform
	c.mu.Unlock()
	return region, platform
}

func lockfileCandidates(commandLines []string) []string {
	var candidates []string
	for _, commandLine := range commandLines {
		match := installPattern.FindStringSubmatch(commandLine)
		if len(match) == 2 {
			dir := strings.Trim(match[1], `"`)
			candidates = append(candidates, filepath.Join(dir, "lockfile"))
		}
	}
	if programData := os.Getenv("ProgramData"); programData != "" {
		candidates = append(candidates,
			filepath.Join(programData, "Riot Games", "League of Legends", "lockfile"),
			filepath.Join(programData, "Tencent", "League of Legends", "lockfile"),
		)
	}
	for _, envName := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if root := os.Getenv(envName); root != "" {
			candidates = append(candidates,
				filepath.Join(root, "Riot Games", "League of Legends", "lockfile"),
				filepath.Join(root, "Tencent Games", "英雄联盟", "LeagueClient", "lockfile"),
				filepath.Join(root, "腾讯游戏", "英雄联盟", "LeagueClient", "lockfile"),
			)
		}
	}
	for driveLetter := 'C'; driveLetter <= 'Z'; driveLetter++ {
		drive := string(driveLetter) + ":"
		candidates = append(candidates,
			filepath.Join(drive+`\`, "Riot Games", "League of Legends", "lockfile"),
			filepath.Join(drive+`\`, "Program Files", "Riot Games", "League of Legends", "lockfile"),
			filepath.Join(drive+`\`, "Program Files (x86)", "Riot Games", "League of Legends", "lockfile"),
			filepath.Join(drive+`\`, "Tencent Games", "英雄联盟", "LeagueClient", "lockfile"),
			filepath.Join(drive+`\`, "WeGameApps", "英雄联盟", "LeagueClient", "lockfile"),
			filepath.Join(drive+`\`, "WeGameApps", "yxlm", "LeagueClient", "lockfile"),
			filepath.Join(drive+`\`, "WeGame", "WeGameApps", "yxlm", "LeagueClient", "lockfile"),
			filepath.Join(drive+`\`, "Program Files", "腾讯游戏", "英雄联盟", "lockfile"),
			filepath.Join(drive+`\`, "腾讯游戏", "英雄联盟", "LeagueClient", "lockfile"),
			filepath.Join(drive+`\`, "英雄联盟", "LeagueClient", "lockfile"),
		)
	}
	seen := map[string]bool{}
	var unique []string
	for _, candidate := range candidates {
		key := strings.ToLower(filepath.Clean(candidate))
		if !seen[key] {
			seen[key] = true
			unique = append(unique, candidate)
		}
	}
	return unique
}

func clientFromLockfile(path string) (*LCUClient, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 5)
	if len(parts) != 5 {
		return nil, fmt.Errorf("invalid lockfile %s", path)
	}
	if !strings.Contains(strings.ToLower(parts[0]), "leagueclient") {
		return nil, fmt.Errorf("invalid lockfile client")
	}
	port, err := strconv.Atoi(parts[2])
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid lockfile port")
	}
	if strings.TrimSpace(parts[3]) == "" || !strings.EqualFold(strings.TrimSpace(parts[4]), "https") {
		return nil, fmt.Errorf("invalid lockfile credentials or protocol")
	}
	return newLCUClient(port, parts[3]), nil
}

func newLCUClient(port int, token string) *LCUClient {
	transport := &http.Transport{
		Proxy: nil,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // LCU uses a self-signed certificate on loopback only.
			MinVersion:         tls.VersionTLS12,
		},
		DisableKeepAlives: false,
	}
	return &LCUClient{
		baseURL: fmt.Sprintf("https://127.0.0.1:%d", port),
		token:   token,
		port:    port,
		http: &http.Client{
			Transport: transport,
			Timeout:   8 * time.Second,
		},
	}
}

func (c *LCUClient) probe() error {
	var value map[string]any
	if err := c.GetJSON("/lol-summoner/v1/current-summoner", &value); err != nil {
		return err
	}
	if firstInt(value, "summonerId") <= 0 {
		return errors.New("LCU probe returned no active summoner")
	}
	return nil
}

func (c *LCUClient) GetJSON(path string, target any) error {
	data, err := c.GetBytes(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// RequestJSON performs a narrowly scoped JSON request against the authenticated
// loopback LCU service. Callers remain responsible for using a fixed, reviewed
// endpoint; arbitrary paths are never accepted from the renderer.
func (c *LCUClient) RequestJSON(ctx context.Context, method, path string, body, target any) error {
	if err := validateLCURequestPath(path); err != nil {
		return err
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return fmt.Errorf("unsupported LCU method %q", method)
	}
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode %s %s: %w", method, path, err)
		}
		if len(payload) > 1024*1024 {
			return errors.New("LCU request body exceeds 1 MiB")
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	token, ok := c.credentials()
	if !ok {
		return errors.New("LCU client credentials are no longer available")
	}
	auth := base64.StdEncoding.EncodeToString([]byte("riot:" + token))
	request.Header.Set("Authorization", "Basic "+auth)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("LCU %s %s: %w", method, path, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return &LCUHTTPError{Method: method, Path: path, StatusCode: response.StatusCode}
	}
	if target == nil || response.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil
	}
	data, err := readLimited(response.Body, 4*1024*1024)
	if err != nil {
		return fmt.Errorf("LCU %s %s: %w", method, path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, path, err)
	}
	return nil
}

func validateLCURequestPath(value string) error {
	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, `\`) {
		return errors.New("LCU path must be an absolute local path")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Fragment != "" {
		return errors.New("LCU path is invalid")
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return errors.New("LCU path traversal is not allowed")
		}
	}
	return nil
}

func (c *LCUClient) GetBytes(path string) ([]byte, error) {
	return c.GetBytesContext(context.Background(), path)
}

func (c *LCUClient) GetBytesContext(ctx context.Context, path string) ([]byte, error) {
	return c.getBytes(ctx, path, 16*1024*1024, "application/json, image/*;q=0.9, */*;q=0.1")
}

func (c *LCUClient) GetMediaBytes(path string) ([]byte, error) {
	return c.GetMediaBytesContext(context.Background(), path)
}

func (c *LCUClient) GetMediaBytesContext(ctx context.Context, path string) ([]byte, error) {
	return c.getBytes(ctx, path, 64*1024*1024, "video/webm, video/mp4;q=0.9, */*;q=0.1")
}

func (c *LCUClient) getBytes(ctx context.Context, path string, limit int64, accept string) ([]byte, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("LCU path must be absolute")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	token, ok := c.credentials()
	if !ok {
		return nil, errors.New("LCU client credentials are no longer available")
	}
	auth := base64.StdEncoding.EncodeToString([]byte("riot:" + token))
	request.Header.Set("Authorization", "Basic "+auth)
	request.Header.Set("Accept", accept)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("LCU GET %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, &LCUHTTPError{Method: http.MethodGet, Path: path, StatusCode: response.StatusCode}
	}
	if response.ContentLength > limit {
		return nil, fmt.Errorf("LCU GET %s: response exceeds %d MiB", path, limit/(1024*1024))
	}
	data, err := readLimited(response.Body, limit)
	if err != nil {
		return nil, fmt.Errorf("LCU GET %s: %w", path, err)
	}
	return data, nil
}

func (c *LCUClient) credentials() (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token, c.token != ""
}

// Close removes the short-lived LCU credential from this process and releases
// idle loopback connections. The credential is never persisted or logged.
func (c *LCUClient) Close() {
	c.mu.Lock()
	c.token = ""
	c.mu.Unlock()
	if transport, ok := c.http.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}
