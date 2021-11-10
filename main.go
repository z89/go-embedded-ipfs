package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"time"
)

// time tracker
func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("function: %s took %s\n", name, elapsed)
}

// creates a temporary directory under a given string
func createTempDir(str string) string {
	path, err := ioutil.TempDir("", str)
	if err != nil {
		panic(fmt.Errorf("could not create a temporary output directory (%v)", err))
	}

	fmt.Printf("creating temporary output directory for IPFS: %s\n\n", path)

	return path
}

func main() {
	defer timeTrack(time.Now(), "main")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flag.Parse()

	// create a temporary directory for the instance to store content (eg. tempPath = "/tmp1278371/")
	tempPath := createTempDir("tmp") + "/"

	// create() - initalises an embedded IPFS instance w/ a given context
	ipfs := create(ctx)

	/// TEST 1 (must work offline): PASSED
	/** desc:
			1. add a file from local filesystem to the embedded IPFS instance
			2. get the file from the IPFS instance after adding
			3. save the file to a given localtion on disk
	**/

	// // add() - adds any content to IPFS instance
	// fileObj := add(ipfs, ctx, getFile("./data/directory/custom"))

	// // get() - get any given content from IPFS instance
	// file := get(ipfs, ctx, fileObj.Cid().String())

	// // save() - save a file to any given directory
	// save(tempPath, file, fileObj.Cid().String())

	/// TEST 2 (must work online): PASSED
	/** desc:
			1. fetch & add a file from remote IPFS instance to embedded IPFS instance
			2. get the file from the IPFS instance after fetching & adding
			3. save the fetched file to a given location on disk
	**/

	// add() - adds any content to IPFS instance
	// QmdjWNJPGBWL8Vs5M6TFNatphsgTpiPRHXjWt7M5TsDXje - a random picture from pinata.cloud
	fetchedFileObj := fetch(ipfs, ctx, "QmZULkCELmmk5XNfCgTnCyFgAVxBRBXyDHGGMVoLFLiXEN")

	// get() - get any given content from IPFS instance
	fetchedfile := get(ipfs, ctx, fetchedFileObj.Cid().String())

	// save() - save a file to any given directory
	save(tempPath, fetchedfile, fetchedFileObj.Cid().String())
}
