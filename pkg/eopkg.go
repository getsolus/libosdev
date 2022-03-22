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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/getsolus/libosdev/commands"
	"github.com/getsolus/libosdev/disk"
)

const (
	// EopkgCacheDirectory is where we'll bind mount to provide package caching
	// to speed up subsequent image builds.
	// This will be mounted at $rootfs/var/cache/eopkg/packages.
	// It uses the evobuild directory for consistency with evobuild, so that
	// Solus developers only need one cache system wide.
	EopkgCacheDirectory = "/var/lib/evobuild/packages"
)

// EopkgManager is used to apply operations with the eopkg package manager
// for Solus systems.
type EopkgManager struct {
	root        string // rootfs path
	cacheTarget string // Where we mount the cache directory

	targetMode bool // Whether we're in target mode or not.

	dbusActive bool // Whether we have dbus alive or not

	cacheSource string // Where we find the cache directory
}

// NewEopkgManager will return a newly initialised EopkgManager
func NewEopkgManager() *EopkgManager {
	return &EopkgManager{targetMode: false, cacheSource: EopkgCacheDirectory}
}

// SetCacheDirectory is used to override the system cache directory
func (e *EopkgManager) SetCacheDirectory(source string) {
	e.cacheSource = source
}

// Init will check that eopkg is available host side
func (e *EopkgManager) Init() error {
	// Ensure the system has eopkg available first!
	if _, err := exec.LookPath("eopkg"); err != nil {
		return err
	}
	return nil
}

// InitRoot will set up the filesystem root in accordance with eopkg needs
func (e *EopkgManager) InitRoot(root string) error {
	e.root = root
	e.targetMode = true

	// Ensures we don't end up with /var/lock vs /run/lock nonsense
	reqDirs := []string{
		"run/lock",
		"var",
		// Enables our bind mounting for caching
		"var/cache/eopkg/packages",
	}

	// Construct the required directories in the tree
	for _, dir := range reqDirs {
		dirPath := filepath.Join(root, dir)
		if err := os.MkdirAll(dirPath, 00755); err != nil {
			return err
		}
	}

	// Attempt to create the system wide cache directory
	if err := os.MkdirAll(e.cacheSource, 00755); err != nil {
		return err
	}

	if err := os.Symlink("../run/lock", filepath.Join(root, "var", "lock")); err != nil {
		return err
	}
	if err := os.Symlink("../run", filepath.Join(root, "var", "run")); err != nil {
		return err
	}

	// Now attempt to bind mount the cache directory to be .. well. usable
	e.cacheTarget = filepath.Join(root, "var", "cache", "eopkg", "packages")
	if err := disk.GetMountManager().BindMount(e.cacheSource, e.cacheTarget); err != nil {
		return err
	}

	return nil
}

// FinalizeRoot will configure all of the eopkgs installed in the system, and
// ensure that dbus, etc, works.
func (e *EopkgManager) FinalizeRoot() error {
	// First things first, unmount the cache
	if err := disk.GetMountManager().Unmount(e.cacheTarget); err != nil {
		return err
	}
	// Copy base layout
	if err := e.copyBaselayout(); err != nil {
		return err
	}
	// Before we start chrooting, update libraries to be usable..
	if err := commands.ChrootExec(e.root, "ldconfig"); err != nil {
		return err
	}
	// Set up account for dbus (TODO: Add sysusers.d file for this
	if err := e.configureDbus(); err != nil {
		return err
	}
	// Create the required nodes for eopkg to run without bind mounts
	if err := disk.CreateDeviceNode(e.root, disk.DevNodeRandom); err != nil {
		return err
	}
	if err := disk.CreateDeviceNode(e.root, disk.DevNodeURandom); err != nil {
		return err
	}
	// Start dbus to allow configure-pending
	if err := e.startDBUS(); err != nil {
		return err
	}
	// Run all postinstalls inside chroot
	if err := commands.ChrootExec(e.root, "eopkg configure-pending"); err != nil {
		e.killDBUS()
		return err
	}
	// Buhbye dbus
	if err := e.killDBUS(); err != nil {
		return err
	}
	// Delete cached assets
	if err := commands.ChrootExec(e.root, "eopkg delete-cache"); err != nil {
		return err
	}
	return nil
}

