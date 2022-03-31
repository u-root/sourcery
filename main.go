// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.18
// +build go1.18

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hashicorp/go-multierror"
)

var (
	version = "go1.17.7"
	V       = log.Printf
)

func clone(d, v, r string) error {
	cmd := []string{"clone", "--depth", "1"}
	if len(v) > 0 {
		cmd = append(cmd, "-b", v)
	}
	cmd = append(cmd, r)
	c := exec.Command("git", cmd...)
	c.Dir = d
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return err
	}
	return nil
}

func tidy(d, r string) error {
	c := exec.Command("go", "mod", "tidy")
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Env = append(c.Env, "GOPATH="+d)
	c.Dir = filepath.Join(d, filepath.Base(r))
	V("Run %v(%q, %q in %q)", c, c.Args, c.Env, c.Dir)
	if err := c.Run(); err != nil {
		return err
	}
	return nil
}

func getgo(d, v string) error {
	c := exec.Command("git", "clone", "-b", version, "--depth", "1", "git@github.com:golang/go")
	c.Dir = d
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return err
	}
	// simply sanity check
	gover := filepath.Join(d, "go", "VERSION")
	dat, err := ioutil.ReadFile(gover)
	if err != nil {
		return fmt.Errorf("Reading %q: %v", gover, err)
	}
	if string(dat) != version {
		return fmt.Errorf("Version file has %q, but want version %q", string(dat), version)
	}
	return nil
}

func get(target string, args ...string) error {
	var err error
	for _, d := range args {
		if e := clone(target, "", d); err != nil {
			err = multierror.Append(err, e)
			continue
		}
		if e := tidy(target, d); e != nil {
			err = multierror.Append(err, e)
			continue
		}
	}
	return err
}

func main() {
	flag.Parse()

	// Build the target directory
	// Start with a temporary directory
	// copy the toolchain there
	// copy the rest of the other directories there

	d, err := os.MkdirTemp("", "sourcery")
	defer fmt.Printf("Tree is %q\n", d)
	if err != nil {
		log.Fatal(err)
	}
	if err := getgo(d, version); err != nil {
		log.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(d, "src"), 0755); err != nil {
		log.Fatal(err)
	}
	if err := get(filepath.Join(d, "src"), flag.Args()...); err != nil {
		log.Fatalf("Getting packages: %v", err)
	}
}
