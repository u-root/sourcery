// Copyright 2013-2020 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// cpio2json reads a cpio and spits it out as a JSON.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/u-root/u-root/pkg/cpio"
)

var (
	debug = func(string, ...interface{}) {}
	d     = flag.Bool("v", false, "Debug prints")
)

func main() {
	archiver, err := cpio.Format("newc")
	if err != nil {
		log.Fatalf("Format newc not supported: %v", err)
	}

	rr, err := archiver.NewFileReader(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	all, err := cpio.ReadAllRecords(rr)
	if err != nil {
		log.Fatal(err)
	}
	if false {
		b, err := json.MarshalIndent(all, "", "\t")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", string(b))
	}
	fmt.Printf("%#v", all)
}
