package dmmdata

import (
	"fmt"
	"os"
	"sort"

	"sdmm/internal/util"
)

type (
	DataDictionary map[Key]Prefabs
	DataGrid       map[util.Point]Key
)

// DmmData stores raw information about the map. Mostly needed to for parsing and saving.
type DmmData struct {
	Filepath string

	IsTgm     bool
	LineBreak string

	KeyLength        int
	MaxX, MaxY, MaxZ int

	Dictionary DataDictionary
	Grid       DataGrid
}

// APHELION EDIT CHANGE - ATOMIC_SAVE - ORIGINAL: func (d DmmData) Save()
func (d DmmData) Save() error {
	if d.IsTgm {
		return d.SaveTGM(d.Filepath)
	}
	return d.SaveDM(d.Filepath)
}

func (d DmmData) Keys() []Key {
	keys := make([]Key, 0, len(d.Dictionary))
	for key := range d.Dictionary {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		return keys[i].ToNum() < keys[j].ToNum()
	})

	return keys
}

func (d DmmData) String() string {
	var winLineBreak bool
	if d.LineBreak == "\r\n" {
		winLineBreak = true
	}
	return fmt.Sprintf(
		"Filepath: %s, IsTgm: %t, WinLineBreak: %v, KeyLength: %d, MaxX: %d, MaxY: %d, MaxZ: %d",
		d.Filepath, d.IsTgm, winLineBreak, d.KeyLength, d.MaxX, d.MaxY, d.MaxZ)
}

func New(path string) (*DmmData, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// APHELION EDIT CHANGE - STATIC_ANALYSIS - ORIGINAL: defer file.Close()
	defer func() { _ = file.Close() }()
	return parse(file)
}
