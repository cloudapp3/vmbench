//go:build linux

package sysinfo

import "testing"

const dmidecodeTwoModules = `# dmidecode 3.5
Getting SMBIOS data from sysfs.
SMBIOS 3.0.0 present.

Handle 0x0032, DMI type 16, 23 bytes
Physical Memory Array
	Location: System Board Or Motherboard
	Use: System Memory
	Error Correction Type: Multi-bit ECC
	Maximum Capacity: 64 GB
	Error Information Handle: Not Provided
	Number Of Devices: 2

Handle 0x0033, DMI type 17, 40 bytes
Memory Device
	Array Handle: 0x0032
	Error Information Handle: Not Provided
	Total Width: 72 bits
	Data Width: 64 bits
	Size: 32 GB
	Form Factor: DIMM
	Set: None
	Locator: DIMM_A1
	Bank Locator: NODE 1
	Type: DDR4
	Type Detail: Synchronous
	Speed: 2666 MT/s
	Manufacturer: Hynix
	Serial Number: N/A
	Asset Tag: NONE
	Part Number: HMAA8GU7AJR8N
	Rank: 2
	Configured Memory Speed: 2666 MT/s
	Minimum Voltage: 1.2 V
	Maximum Voltage: 1.2 V
	Configured Voltage: 1.2 V

Handle 0x0034, DMI type 17, 40 bytes
Memory Device
	Array Handle: 0x0032
	Error Information Handle: Not Provided
	Total Width: 72 bits
	Data Width: 64 bits
	Size: 32 GB
	Form Factor: DIMM
	Set: None
	Locator: DIMM_B1
	Bank Locator: NODE 1
	Type: DDR4
	Type Detail: Synchronous
	Speed: 2666 MT/s
	Manufacturer: Hynix
	Serial Number: N/A
	Asset Tag: NONE
	Part Number: HMAA8GU7AJR8N
	Rank: 2
	Configured Memory Speed: 2666 MT/s
	Minimum Voltage: 1.2 V
	Maximum Voltage: 1.2 V
	Configured Voltage: 1.2 V

Handle 0x0035, DMI type 17, 40 bytes
Memory Device
	Array Handle: 0x0032
	Error Information Handle: Not Provided
	Size: No Module Installed
	Type: Unknown
	Type Detail: Other
	Speed: Unknown
`

func TestParseDMIDecodeMemoryTwoModules(t *testing.T) {
	got := parseDMIDecodeMemory(dmidecodeTwoModules)
	want := dmidecodeMemory{Type: "DDR4", FreqMHz: 2666, Channels: 2}
	if got != want {
		t.Fatalf("parseDMIDecodeMemory = %+v, want %+v", got, want)
	}
}

func TestParseDMIDecodeMemoryEmptySlotsAndUnpopulated(t *testing.T) {
	// Half-populated board: one real LPDDR5 module plus empty slots whose
	// "Type: Unknown" and "Speed: Unknown" must not win.
	text := `Handle 0x0005, DMI type 17, 40 bytes
Memory Device
	Size: 16 GB
	Type: LPDDR5
	Speed: 5500 MT/s
	Configured Memory Speed: 5200 MT/s

Handle 0x0006, DMI type 17, 40 bytes
Memory Device
	Size: No Module Installed
	Type: Unknown
	Speed: Unknown

Handle 0x0007, DMI type 17, 40 bytes
Memory Device
	Size: No Module Installed
	Type: Other
	Speed: 0 MT/s
`
	got := parseDMIDecodeMemory(text)
	want := dmidecodeMemory{Type: "LPDDR5", FreqMHz: 5500, Channels: 1}
	if got != want {
		t.Fatalf("parseDMIDecodeMemory = %+v, want %+v", got, want)
	}
}

func TestParseDMIDecodeMemoryGarbage(t *testing.T) {
	cases := []struct {
		name string
		text string
		want dmidecodeMemory
	}{
		{"empty", "", dmidecodeMemory{}},
		{"no devices", "Physical Memory Array\n\tUse: System Memory\n", dmidecodeMemory{}},
		{"bad speed keeps module", "Memory Device\n\tSize: 8 GB\n\tType: DDR3\n\tSpeed: bananas\n", dmidecodeMemory{Type: "DDR3", Channels: 1}},
		{"unknown speed", "Memory Device\n\tSize: 8 GB\n\tType: DDR3\n\tSpeed: Unknown\n", dmidecodeMemory{Type: "DDR3", Channels: 1}},
		{"orphan fields outside device", "\tSize: 8 GB\n\tType: DDR4\n", dmidecodeMemory{}},
	}
	for _, tc := range cases {
		if got := parseDMIDecodeMemory(tc.text); got != tc.want {
			t.Fatalf("%s: parseDMIDecodeMemory = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
