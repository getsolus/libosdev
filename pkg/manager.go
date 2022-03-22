//
// Copyright © 2016 Ikey Doherty <ikey@solus-project.com>
// Copyright © 2018-2022 Solus Project <copyright@getsol.us>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package pkg

import (
	"errors"
)

// Manager is the interface that should be implemented by vendors to enable
// USpin to understand them and construct images according to their particulars.
type Manager interface {

	// Init will allow implementations to initialise themselves assuming that
	// host-side dependencies need to be met.
	Init() error

	// InitRoot implementations should set up the root filesystem to handle any
	// quirks prior to installing packages. This also allows manipulating the
	// filesystem layout, i.e. for usr-merge situations, or for working around
	// default directories created by a host-side package manager tool.
	InitRoot(root string) error

	// FinalizeRoot should be invoked once all packaging operations have been applied,
	// allowing any post configuration, etc, to take place.
	FinalizeRoot() error

	// InstallPackages will ask the package manager implementation to install the
	// given package set.
	// ignoreSafety is dependent on the package manager, but is usually used to
	// avoid automatic dependencies such as system.base in Solus, or recommends
	// in dpkg.
	InstallPackages(ignoreSafety bool, packages []string) error

	// InstallGroups will ask the package manager implementation to install the
	// given groups (components in some distros)
	// ignoreSafety is dependent on the package manager, but is usually used to
	// avoid automatic dependencies such as system.base in Solus, or recommends
	// in dpkg.
	InstallGroups(ignoreSafety bool, groups []string) error

	// AddRepo asks the package manager to add the given repository to the system
	AddRepo(identifier, uri string) error

	// Cleanup may be called at any time, and the package manager implementation
	// should ensure it cleans anything it did in the past, such as closing open
	// processes.
	Cleanup() error
}

// NewManager will return an appropriate package manager instance for
// the given name, if it exists.
func NewManager(name PackageManager) (Manager, error) {
	switch name {
	case PackageManagerEopkg:
		return NewEopkgManager(), nil
	default:
		return nil, errors.New("Not yet implemented")
	}
}
