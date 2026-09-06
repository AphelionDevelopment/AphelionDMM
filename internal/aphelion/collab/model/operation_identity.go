package model

import (
	"reflect"
	"sort"
)

// SameOperation compares submitted intent independent of tile ordering and
// empty collection representations. All identity and precondition fields count.
func SameOperation(left, right Operation) bool {
	left, right = CloneOperation(left), CloneOperation(right)
	leftChanges, rightChanges := left.Changes, right.Changes
	left.Changes, right.Changes = nil, nil
	if !reflect.DeepEqual(left, right) || len(leftChanges) != len(rightChanges) {
		return false
	}
	for _, changes := range [][]TileChange{leftChanges, rightChanges} {
		sort.Slice(changes, func(i, j int) bool {
			a, b := changes[i].Coord, changes[j].Coord
			if a.Z != b.Z {
				return a.Z < b.Z
			}
			if a.Y != b.Y {
				return a.Y < b.Y
			}
			return a.X < b.X
		})
	}
	for i, change := range leftChanges {
		other := rightChanges[i]
		if change.Coord != other.Coord || !change.Before.Equal(other.Before) || !change.After.Equal(other.After) {
			return false
		}
	}
	return true
}
