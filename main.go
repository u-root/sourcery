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
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/hashicorp/go-multierror"
	url "github.com/whilp/git-urls"
)

var (
	version = "go1.17.7"
	V       = log.Printf
	arch    = runtime.GOARCH
	kern    = runtime.GOOS
	bin     string
	testrun = true
)

func clone(tmp, version, repo, dir, base string) error {
	V("clone: %q, %q, %q", tmp, version, dir, base)
	dest := filepath.Join(tmp, dir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	cmd := []string{"clone", "--depth", "1"}
	if len(version) > 0 {
		cmd = append(cmd, "-b", version)
	}
	cmd = append(cmd, repo)
	c := exec.Command("git", cmd...)
	c.Dir = dest
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return err
	}
	return nil
}

func tidy(tmp, dir, base string) error {
	c := exec.Command("go", "mod", "tidy")
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Env = append(c.Env, "GOPATH="+tmp)
	c.Dir = filepath.Join(tmp, dir, base)
	V("Run %v(%q, %q in %q)", c, c.Args, c.Env, c.Dir)
	if err := c.Run(); err != nil {
		return err
	}
	return nil
}

func download(tmp, dir, base string) error {
	c := exec.Command("go", "mod", "download")
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Env = append(c.Env, "GOPATH="+tmp)
	c.Dir = filepath.Join(tmp, dir, base)
	V("Run %v(%q, %q in %q)", c, c.Args, c.Env, c.Dir)
	if err := c.Run(); err != nil {
		return err
	}
	return nil
}

func modinit(tmp, host, dir, base string) error {
	path := filepath.Join(tmp, dir, base)
	V("modinit: check %q for go.mod", path)
	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
		V("modinit: it has go.mod")
		return nil
	}
	c := exec.Command("go", "mod", "init", filepath.Join(host, dir, base))
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Env = append(c.Env, "GOPATH="+tmp)
	c.Dir = path
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

func build(tmp, dir, bin string, extra ...string) error {
	c := exec.Command("go", "build", "-o", bin)
	c.Args = append(c.Args, extra...)
	c.Dir = filepath.Join(tmp, dir)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	//	c.Env = append(c.Env, "CGO_ENABLED=0")
	if err := c.Run(); err != nil {
		return err
	}
	return nil
}

// buildToolchain builds the needed Go toolchain binaries: go, compile, link,
// asm.
func buildToolchain(tmp string) error {
	// The traditional location for go is go/bin/go.
	// We are building for multi-architecture, so the traditional
	// location is wrong.
	goBin := filepath.Join(tmp, bin, "go")

	// let's not worry about this atm. We don't care about the size any more.
	//tcbo := golang.BuildOpts{
	//ExtraArgs: []string{"-tags", "cmd_go_bootstrap"},
	//}
	var err error
	if e := build(tmp, "go/src/cmd/go", goBin, "-tags", "cmd_go_bootstrap"); e != nil {
		err = multierror.Append(err, e)
	}

	toolDir := filepath.Join(tmp, "go/pkg/tool", filepath.Dir(bin))
	for _, pkg := range []string{"compile", "link", "asm", "buildid"} {
		c := filepath.Join(toolDir, pkg)
		if e := build(tmp, filepath.Join("go/src/cmd", pkg), c); e != nil {
			err = multierror.Append(err, e)
		}
	}

	return err
}

func goName(p string) (string, string, string, error) {
	u, err := url.ParseScp(p)
	if err != nil {
		return "", "", "", err
	}
	// The `Host` contains both the hostname and the port,
	// if present. Use `SplitHostPort` to extract them.
	fmt.Println(u.Host)
	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
	}
	return host, filepath.Dir(u.Path), filepath.Base(u.Path), nil
}

func get(target string, args ...string) error {
	var err error
	for _, d := range args {
		V("Get %q", d)
		host, dir, base, err := goName(d)
		if err != nil {
			V("URL %q: %v", d, err)
			err = multierror.Append(err, fmt.Errorf("%q: %v", d, err))
			continue
		}
		dir = filepath.Join(host, dir)
		V("goName for %q: %q, %q, %q", d, host, dir, base)
		if e := clone(target, "", d, dir, base); err != nil {
			err = multierror.Append(err, e)
			continue
		}

		if e := modinit(target, host, dir, base); e != nil {
			err = multierror.Append(err, e)
			continue
		}
		if e := tidy(target, dir, base); e != nil {
			err = multierror.Append(err, e)
			continue
		}
		if e := download(target, dir, base); e != nil {
			err = multierror.Append(err, e)
			continue
		}
	}
	return err
}

func init() {
	if a, ok := os.LookupEnv("GOARCH"); ok {
		arch = a
	}
	if a, ok := os.LookupEnv("GOOS"); ok {
		kern = a
	}
}

func tree(d string) error {
	var err error
	for _, n := range []string{"tmp", "dev", "etc"} {
		if e := os.MkdirAll(filepath.Join(d, n), 0755); e != nil {
			err = multierror.Append(err, e)
		}
	}
	return err
}

func files(bin string) error {
	var err error
	if err = os.MkdirAll(bin, 0755); err != nil {
		return err
	}

	for _, n := range []string{
		"/src/github.com/u-root/cpu/cmds/cpud",
		"/src/github.com/u-root/cpu/cmds/cpu",
	} {
		f := filepath.Join(bin, filepath.Base(n))
		dat := []byte("#!/linux_amd64/bin/installcommand #!" + n + "\n")
		V("Write %q with %q", f, dat)
		if e := ioutil.WriteFile(f, dat, 0755); e != nil {
			err = multierror.Append(err, e)
		}
	}
	return err
}

func main() {
	flag.Parse()
	V("Building for %v_%v", arch, kern)

	// Build the target directory
	// Start with a temporary directory
	// copy the toolchain there
	// copy the rest of the other directories there
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	d, err := os.MkdirTemp("", "sourcery")
	defer fmt.Printf("Tree is %q\n", d)
	if err != nil {
		log.Fatal(err)
	}
	if err := tree(d); err != nil {
		log.Fatal(err)
	}
	bin = filepath.Join(fmt.Sprintf("%v_%v", kern, arch), "bin")
	if err := files(filepath.Join(d, bin)); err != nil {
		log.Fatal(err)
	}

	if err := getgo(d, version); err != nil {
		log.Printf("getgo errored, %v, keep going", err)
	}
	if err := buildToolchain(d); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(d, "src"), 0755); err != nil {
		log.Fatal(err)
	}
	if err := get(filepath.Join(d, "src"), append(flag.Args(), "git@github.com:u-root/sourcery")...); err != nil {
		log.Fatalf("Getting packages: %v", err)
	}

	goBin := filepath.Join(d, bin, "installcommand")
	V("Build the installcommand in %q", goBin)
	if err := build(d, "src/github.com/u-root/sourcery/installcommand", goBin); err != nil {
		log.Fatalf("Building installcommand: %v", err)
	}

	goBin = filepath.Join(d, bin, "init")
	V("Build the init in %q", goBin)
	if err := build(pwd, "init", goBin); err != nil {
		log.Fatalf("Building init: %v", err)
	}
	log.Printf("sudo strace -o shit -f unshare -m chroot %q /linux_amd64/bin/init", d)
}
