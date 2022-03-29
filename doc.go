// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Sourcery is a new take on source mode.
// The basic flow is this:
// create a directory in /tmp
// populate with a go toolchain of your choosing
//   For now, you must provide the path
// populate it with repos of your choosing
// populate it via go mod with needed packages
// build an 'installcommand' into a buildbin directory
// populate the buildbin directory with needed commands
// package it into an initramfs
package main
