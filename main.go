//
// Copyright (C) 2014 Sebastian 'tokkee' Harl <sh@tokkee.org>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// ``AS IS'' AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED
// TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR
// PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDERS OR
// CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL,
// EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS;
// OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY,
// WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR
// OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF
// ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

// webui is a web-based user-interface for SysDB.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"

	"github.com/sysdb/webui/server"
)

var (
	addr     = flag.String("address", "/var/run/sysdbd.sock", "SysDB server address")
	username *string

	listen = flag.String("listen", ":8080", "address to listen for incoming connections")
	tmpl   = flag.String("template-path", "templates", "location of template files")
	static = flag.String("static-path", "static", "location of static files")

	root = flag.String("root", "/", "root mount point of the server")
)

func init() {
	u, err := user.Current()
	var def string
	if err != nil {
		log.Printf("WARNING: Unable to determine current user: %v", err)
	} else {
		def = u.Username
	}

	username = flag.String("user", def, "SysDB user name")
}

func main() {
	flag.Parse()

	log.Printf("Connecting to SysDB at %s.", *addr)
	srv, err := server.New(*addr, *username, server.Config{
		TemplatePath: *tmpl,
		StaticPath:   *static,
		Root:         *root,
	})
	if err != nil {
		fatalf("Failed to construct web-server: %v", err)
	}

	log.Printf("Listening on %s.", *listen)
	http.Handle(*root, srv)
	err = http.ListenAndServe(*listen, nil)
	if err != nil {
		fatalf("Failed to set up HTTP server on address %q: %v", *listen, err)
	}
}

func fatalf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	fmt.Fprintln(os.Stderr)
	os.Exit(1)
}

// vim: set tw=78 sw=4 sw=4 noexpandtab :
