// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

func tree(args ...string) error {
	d, err := os.MkdirTemp("", "sourcery")
	if err != nil {
		log.Fatal(err)
	}
	opts := cp.Options{
		NoFollowSymlinks: f.noFollowSymlinks,
	}

	var err error
	for _, file := range args {
		if e := opts.CopyTree(file, d); e != nil {
			err = multierror.Append(err, e)
		}
	}
	return err
}
func main() {
	flag.Parse()

	a := flag.Args()
	// Build the target directory
	// Start with a temporary directory
	// copy the toolchain there
	// copy the rest of the other directories there

	tree, err := tree(args...)
	if err != nil {
		log.Fatal(err)
	}
}