// This needs to die in a fire and will not be supported when sol replaces eopkg
func (e *EopkgManager) copyBaselayout() error {
	var files []os.FileInfo
	var err error

	// elements of /usr/share/baselayout are copied to /etc/ - ANTI STATELESS
	baseDir := filepath.Join(e.root, "usr", "share", "baselayout")
	tgtDir := filepath.Join(e.root, "etc")
	if files, err = ioutil.ReadDir(baseDir); err != nil {
		return err
	}

	for _, file := range files {
		srcPath := filepath.Join(baseDir, file.Name())
		tgtPath := filepath.Join(tgtDir, file.Name())

		if err = disk.CopyFile(srcPath, tgtPath); err != nil {
			return err
		}
	}
	return nil
}

// Attempt to start dbus in the root..
func (e *EopkgManager) startDBUS() error {
	if e.dbusActive {
		return nil
	}
	if err := commands.ChrootExec(e.root, "dbus-uuidgen --ensure"); err != nil {
		return err
	}
	if err := commands.ChrootExec(e.root, "dbus-daemon --system"); err != nil {
		return err
	}
	e.dbusActive = true
	return nil
}

// killDBUS will stop dbus again
func (e *EopkgManager) killDBUS() error {
	// No sense killing dbus twice
	if !e.dbusActive {
		return nil
	}
	fpath := filepath.Join(e.root, "var/run/dbus/pid")
	var b []byte
	var err error
	var f *os.File

	if f, err = os.Open(fpath); err != nil {
		return err
	}
	defer func() {
		f.Close()
		os.Remove(fpath)
		e.dbusActive = false
	}()

	if b, err = ioutil.ReadAll(f); err != nil {
		return err
	}

	pid := strings.Split(string(b), "\n")[0]
	return commands.ExecStdoutArgs("kill", []string{"-9", pid})
}

// This is also largely anti-stateless but is required just to get dbus running
// so we can configure-pending. sol can't come quick enough...
func (e *EopkgManager) configureDbus() error {
	if err := commands.AddGroup(e.root, "messagebus", 18); err != nil {
		return err
	}
	if err := commands.AddSystemUser(e.root, "messagebus", "D-Bus Message Daemon", "/var/run/dbus", "/bin/false", 18, 18); err != nil {
		return err
	}
	return nil
}

// Cleanup will cleanup the rootfs at any given point
func (e *EopkgManager) Cleanup() error {
	return e.killDBUS()
}

// Eopkg specific functions

func (e *EopkgManager) eopkgExecRoot(args []string) error {
	if !e.targetMode {
		return commands.ExecStdoutArgs("eopkg", args)
	}
	endArgs := []string{
		"-D", e.root,
	}
	args = append(args, endArgs...)
	return commands.ExecStdoutArgs("eopkg", args)
}

// AddRepo will add the new eopkg repo to the target
func (e *EopkgManager) AddRepo(identifier, uri string) error {
	return e.eopkgExecRoot([]string{"add-repo", identifier, uri})
}

// InstallGroups will install the named eopkg components to the target
func (e *EopkgManager) InstallGroups(ignoreSafety bool, groups []string) error {
	var componentNames []string
	for _, comp := range groups {
		componentNames = append(componentNames, []string{
			"-c",
			comp,
		}...)
	}
	cmd := []string{"install", "-y"}
	if e.targetMode {
		cmd = append(cmd, "--ignore-comar")
	}
	cmd = append(cmd, componentNames...)
	if ignoreSafety {
		cmd = append(cmd, "--ignore-safety")
	}
	return e.eopkgExecRoot(cmd)
}

// InstallPackages will install the named eopkgs to the target
func (e *EopkgManager) InstallPackages(ignoreSafety bool, packages []string) error {
	cmd := []string{"install", "-y"}
	if e.targetMode {
		cmd = append(cmd, "--ignore-comar")
	}
	cmd = append(cmd, packages...)
	if ignoreSafety {
		cmd = append(cmd, "--ignore-safety")
	}
	return e.eopkgExecRoot(cmd)
}
