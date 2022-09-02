// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.18
// +build go1.18

package main

import (
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/u-root/u-root/pkg/cpio"
	url "github.com/whilp/git-urls"
)

var (
	version     = "go1.17.7"
	V           = log.Printf
	archList    = flag.String("arches", "amd64", "comma-separated list of GOARCH")
	kernList    = flag.String("kernels", "linux", "comma-seperate list of GOOS")
	bin         string
	testrun     = true
	dest        = flag.String("d", "", "Destination directory -- default is os.MkdirTemp")
	development = flag.Bool("D", true, "Use development (i.e.) pwd version of installcommand/init, not github version")
	outCPIO     = flag.String("cpio", "", "output cpio")
)

// Little note here: you'll see we use go/bin/go a lot, instead of kern_arch/bin/go.
// Lessons learned the hard way: go/bin/go seems to do a better job of finding
// GOROOT and other things when run from go/bin. We have not confirmed this from code,
// but from running it: best to run go/bin/go when doing this build.
// Will it work for replication? Remains to be seen.

func clone(tmp, version, repo, dir, base string) error {
	V("clone: %q, %q, %q, %q", tmp, version, dir, base)
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

func tidy(tmp, dir, base, kern, arch string) error {
	c := exec.Command(filepath.Join(tmp, "go/bin/go"), "mod", "tidy")
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Env = append(c.Env, "GOPATH="+tmp)
	c.Env = append(c.Env, "GOARCH="+arch, "GOOS="+kern, "GOROOT_FINAL=/go", "CGO_ENABLED=0")
	if a, ok := os.LookupEnv("GOARM"); ok {
		c.Env = append(c.Env, a)
	}
	c.Dir = filepath.Join(tmp, dir, base)
	V("Run %v(%q, %q in %q)", c, c.Args, c.Env, c.Dir)
	if err := c.Run(); err != nil {
		return err
	}
	return nil
}

func modinit(tmp, host, dir, base, kern, arch string) error {
	path := filepath.Join(tmp, dir, base)
	V("modinit: check %q for go.mod", path)
	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
		V("modinit: it has go.mod")
		return nil
	}
	c := exec.Command(filepath.Join(tmp, "go/bin/go"), "mod", "init", filepath.Join(host, dir, base))
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

// build builds the code found in filepath.Join(tmp, dir)
// into bin.
func build(tmp, sourcePath, dir, bin, kern, arch string, extra ...string) error {
	c := exec.Command(filepath.Join(tmp, "go/bin/go"), "build", "-o", bin)
	c.Args = append(c.Args, extra...)
	c.Dir = filepath.Join(sourcePath, dir)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Env = os.Environ()
	c.Env = append(c.Env, "GOROOT_FINAL=/go", "CGO_ENABLED=0", "GOOS="+kern, "GOARCH="+arch)
	if err := c.Run(); err != nil {
		return err
	}
	return nil
}

// buildToolchain builds the needed Go toolchain binaries: go, compile, link,
// asm. We can no longer do this without the script. Damn.
// TODO: figure out what files we can remove.
func buildToolchain(tmp, kern, arch string) error {
	c := exec.Command("bash", "make.bash")
	c.Dir = filepath.Join(tmp, "go/src")
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Env = os.Environ()
	c.Env = append(c.Env, "GOROOT_FINAL=/go", "CGO_ENABLED=0")
	if err := c.Run(); err != nil {
		return err
	}
	// Need to also build the go command itself.
	c = exec.Command(filepath.Join(tmp, "go/bin/go"), "build", "-o", filepath.Join(tmp, bin, "go"))
	c.Dir = filepath.Join(tmp, "go/src/cmd/go")
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Env = os.Environ()
	c.Env = append(c.Env, "GOARCH="+arch, "GOOS="+kern, "GOROOT_FINAL=/go", "CGO_ENABLED=0")
	if a, ok := os.LookupEnv("GOARM"); ok {
		c.Env = append(c.Env, a)
	}
	V("Build go toolchain, Args %v, Env %v", c.Args, c.Env)
	if err := c.Run(); err != nil {
		return err
	}
	return nil
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

func get(target string, kernels, archs []string, args ...string) error {
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
		for _, kern := range kernels {
			for _, arch := range archs {
				if e := modinit(target, host, dir, base, kern, arch); e != nil {
					err = multierror.Append(err, e)
					continue
				}
				if e := tidy(target, dir, base, kern, arch); e != nil {
					err = multierror.Append(err, e)
					continue
				}
			}
		}
	}
	return err
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

func files(tmp, binpath, destdir string) error {
	var err error
	if err = os.MkdirAll(destdir, 0755); err != nil {
		return err
	}
	include := filepath.Join(tmp, "go/pkg/include")
	if err = os.MkdirAll(include, 0755); err != nil {
		return err
	}

	// There are certain common patterns we know are commands.
	// Just Do It.
	var dirs []string
	for _, g := range []string{
		"/src/github.com/u-root/u-root/cmds/*/*",
		"/src/github.com/u-root/NiChrome/cmds/*",
		"/src/github.com/u-root/cpu/cmds/*",
		"/src/github.com/nsf/godit",
	} {
		m, err := filepath.Glob(filepath.Join(tmp, g))
		if err != nil {
			V("%q: %v", g, err)
			continue
		}
		dirs = append(dirs, m...)
	}
	for _, n := range dirs {
		r, e := filepath.Rel(tmp, n)
		if e != nil {
			err = multierror.Append(err, e)
		}
		f := filepath.Join(destdir, filepath.Base(n))
		dat := []byte("#!/" + binpath + "/installcommand #!/" + r + "\n")
		V("Write %q with %q", f, dat)
		if e := ioutil.WriteFile(f, dat, 0755); e != nil {
			err = multierror.Append(err, e)
		}
	}

	return err
}

func ramfs(from, out string, filter ...string) error {
	to, err := os.Create(out)
	if err != nil {
		return err
	}
	log.Printf("Archiving to %v", out)
	archiver, err := cpio.Format("newc")
	if err != nil {
		log.Fatalf("Format %q not supported: %v", "newc", err)
	}

	rw := archiver.Writer(to)
	cr := cpio.NewRecorder()

	if err := filepath.WalkDir(from, func(name string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		n, err := filepath.Rel(from, name)
		if err != nil {
			return err
		}
		V("Archive %q", name)
		rec, err := cr.GetRecord(name)
		rec.Name = n
		if err != nil {
			return fmt.Errorf("Getting record of %q failed: %v", name, err)
		}
		if err := rw.WriteRecord(rec); err != nil {
			log.Fatalf("Writing record %q failed: %v", name, err)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := cpio.WriteTrailer(rw); err != nil {
		return fmt.Errorf("Error writing trailer record: %v", err)
	}
	return nil
}

func main() {
	flag.Parse()

	kernels := strings.Split(*kernList, ",")
	archs := strings.Split(*archList, ",")
	V("Building for %v_%v", kernels, archs)

	// Build the target directory
	// Start with a temporary directory
	// copy the toolchain there
	// copy the rest of the other directories there
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	d := *dest
	if len(*dest) == 0 {
		var err error
		d, err = os.MkdirTemp("", "sourcery")
		if err != nil {
			log.Fatal(err)
		}

	}
	defer fmt.Printf("Tree is %q\n", d)
	if err != nil {
		log.Fatal(err)
	}
	if err := tree(d); err != nil {
		log.Fatal(err)
	}
	if err := getgo(d, version); err != nil {
		log.Printf("getgo errored, %v, keep going", err)
	}

	// Some things need GOOS and GOARCH awareness in surprising ways. Modules are once such.
	for _, kern := range kernels {
		for _, arch := range archs {
			bin = filepath.Join(fmt.Sprintf("%v_%v", kern, arch), "bin")
			if err := os.MkdirAll(filepath.Join(d, bin), 0755); err != nil {
				log.Fatal(err)
			}

			if err := buildToolchain(d, kern, arch); err != nil {
				log.Fatal(err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(d, "src"), 0755); err != nil {
		log.Fatal(err)
	}
	if err := get(filepath.Join(d, "src"), kernels, archs, append(flag.Args(), "git@github.com:u-root/sourcery")...); err != nil {
		log.Fatalf("Getting packages: %v", err)
	}

	if err := files(d, bin, filepath.Join(d, bin)); err != nil {
		log.Fatal(err)
	}

	baseToolPath := filepath.Join(d, bin)
	if *development {
		baseToolPath = pwd
	}
	V("Build tools from %q", baseToolPath)
	for _, kern := range kernels {
		for _, arch := range archs {
			for _, tool := range []string{"installcommand", "init"} {
				goBin := filepath.Join(d, bin, tool)
				V("Build %q in %q, install to %q", tool, baseToolPath, goBin)
				if err := build(d, baseToolPath, tool, goBin, kern, arch); err != nil {
					log.Fatalf("Building %q -> %q: %v", goBin, tool, err)
				}
			}

		}
	}
	if *outCPIO != "" {
		if err := ramfs(d, *outCPIO); err != nil {
			log.Printf("ramfs: %v", err)
		}
	}
	log.Printf("sudo strace -o syscalltrace -f unshare -m chroot %q /%q_%q/bin/init", d, kernels, archs)
	log.Printf("unshare -m chroot %q /%q_%q/bin/init", d, kernels, archs)
	log.Printf("rsync -avz --no-owner --no-group -I %q somewhere", d)
}
