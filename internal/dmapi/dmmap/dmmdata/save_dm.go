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
// SaveDM atomically writes DmmData in DM format to the provided path.
func (d DmmData) SaveDM(path string) error {
	log.Print("saving dmm data in format...")
	if err := SaveAtomic(path, d.WriteDM, func(stagedPath string) error {
		return d.validateSaved(stagedPath, false)
	}); err != nil {
		return fmt.Errorf("save DM map %q: %w", path, err)
	}
	log.Printf("[%s] saved in format to: %s", d, path)
	return nil
}

// WriteDM serializes DmmData in DM format.
func (d DmmData) WriteDM(writer io.Writer) error {
	buffer := bufio.NewWriter(writer)
	write := func(value string) error {
		if _, err := buffer.WriteString(value); err != nil {
			return fmt.Errorf("write DM map: %w", err)
		}
		return nil
	}

	log.Print("writing prefabs...")

	for _, key := range d.Keys() {
		if err := write(toDMStr(key, d.Dictionary[key])); err != nil {
			return err
		}
		if err := write(d.LineBreak); err != nil {
			return err
		}
	}

	log.Print("writing grid...")

	for z := 1; z <= d.MaxZ; z++ {
		if err := write(d.LineBreak); err != nil {
			return err
		}
		if err := write(fmt.Sprintf("(1,1,%d) = {\"", z)); err != nil {
			return err
		}
		if err := write(d.LineBreak); err != nil {
			return err
		}

		for y := d.MaxY; y >= 1; y-- {
			for x := 1; x <= d.MaxX; x++ {
				if err := write(string(d.Grid[util.Point{X: x, Y: y, Z: z}])); err != nil {
					return err
				}
			}
			if err := write(d.LineBreak); err != nil {
				return err
			}
		}

		if err := write("\"}"); err != nil {
			return err
		}
	}

	if err := write(d.LineBreak); err != nil {
		return err
	}
	if err := buffer.Flush(); err != nil {
		return fmt.Errorf("flush DM map: %w", err)
	}
	return nil
}

// APHELION EDIT ADDITION END

func toDMStr(key Key, prefabs Prefabs) string {
	sb := strings.Builder{}

	fmt.Fprintf(&sb, "\"%s\" = (", key)

	for idx, prefab := range prefabs {
		sb.WriteString(prefab.Path())

		if prefab.Vars().Len() > 0 {
			sb.WriteString("{")

			for idx, varName := range prefab.Vars().Iterate() {
				varValue, _ := prefab.Vars().Value(varName)

				sb.WriteString(varName)
				sb.WriteString(" = ")
				sb.WriteString(varValue)

				if idx != prefab.Vars().Len()-1 {
					sb.WriteString("; ")
				}
			}

			sb.WriteString("}")
		}

		if idx != len(prefabs)-1 {
			sb.WriteString(",")
		}
	}

	sb.WriteString(")")

	return sb.String()
}
