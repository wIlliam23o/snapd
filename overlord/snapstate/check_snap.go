// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2016 Canonical Ltd
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

package snapstate

import (
	"fmt"
	"strings"

	"github.com/snapcore/snapd/arch"
	"github.com/snapcore/snapd/overlord/snapstate/backend"
	"github.com/snapcore/snapd/overlord/state"
	"github.com/snapcore/snapd/release"
	"github.com/snapcore/snapd/snap"
)

// featureSet contains the flag values that can be listed in assumes entries
// that this ubuntu-core actually provides.
var featureSet = map[string]bool{
	// Support for common data directory across revisions of a snap.
	"common-data-dir": true,
	// Support for the "Environment:" feature in snap.yaml
	"snap-env": true,
}

func checkAssumes(s *snap.Info) error {
	missing := ([]string)(nil)
	for _, flag := range s.Assumes {
		if !featureSet[flag] {
			missing = append(missing, flag)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("snap %q assumes unsupported features: %s (try new ubuntu-core)", s.Name(), strings.Join(missing, ", "))
	}
	return nil
}

var openSnapFile = backend.OpenSnapFile

// checkSnap ensures that the snap can be installed.
func checkSnap(st *state.State, snapFilePath string, si *snap.SideInfo, curInfo *snap.Info, flags Flags) error {
	// This assumes that the snap was already verified or --dangerous was used.

	s, _, err := openSnapFile(snapFilePath, si)
	if err != nil {
		return err
	}

	if s.NeedsDevMode() && !flags.DevModeAllowed() {
		return fmt.Errorf("snap %q requires devmode or confinement override", s.Name())
	}

	// verify we have a valid architecture
	if !arch.IsSupportedArchitecture(s.Architectures) {
		return fmt.Errorf("snap %q supported architectures (%s) are incompatible with this system (%s)", s.Name(), strings.Join(s.Architectures, ", "), arch.UbuntuArchitecture())
	}

	// check assumes
	err = checkAssumes(s)
	if err != nil {
		return err
	}

	st.Lock()
	defer st.Unlock()

	if CheckInterfaces != nil {
		if err := CheckInterfaces(st, s); err != nil {
			return err
		}
	}

	switch s.Type {
	case snap.Type(""), snap.TypeApp, snap.TypeKernel:
		// "" used in a lot of tests :-/
		return nil
	case snap.TypeOS:
		if curInfo != nil {
			// already one of these installed
			return nil
		}
		core, err := CoreInfo(st)
		if err == state.ErrNoState {
			return nil
		}
		if err != nil {
			return err
		}
		if core.Name() != s.Name() {
			return fmt.Errorf("cannot install core snap %q when core snap %q is already present", s.Name(), core.Name())
		}

		return nil
	case snap.TypeGadget:
		// gadget specific checks
		if release.OnClassic {
			// for the time being
			return fmt.Errorf("cannot install a gadget snap on classic")
		}

		currentGadget, err := GadgetInfo(st)
		// FIXME: check from the model assertion that its the
		//        right gadget
		//
		// in firstboot we have no gadget yet - that is ok
		if err == state.ErrNoState {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot find original gadget snap")
		}

		// TODO: actually compare snap ids, from current gadget and candidate
		if currentGadget.Name() != s.Name() {
			return fmt.Errorf("cannot replace gadget snap with a different one")
		}

		return nil
	}

	panic("unknown type")
}

// CheckInterfaces allows to hook into snap checking to verify interfaces.
var CheckInterfaces func(s *state.State, snap *snap.Info) error