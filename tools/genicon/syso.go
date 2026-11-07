package main

// The Windows resources of the executable, written as COFF object files
// (.syso) that go build links in when it builds for Windows: the icon, which
// Explorer and the taskbar show for the .exe, and the manifest, which asks
// for the visual style of Windows' own controls in the settings window and
// for drawing at each monitor's own scale.
//
// A .syso holds one section, .rsrc, laid out as the resource section of an
// executable: a three-level directory (type, ID, language) whose leaves point
// at the data. The leaves hold addresses relative to the image, which only
// the linker knows, so each comes with a relocation against the section.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// manifest is the application manifest.
const manifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity type="win32" name="io.github.aki-kuramoto.claude-usage-visualizer" version="1.0.0.0"/>
  <dependency>
    <dependentAssembly>
      <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
    </dependentAssembly>
  </dependency>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2</dpiAwareness>
    </windowsSettings>
  </application>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
    </application>
  </compatibility>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
</assembly>
`

const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtManifest  = 24
	langEnUS    = 0x0409
)

// machines are the architectures to write a .syso for, with each one's COFF
// machine type and the type of a relocation to an address relative to the
// image (ADDR32NB).
var machines = []struct {
	arch    string
	machine uint16
	reloc   uint16
}{
	{"amd64", 0x8664, 0x0003},
	{"arm64", 0xAA64, 0x0002},
	{"386", 0x014C, 0x0007},
}

// writeSysos writes rsrc_windows_<arch>.syso into dir.
func writeSysos(dir string, pngs map[int][]byte, sizes []int) error {
	res := resources(pngs, sizes)
	for _, m := range machines {
		data := coff(m.machine, m.reloc, res)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("rsrc_windows_%s.syso", m.arch)), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// resource is one leaf of the resource directory.
type resource struct {
	typ, id uint32
	data    []byte
}

// resources lists the icon images (RT_ICON 1, 2, ...), the icon group that
// names them (RT_GROUP_ICON 1, the icon Explorer shows) and the manifest
// (RT_MANIFEST 1, which Windows reads at start).
func resources(pngs map[int][]byte, sizes []int) []resource {
	var res []resource
	var group bytes.Buffer
	binary.Write(&group, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	for i, n := range sizes {
		dim := uint8(n)
		if n >= 256 {
			dim = 0 // 0 means 256
		}
		binary.Write(&group, binary.LittleEndian, struct {
			W, H, Colors, Reserved uint8
			Planes, Bits           uint16
			Size                   uint32
			ID                     uint16
		}{dim, dim, 0, 0, 1, 32, uint32(len(pngs[n])), uint16(i + 1)})
		res = append(res, resource{rtIcon, uint32(i + 1), pngs[n]})
	}
	res = append(res, resource{rtGroupIcon, 1, group.Bytes()})
	res = append(res, resource{rtManifest, 1, []byte(manifest)})
	return res
}

// section lays the resources out as a .rsrc section. It returns the section
// and the offsets of the fields that need a relocation.
func section(res []resource) (sec []byte, relocs []uint32) {
	// Group by type, in ascending order as Windows expects.
	byType := map[uint32][]resource{}
	var types []uint32
	for _, r := range res {
		if _, ok := byType[r.typ]; !ok {
			types = append(types, r.typ)
		}
		byType[r.typ] = append(byType[r.typ], r)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	for _, t := range types {
		rs := byType[t]
		sort.Slice(rs, func(i, j int) bool { return rs[i].id < rs[j].id })
	}

	const dirSize, entrySize, leafSize = 16, 8, 16
	// Sizes of each part, to know the offsets before writing.
	root := dirSize + entrySize*len(types)
	idDirs := 0
	for _, t := range types {
		idDirs += dirSize + entrySize*len(byType[t])
	}
	langDirs := len(res) * (dirSize + entrySize)
	leaves := len(res) * leafSize

	idDirAt := root
	langDirAt := root + idDirs
	leafAt := langDirAt + langDirs
	dataAt := leafAt + leaves

	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	dir := func(n int) {
		w(struct {
			Chars, Time              uint32
			Major, Minor, Named, IDs uint16
		}{0, 0, 0, 0, 0, uint16(n)})
	}
	const subdir = 0x80000000

	// Root: one entry per type, pointing at the type's ID directory.
	dir(len(types))
	at := idDirAt
	for _, t := range types {
		w([2]uint32{t, subdir | uint32(at)})
		at += dirSize + entrySize*len(byType[t])
	}
	// ID directories: one entry per resource, pointing at its language
	// directory.
	k := 0
	for _, t := range types {
		dir(len(byType[t]))
		for _, r := range byType[t] {
			w([2]uint32{r.id, subdir | uint32(langDirAt+k*(dirSize+entrySize))})
			k++
		}
	}
	// Language directories: one entry each, pointing at the leaf.
	for i := range res {
		dir(1)
		w([2]uint32{langEnUS, uint32(leafAt + i*leafSize)})
	}
	// Leaves: where the data is and how long. Data is aligned to 8 bytes.
	var data bytes.Buffer
	ordered := make([]resource, 0, len(res))
	for _, t := range types {
		ordered = append(ordered, byType[t]...)
	}
	for _, r := range ordered {
		for data.Len()%8 != 0 {
			data.WriteByte(0)
		}
		relocs = append(relocs, uint32(b.Len()))
		w([4]uint32{uint32(dataAt + data.Len()), uint32(len(r.data)), 0, 0})
		data.Write(r.data)
	}
	b.Write(data.Bytes())
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
	return b.Bytes(), relocs
}

// coff wraps the resources in a COFF object with one section, .rsrc, and the
// symbol of that section, which the relocations refer to.
func coff(machine, relocType uint16, res []resource) []byte {
	sec, relocs := section(res)
	const fileHeader, sectionHeader, relocSize = 20, 40, 10
	secAt := fileHeader + sectionHeader
	relocAt := secAt + len(sec)
	symAt := relocAt + relocSize*len(relocs)

	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	w(struct {
		Machine         uint16
		Sections        uint16
		Time            uint32
		SymbolTable     uint32
		Symbols         uint32
		OptionalHeader  uint16
		Characteristics uint16
	}{machine, 1, 0, uint32(symAt), 1, 0, 0})
	w(struct {
		Name            [8]byte
		VirtualSize     uint32
		VirtualAddress  uint32
		RawSize         uint32
		RawAt           uint32
		RelocAt         uint32
		LineNumbersAt   uint32
		Relocs          uint16
		LineNumbers     uint16
		Characteristics uint32
	}{[8]byte{'.', 'r', 's', 'r', 'c'}, 0, 0, uint32(len(sec)), uint32(secAt), uint32(relocAt), 0,
		uint16(len(relocs)), 0, 0x40000040}) // initialized data, readable
	b.Write(sec)
	for _, r := range relocs {
		w(struct {
			At     uint32
			Symbol uint32
			Type   uint16
		}{r, 0, relocType})
	}
	// The section's symbol, then an empty string table (its size, 4).
	w(struct {
		Name    [8]byte
		Value   uint32
		Section int16
		Type    uint16
		Class   uint8
		Aux     uint8
	}{[8]byte{'.', 'r', 's', 'r', 'c'}, 0, 1, 0, 3, 0}) // IMAGE_SYM_CLASS_STATIC
	w(uint32(4))
	return b.Bytes()
}
