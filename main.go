// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.18
// +build go1.18

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-multierror"
	"github.com/u-root/uio/cp"
)

var (
	home string
	V    = log.Printf
)

func tree(gotoolchain string, args ...string) (string, error) {
	d, err := os.MkdirTemp("", "sourcery")
	if err != nil {
		return "", err
	}
	opts := cp.Options{
		NoFollowSymlinks: true,
	}
	err = nil
	for i, dir := range append([]string{gotoolchain}, args...) {
		b := "go"
		if i > 0 {
			b = filepath.Join("src", filepath.Base(dir))
		}
		if !filepath.HasPrefix(dir, home) {
			err = multierror.Append(err, fmt.Errorf("%q does not have %q as a prefix", dir, home))
		}
		b = filepath.Join(d, b)
		V("Copy %q -> %q", dir, b)
		if e := opts.CopyTree(dir, b); e != nil {
			err = multierror.Append(err, e)
		}
	}
	return d, err
}

func env() error {
	var (
		ok  bool
		err error
	)
	home, ok = os.LookupEnv("HOME")
	if !ok {
		err = multierror.Append(fmt.Errorf("$HOME is not set"))
	}
	return err
}

func main() {
	flag.Parse()

	if err := env(); err != nil {
		log.Fatal(err)
	}
	a := flag.Args()
	if len(a) == 0 {
		log.Fatal("Usage: sourcery go-toolchain [args...]")
	}
	// Build the target directory
	// Start with a temporary directory
	// copy the toolchain there
	// copy the rest of the other directories there

	tree, err := tree(a[0], a[1:]...)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("The tree is in %q", tree)
}
