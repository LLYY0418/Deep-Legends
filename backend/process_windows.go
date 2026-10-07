//go:build windows

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	processQueryLimitedInformation = 0x1000
	processQueryInformation        = 0x0400
	processVMRead                  = 0x0010
	processCommandLineInformation  = 60
	createNoWindow                 = 0x08000000
)

var ntQueryInformationProcess = syscall.NewLazyDLL("ntdll.dll").NewProc("NtQueryInformationProcess")

type nativeUnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

func hideCommandWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func nativeLeagueProcessCommands() (processQueryResult, error) {
	return nativeProcessCommands("LeagueClientUx.exe", "LeagueClient.exe")
}

func nativeRiotClientProcessCommands() (processQueryResult, error) {
	return nativeProcessCommands("RiotClientServices.exe")
}

func nativeLeagueProcessSnapshot() (processQueryResult, error) {
	return nativeProcessSnapshot(false, "LeagueClientUx.exe", "LeagueClient.exe")
}
func nativeProcessCommands(processNames ...string) (processQueryResult, error) {
	return nativeProcessSnapshot(true, processNames...)
}
func nativeProcessSnapshot(readCommands bool, processNames ...string) (processQueryResult, error) {
	result := processQueryResult{Method: "native"}
	accepted := make(map[string]struct{}, len(processNames))
	for _, name := range processNames {
		accepted[strings.ToLower(name)] = struct{}{}
	}
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return result, fmt.Errorf("create process snapshot: %w", err)
	}
	defer syscall.CloseHandle(snapshot)

	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	if err := syscall.Process32First(snapshot, &entry); err != nil {
		return result, fmt.Errorf("read first process: %w", err)
	}
	for {
		name := syscall.UTF16ToString(entry.ExeFile[:])
		if _, ok := accepted[strings.ToLower(name)]; ok {
			result.ProcessCount++
			result.ProcessIDs = append(result.ProcessIDs, entry.ProcessID)
			if !readCommands {
				err = syscall.Process32Next(snapshot, &entry)
				if err != nil {
					break
				}
				continue
			}
			if strings.EqualFold(name, "LeagueClient.exe") {
				if path := windowsProcessImagePath(entry.ProcessID); path != "" {
					result.ImagePaths = append(result.ImagePaths, path)
				}
			}
			commandLine, commandErr := windowsProcessCommandLine(entry.ProcessID)
			if commandErr != nil || strings.TrimSpace(commandLine) == "" {
				result.Unreadable++
			} else {
				result.CommandLines = append(result.CommandLines, commandLine)
			}
		}
		err = syscall.Process32Next(snapshot, &entry)
		if err != nil {
			break
		}
	}
	if !errors.Is(err, syscall.ERROR_NO_MORE_FILES) {
		return result, fmt.Errorf("enumerate processes: %w", err)
	}
	return result, nil
}

func windowsProcessCommandLine(pid uint32) (string, error) {
	var lastErr error
	for _, access := range []uint32{processQueryLimitedInformation, processQueryInformation | processVMRead} {
		handle, err := syscall.OpenProcess(access, false, pid)
		if err != nil {
			lastErr = err
			continue
		}
		commandLine, queryErr := queryProcessCommandLine(handle)
		_ = syscall.CloseHandle(handle)
		if queryErr == nil {
			return commandLine, nil
		}
		lastErr = queryErr
	}
	if lastErr == nil {
		lastErr = errors.New("process command line unavailable")
	}
	return "", lastErr
}

func queryProcessCommandLine(handle syscall.Handle) (string, error) {
	var size uint32
	_, _, _ = ntQueryInformationProcess.Call(
		uintptr(handle), processCommandLineInformation, 0, 0, uintptr(unsafe.Pointer(&size)),
	)
	if size < uint32(unsafe.Sizeof(nativeUnicodeString{})) || size > 1024*1024 {
		return "", errors.New("invalid command-line buffer size")
	}
	buffer := make([]byte, size)
	status, _, _ := ntQueryInformationProcess.Call(
		uintptr(handle), processCommandLineInformation, uintptr(unsafe.Pointer(&buffer[0])), uintptr(size), uintptr(unsafe.Pointer(&size)),
	)
	if status != 0 {
		return "", fmt.Errorf("NtQueryInformationProcess status 0x%x", status)
	}
	value := (*nativeUnicodeString)(unsafe.Pointer(&buffer[0]))
	if value.Buffer == nil || value.Length == 0 || value.Length%2 != 0 || int(value.Length) > len(buffer) {
		return "", errors.New("invalid command-line response")
	}
	bufferStart := uintptr(unsafe.Pointer(&buffer[0]))
	bufferEnd := bufferStart + uintptr(len(buffer))
	stringStart := uintptr(unsafe.Pointer(value.Buffer))
	stringLength := uintptr(value.Length)
	if stringStart < bufferStart || stringStart >= bufferEnd || stringLength > bufferEnd-stringStart {
		return "", errors.New("command-line string points outside response buffer")
	}
	commandLine := syscall.UTF16ToString(unsafe.Slice(value.Buffer, int(value.Length/2)))
	if strings.TrimSpace(commandLine) == "" {
		return "", errors.New("empty command line")
	}
	return commandLine, nil
}

func windowsProcessImagePath(pid uint32) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size) != nil {
		return ""
	}
	return windows.UTF16ToString(buffer[:size])
}
