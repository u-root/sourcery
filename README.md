# sourcery
A new take on u-root source mode, in the age of Go modules.

Sourcery is a program that builds root file systems consisting mostly
of Go source code: of the 90,000 files in a typical sourcery root,
there are only 12 or so programs. Other programs are compiled on
demand to a ramfs-backed file system. Compilation takes a fraction of
a second for most programs, and never more than 2 seconds. Once the
program is compiled to a statically-linked, tmpfs-based binary,
invocation is instantaneous.

Because these images are mostly source, they can also be
multi-architecture. Binaries present on boot have a path formed from
the target os and architecture, e.g. /$OS_$ARCH/bin/init for
init. Dynamically compiled binaries are placed in the tmpfs-backed
/bin, since these binaries vanish on boot, the path can be simpler.

The file system includes the full Go toolchain as well as all source
code. Constructing the root file system, including the git clone steps
and Go toolchain build, takes under 4 minutes; each additional
architecture takes another 90 seconds (to ensure reproducible builds,
the Go toolchain builds itself 3 times).

Sourcery root file systems are designed for VFAT, a standard for
firmware for x86, ARM, and RISC-V. A typical USB stick for sourcery
would include a syslinux bootstrap for x86, required for those
platforms; a kernel Image file for ARM; and a kernel file for RISC-V:
the firmware for ARM and RISC-V is able to find boot kernels without
using an on-stick bootstrap.

Try it yourself!
To try with a typical u-root install, including an emacs-like editor
written in Go:

```
git clone git@github.com:u-root/sourcery
cd sourcery
go build # note: AT LEAST go 1.17
./sourcery git@github.com:u-root/u-root git@github.com:nsf/godit
```

Sourcery will print out a command you can use to try the file system out, including
an strace command if you want to track what it does.
```
unshare -m chroot "/tmp/sourcery3965857644" /linux_amd64/bin/init.
```

Sourcery may be found at github.com:u-root/sourcery.
