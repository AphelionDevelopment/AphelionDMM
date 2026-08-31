//go:build windows

package server

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func secretFilePermissionsAllowed(filePath string, _ os.FileMode) (bool, error) {
	descriptor, err := windows.GetNamedSecurityInfo(
		filePath,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return false, fmt.Errorf("read owner and DACL: %w", err)
	}
	if descriptor == nil {
		return false, fmt.Errorf("security descriptor is unavailable")
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return false, fmt.Errorf("read owner SID: %w", err)
	}
	if owner == nil || !owner.IsValid() {
		return false, fmt.Errorf("owner SID is unavailable or invalid")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return false, fmt.Errorf("read DACL: %w", err)
	}
	if dacl == nil {
		return false, fmt.Errorf("null DACL is forbidden")
	}
	system, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		return false, fmt.Errorf("resolve LocalSystem SID: %w", err)
	}
	administrators, err := windows.StringToSid("S-1-5-32-544")
	if err != nil {
		return false, fmt.Errorf("resolve Administrators SID: %w", err)
	}
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			return false, fmt.Errorf("read DACL ACE %d: %w", index, err)
		}
		if ace == nil {
			return false, fmt.Errorf("DACL ACE %d is unavailable", index)
		}
		switch ace.Header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE:
			continue
		case windows.ACCESS_ALLOWED_ACE_TYPE:
		default:
			return false, fmt.Errorf("DACL ACE %d has unsupported type %d", index, ace.Header.AceType)
		}
		if !accessMaskReadsFile(ace.Mask) {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid == nil || !sid.IsValid() {
			return false, fmt.Errorf("DACL ACE %d has an invalid SID", index)
		}
		if !sid.Equals(owner) && !sid.Equals(system) && !sid.Equals(administrators) {
			return false, nil
		}
	}
	return true, nil
}

func accessMaskReadsFile(mask windows.ACCESS_MASK) bool {
	if mask&windows.GENERIC_ALL != 0 {
		return true
	}
	if mask&windows.GENERIC_READ != 0 {
		mask = mask&^windows.GENERIC_READ | windows.FILE_GENERIC_READ
	}
	return mask&windows.FILE_READ_DATA != 0
}
