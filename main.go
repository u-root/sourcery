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
	"regexp"
	"runtime"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/u-root/u-root/pkg/cpio"
	url "github.com/whilp/git-urls"
)

var (
	version     = "go1.17.7"
	V           = log.Printf
	arch        = runtime.GOARCH
	kern        = runtime.GOOS
	bin         string
	testrun     = true
	dest        = flag.String("d", "", "Destination directory -- default is os.MkdirTemp")
	development = flag.Bool("D", true, "Use development (i.e.) pwd version of installcommand/init, not github version")
	outCPIO     = flag.String("cpio", "", "output cpio")
	cpioFilter  = flag.String("filter", "", "filter files/directories/names for cpio")
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

func tidy(tmp, dir, base string) error {
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

func modinit(tmp, host, dir, base string) error {
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
func build(tmp, sourcePath, dir, bin string, extra ...string) error {
	c := exec.Command(filepath.Join(tmp, "go/bin/go"), "build", "-o", bin)
	c.Args = append(c.Args, extra...)
	c.Dir = filepath.Join(sourcePath, dir)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Env = os.Environ()
	c.Env = append(c.Env, "GOROOT_FINAL=/go", "CGO_ENABLED=0")
	if err := c.Run(); err != nil {
		return err
	}
	return nil
}

// buildToolchain builds the needed Go toolchain binaries: go, compile, link,
// asm. We can no longer do this without the script. Damn.
// TODO: figure out what files we can remove.
func buildToolchain(tmp string) error {
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

var root = []cpio.Record{
	{Info: cpio.Info{Mode: 0x41ed, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "bin"}},
	{Info: cpio.Info{Mode: 0x41ed, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "dev"}},
	{Info: cpio.Info{Mode: 0x2180, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x5, Rminor: 0x1, Name: "dev/console"}},
	{Info: cpio.Info{Mode: 0x21b6, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x1, Rminor: 0x3, Name: "dev/null"}},
	{Info: cpio.Info{Mode: 0x21a0, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x1, Rminor: 0x4, Name: "dev/port"}},
	{Info: cpio.Info{Mode: 0x21b6, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x5, Rminor: 0x0, Name: "dev/tty"}},
	{Info: cpio.Info{Mode: 0x21b6, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x1, Rminor: 0x9, Name: "dev/urandom"}},
	{Info: cpio.Info{Mode: 0x41ed, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "env"}},
	{Info: cpio.Info{Mode: 0x41ed, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "etc"}},
	{Info: cpio.Info{Mode: 0x81a4, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x7f, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "etc/localtime"}},
	{Info: cpio.Info{Mode: 0x81a4, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x13, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "etc/resolv.conf"}},
	{Info: cpio.Info{Mode: 0x41ed, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "lib64"}},
	{Info: cpio.Info{Mode: 0x41ed, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "proc"}},
	{Info: cpio.Info{Mode: 0x41ed, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "sys"}},
	{Info: cpio.Info{Mode: 0x41ed, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "tcz"}},
	{Info: cpio.Info{Mode: 0x41ff, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "tmp"}},
	{Info: cpio.Info{Mode: 0x41ed, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "ubin"}},
	{Info: cpio.Info{Mode: 0x41ed, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "usr"}},
	{Info: cpio.Info{Mode: 0x41ed, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "usr/lib"}},
	{Info: cpio.Info{Mode: 0x41ed, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "var"}},
	{Info: cpio.Info{Mode: 0x41ff, UID: 0x0, GID: 0x0, NLink: 0x0, MTime: 0x0, FileSize: 0x0, Dev: 0x0, Major: 0x0, Minor: 0x0, Rmajor: 0x0, Rminor: 0x0, Name: "var/log"}},
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

	if err := cpio.WriteRecords(rw, root); err != nil {
		log.Fatal(err)
	}

	var f = func(n string) bool { return false }
	if len(filter) > 0 {
		re := regexp.MustCompile(strings.Join(filter, "|"))
		f = func(n string) bool {
			return re.MatchString(n)
		}
	}

	for _, r := range root {
		V("Write %v", r)
		if err := rw.WriteRecord(r); err != nil {
			log.Fatalf("Writing record %v failed: %v", r, err)
		}
	}

	if err := filepath.WalkDir(from, func(name string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		n, err := filepath.Rel(from, name)
		if err != nil {
			return err
		}
		if f(name) {
			V("SKIP %q", name)
			if de.IsDir() {
				return filepath.SkipDir
			}
			return nil
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
	V("Building for %v_%v", arch, kern)

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
	bin = filepath.Join(fmt.Sprintf("%v_%v", kern, arch), "bin")
	if err := os.MkdirAll(filepath.Join(d, bin), 0755); err != nil {
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

	if err := files(d, bin, filepath.Join(d, bin)); err != nil {
		log.Fatal(err)
	}

	baseToolPath := filepath.Join(d, bin)
	if *development {
		baseToolPath = pwd
	}
	V("Build tools from %q", baseToolPath)
	for _, tool := range []string{"installcommand", "init"} {
		goBin := filepath.Join(d, bin, tool)
		V("Build %q in %q, install to %q", tool, baseToolPath, goBin)
		if err := build(d, baseToolPath, tool, goBin); err != nil {
			log.Fatalf("Building %q -> %q: %v", goBin, tool, err)
		}
	}

	if *outCPIO != "" {
		if err := ramfs(d, *outCPIO, "\\.git", "testdata", "go/pkg/[^/][^/]*_[^/][^/]*/"); err != nil {
			log.Printf("ramfs: %v", err)
		}
	}
	log.Printf("sudo strace -o syscalltrace -f unshare -m chroot %q /%q_%q/bin/init", d, kern, arch)
	log.Printf("unshare -m chroot %q /%q_%q/bin/init", d, kern, arch)
	log.Printf("rsync -avz --no-owner --no-group -I %q somewhere", d)
}
