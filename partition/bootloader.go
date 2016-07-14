// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2015 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package partition

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/snapcore/snapd/osutil"
)

const (
	// bootloader variable used to determine if boot was successful.
	// Set to value of either bootloaderBootmodeTry (when attempting
	// to boot a new rootfs) or bootloaderBootmodeSuccess (to denote
	// that the boot of the new rootfs was successful).
	bootmodeVar  = "snappy_mode"
	trialBootVar = "snappy_trial_boot"

	// Initial and final values
	modeTry     = "try"
	modeSuccess = "regular"
)

var (
	// ErrBootloader is returned if the bootloader can not be determined
	ErrBootloader = errors.New("cannot determine bootloader")
)

// Bootloader provides an interface to interact with the system
// bootloader
type Bootloader interface {
	// Return the value of the specified bootloader variable
	GetBootVar(name string) (string, error)

	// Set the value of the specified bootloader variable
	SetBootVar(name, value string) error

	// Dir returns the bootloader directory
	Dir() string

	// Name returns the bootloader name
	Name() string

	// configFile returns the name of the config file
	ConfigFile() string
}

// InstallBootConfig install the bootloader config from the gadget
// snap dir into the right place
func InstallBootConfig(gadgetDir string) error {
	for _, bl := range []Bootloader{&grub{}, &uboot{}} {
		fn, err := find(gadgetDir, filepath.Base(bl.ConfigFile()))
		if err != nil {
			continue
		}

		dst := bl.ConfigFile()
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		return osutil.CopyFile(fn, dst, osutil.CopyFlagOverwrite)
	}

	return fmt.Errorf("cannot find boot config in %q", gadgetDir)
}

var forcedBootloader Bootloader

// FindBootloader returns the bootloader for the given system
// or an error if no bootloader is found
func FindBootloader() (Bootloader, error) {
	if forcedBootloader != nil {
		return forcedBootloader, nil
	}

	// try uboot
	if uboot := newUboot(); uboot != nil {
		return uboot, nil
	}

	// no, try grub
	if grub := newGrub(); grub != nil {
		return grub, nil
	}

	// no, weeeee
	return nil, ErrBootloader
}

// ForceBootloader can be used to force setting a booloader to that FindBootloader will not use the usual lookup process, use nil to reset to normal lookup.
func ForceBootloader(booloader Bootloader) {
	forcedBootloader = booloader
}

// MarkBootSuccessful marks the current boot as sucessful. This means
// that snappy will consider this combination of kernel/os a valid
// target for rollback
func MarkBootSuccessful(bootloader Bootloader) error {
	// FIXME: we should have something better here, i.e. one write
	//        to the bootloader environment only (instead of three)
	//        We need to figure out if that is possible with grub/uboot
	// If we could also do atomic writes to the boot env, that would
	// be even better. The complication here is that the grub
	// environment is handled via grub-editenv and uboot is done
	// via the special uboot.env file on a vfat partition.
	for _, k := range []string{"snappy_os", "snappy_kernel"} {
		value, err := bootloader.GetBootVar(k)
		if err != nil {
			return err
		}

		// FIXME: ugly string replace
		newKey := strings.Replace(k, "snappy_", "snappy_good_", -1)
		if err := bootloader.SetBootVar(newKey, value); err != nil {
			return err
		}

		if err := bootloader.SetBootVar("snappy_mode", modeSuccess); err != nil {
			return err
		}

		if err := bootloader.SetBootVar("snappy_trial_boot", "0"); err != nil {
			return err
		}
	}

	return nil
}
