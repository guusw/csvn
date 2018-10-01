package main

import (
	"log"
	"os/exec"
)

var svnPath string

func init() {
	var err error
	svnPath, err = exec.LookPath("svn")
	if err != nil {
		log.Fatalf("svn executable not found (%s)", err)
	}
}
