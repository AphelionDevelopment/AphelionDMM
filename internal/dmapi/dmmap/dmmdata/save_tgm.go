package dmmdata

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"sdmm/internal/util"

	"github.com/rs/zerolog/log"
)

// APHELION EDIT ADDITION START - ATOMIC_SAVE
// SaveTGM atomically writes DmmData in TGM format to the provided path.
func (d DmmData) SaveTGM(path string) error {
	log.Print("saving dmm data in [TGM] format...")
	if err := SaveAtomic(path, d.WriteTGM, func(stagedPath string) error {
		return d.validateSaved(stagedPath, true)
	}); err != nil {
		return fmt.Errorf("save TGM map %q: %w", path, err)
	}
	log.Printf("[%s] saved in [TGM] format to: %s", d, path)
	return nil
}

// WriteTGM serializes DmmData in TGM format.
func (d DmmData) WriteTGM(writer io.Writer) error {
	buffer := bufio.NewWriter(writer)
	writeln := func(values ...string) error {
		for _, value := range values {
			if _, err := buffer.WriteString(value); err != nil {
				return fmt.Errorf("write TGM map: %w", err)
			}
		}
		if _, err := buffer.WriteString(d.LineBreak); err != nil {
			return fmt.Errorf("write TGM map: %w", err)
		}
		return nil
	}

	// Write TGM header
	// yeah, yeah, dmm2tgm.py, sure...
	if err := writeln("//MAP CONVERTED BY dmm2tgm.py THIS HEADER COMMENT PREVENTS RECONVERSION, DO NOT REMOVE"); err != nil {
		return err
	}

	log.Print("writing prefabs...")

	for _, key := range d.Keys() {
		if err := writeln(toTGMStr(key, d.Dictionary[key], d.LineBreak)); err != nil {
			return err
		}
	}

	log.Print("writing grid...")

	for z := 1; z <= d.MaxZ; z++ {
		if err := writeln(); err != nil {
			return err
		}

		for x := 1; x <= d.MaxX; x++ {
			if err := writeln(fmt.Sprintf("(%d,1,%d) = {\"", x, z)); err != nil {
				return err
			}

			for y := d.MaxY; y >= 1; y-- {
				if err := writeln(string(d.Grid[util.Point{X: x, Y: y, Z: z}])); err != nil {
					return err
				}
			}

			if err := writeln("\"}"); err != nil {
				return err
			}
		}
	}

	if err := buffer.Flush(); err != nil {
		return fmt.Errorf("flush TGM map: %w", err)
	}
	return nil
}

// APHELION EDIT ADDITION END

func toTGMStr(key Key, content Prefabs, lineBreak string) string {
	sb := strings.Builder{}

	fmt.Fprintf(&sb, "\"%s\" = (", key)
	sb.WriteString(lineBreak)

	for idx, prefab := range content {
		sb.WriteString(prefab.Path())

		if prefab.Vars().Len() > 0 {
			sb.WriteString("{")
			sb.WriteString(lineBreak)

			for idx, varName := range prefab.Vars().Iterate() {
				varValue, _ := prefab.Vars().Value(varName)

				sb.WriteString("\t")
				sb.WriteString(varName)
				sb.WriteString(" = ")
				sb.WriteString(varValue)

				if idx != prefab.Vars().Len()-1 {
					sb.WriteString(";")
				}

				sb.WriteString(lineBreak)
			}

			sb.WriteString("\t}")
		}

		if idx != len(content)-1 {
			sb.WriteString(",")
			sb.WriteString(lineBreak)
		}
	}

	sb.WriteString(")")

	return sb.String()
}
