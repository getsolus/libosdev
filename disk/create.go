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

package disk

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/getsolus/libosdev/commands"
)

// CreateSparseFile will create a new sparse file with the given filename and
// size in nMegabytes/
//
// This is highly dependent on the underlying filesystem at the directory
// where the file is to be created, making use of the syscall ftruncate.
func CreateSparseFile(filename string, nMegabytes int) error {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 00644)
	if err != nil {
		return err
	}
	defer f.Close()
	// NOTE: New megabytes, not old megabytes (1000, not 1024)
	sz := int64(nMegabytes * 1000 * 1000)
	if err = syscall.Ftruncate(int(f.Fd()), sz); err != nil {
		return err
	}
	return nil
}

// GetSquashfsArgs returns the compression arg set for a given compression type
func GetSquashfsArgs(compressionType CompressionType) ([]string, error) {
	switch compressionType {
	case CompressionGzip:
		return []string{"-comp", "gzip"}, nil
	case CompressionXZ:
		return []string{"-comp", "xz"}, nil
	default:
		return nil, fmt.Errorf("Unknown compression type: %v", compressionType)
	}
}

// CreateSquashfs will create a new squashfs filesystem image at the given outputFile path,
// containing the tree found at path, using compressionType (gzip or xz).
func CreateSquashfs(path, outputFile string, compressionType CompressionType) error {
	command := []string{
		path,
		outputFile,
	}
	dirName := ""
	if fp, err := filepath.Abs(path); err == nil {
		dirName = filepath.Dir(fp)
	} else {
		return err
	}

	// May have to set -keep-as-directory
	if st, err := os.Stat(path); err == nil {
		if st.Mode().IsDir() {
			command = append(command, "-keep-as-directory")
		}
	} else {
		return err
	}
	// TODO: Add -processors nCPU (4)
	if execArgs, err := GetSquashfsArgs(compressionType); err == nil {
		command = append(command, execArgs...)
	} else {
		return err
	}
	return commands.ExecStdoutArgsDir(dirName, "mksquashfs", command)
}
