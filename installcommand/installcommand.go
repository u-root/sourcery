// Copyright 2012-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// installcommand installs a command from Go source files.
//
// Synopsis:
//     SYMLINK [ARGS...]
//     installcommand [INSTALLCOMMAND_ARGS...] COMMAND [ARGS...]
//
// Description:
//     In u-root's source mode, uncompiled commands in the /bin directory are
//     symbolic links to installcommand. When executed through the symbolic
//     link, installcommand will build the command from source and exec it.
//
//     The second form allows commands to be installed and exec'ed without a
//     symbolic link. In this form additional arguments such as `-v` and
//     `-ludicrous` can be passed into installcommand.
//
// Options:
//     -lowpri:    the scheduler priority to lowered before starting
//     -exec:      build and exec the command
//     -force:     do not build if a file already exists at the destination
//     -v:         print all build commands
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/u-root/u-root/pkg/upath"
)

var (
	lowpri = flag.Bool("lowpri", false, "the scheduler priority is lowered before starting")
	exe    = flag.Bool("exec", true, "build AND execute the command")
	force  = flag.Bool("force", false, "build even if a file already exists at the destination")

	verbose = flag.Bool("v", true, "print all build commands")
	v       = func(string, ...interface{}) {}
	r       = upath.UrootPath
)

type form struct {
	// Name of the command, ex: "ls"
	cmdName string
	// Args passed to the command, ex: {"-l", "-R"}
	cmdArgs []string

	// Args intended for installcommand
	srcPath string
	lowPri  bool
	exec    bool
	force   bool
	verbose bool
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: installcommand [INSTALLCOMMAND_ARGS...] COMMAND [ARGS...]\n")
	os.Exit(2)
}

// Parse the command line to determine the form.
func parseCommandLine() form {
	// First form: 3 args first having a base of installcommand.
	// N.B. sourcery uses #! files, not symlinks.
	// no symlinks on vfat.
	var args []string
	if len(os.Args) > 2 {
		args = os.Args[3:]
	}

	if filepath.Base(os.Args[0]) == "installcommand" {
		return form{
			cmdName: filepath.Base(os.Args[2]),
			cmdArgs: args,
			srcPath: os.Args[1],
			lowPri:  *lowpri,
			exec:    *exe,
			force:   *force,
			verbose: *verbose,
		}
	}

	// Second form:
	//     installcommand [INSTALLCOMMAND_ARGS...] COMMAND [ARGS...]
	flag.Parse()
	if flag.NArg() < 1 {
		log.Println("Second form requires a COMMAND argument")
		usage()
	}
	return form{
		cmdName: flag.Arg(0),
		cmdArgs: flag.Args()[1:],
		lowPri:  *lowpri,
		exec:    *exe,
		force:   *force,
		verbose: *verbose,
	}
}

// run runs the command with the information from form.
// Since run can potentially never return, since it can use Exec,
// it should never return in any other case. Hence, if all goes well
// at the end, we os.Exit(0)
func run(n string, form form) {
	cmd := exec.Command(n, form.cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	v("cmd.Run %q %q", n, form.cmdArgs)
	if err := cmd.Run(); err != nil {
		v("cmd.Run of (%q, %q) returns %v", n, form.cmdArgs, err)
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			log.Fatal(err)
		}
		exitWithStatus(exitErr)
	}
	v("cmd.Run returns OK")
	os.Exit(0)
}

// The kernel will give is this:
// ["/linux_amd64/bin/installcommand" "#!/src/github.com/u-root/u-root/cmds/core/date" "/linux_amd64/bin/date"]
// args[0] tells us we were invoked as the installcommand.
// args[1] tells us the path to source -- no more filepath.Walk!
// args[2] the kernel kindly gives us as the path used -- we can use filepath.Base for the command
// We'll adjust args[1] just to save work.
func main() {
	if *verbose || true {
		v = log.Printf
	}
	v("installcommand called with %q", os.Args)
	// adjust the name if it starts with #!
	if strings.HasPrefix(os.Args[1], "#!") {
		os.Args[1] = os.Args[1][2:]
	}
	form := parseCommandLine()
	v("form is %v", form)

	if form.lowPri {
		if err := lowpriority(); err != nil {
			log.Printf("Cannot set low priority: %v", err)
		}
	}

	destFile := filepath.Join(r("/ubin"), form.cmdName)
	v("destFile is %q", destFile)

	// Is the command there? This covers a race condition
	// in that some other process may have caused it to be
	// built.
	if _, err := os.Stat(destFile); err == nil {
		if !form.exec {
			os.Exit(0)
		}
		run(destFile, form)
	}

	v("Build %q install into %q", form.srcPath, destFile)
	c := exec.Command(fmt.Sprintf("/%s_%s/bin/go", runtime.GOOS, runtime.GOARCH), "build", "-v", "-x", "-o", destFile)
	c.Dir = form.srcPath
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Env = append(c.Env, []string{"GOCACHE=/.cache", "CGO_ENABLED=0", "GOROOT=/go", "GOPATH=/src"}...)
	if err := c.Run(); err != nil {
		log.Fatal(err)
	}

	v("Run it? %q from form %v", destFile, form)
	if form.exec {
		run(destFile, form)
	}
}
