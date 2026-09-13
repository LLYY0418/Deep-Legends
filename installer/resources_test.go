package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"testing"
)

func TestWindowsResourcesMatchManifestAndWindowIcon(t *testing.T) {
	object, err := pe.Open("rsrc_windows_amd64.syso")
	if err != nil {
		t.Fatal(err)
	}
	defer object.Close()
	section := object.Section(".rsrc")
	if section == nil {
		t.Fatal("missing Windows resources")
	}
	data, err := section.Data()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile("app.manifest")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, manifest) {
		t.Fatal("regenerate syso after changing app.manifest")
	}
	for _, required := range []string{`level="asInvoker"`, `>true/pm<`, `>PerMonitorV2,PerMonitor<`} {
		if !bytes.Contains(data, []byte(required)) {
			t.Fatalf("missing resource setting: %s", required)
		}
	}
	// IMAGE_RESOURCE_DIRECTORY: follow RT_GROUP_ICON to its resource ID.
	u16 := func(offset int) int { return int(binary.LittleEndian.Uint16(data[offset:])) }
	u32 := func(offset int) uint32 { return binary.LittleEndian.Uint32(data[offset:]) }
	for i := 0; i < u16(12)+u16(14); i++ {
		entry := 16 + 8*i
		if u32(entry) != 14 {
			continue
		}
		directory := int(u32(entry+4) & 0x7fffffff)
		for j := 0; j < u16(directory+12)+u16(directory+14); j++ {
			if u32(directory+16+8*j) == windowIconResourceID {
				return
			}
		}
	}
	t.Fatal("LoadIcon resource ID is absent from the embedded icon groups")
}
